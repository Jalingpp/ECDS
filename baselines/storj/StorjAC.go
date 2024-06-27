package storjnodes

import (
	pb "ECDS/proto" // 根据实际路径修改
	"ECDS/util"
	"bytes"
	"context"
	"errors"
	"log"
	"net"
	"strconv"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type StorjAC struct {
	IpAddr                               string                               //审计方的IP地址
	SNAddrMap                            map[string]string                    //存储节点的地址表，key:存储节点id，value:存储节点地址
	ClientFileRepMap                     map[string]map[string]string         //客户端文件每个备份所在的存储节点表，key:客户端id，value:subkey:filename-i,subvalue:文件备份对应的存储节点id列表
	CFRMMutex                            sync.RWMutex                         //ClientFileShardsMap的读写锁
	FileRootMap                          map[string]map[string][]byte         //文件的默克尔树根节点哈希值列表，key:clientID,value:subkey:filename-i,subvalue:根节点哈希值
	FileRandMap                          map[string]map[string][]int32        //文件的随机数集,key:clientID,value:subkey:filename-i,subvalue:随机数数组
	FRRMMutex                            sync.RWMutex                         //FileRootMap和FileRandMap的读写锁
	DataNum                              int                                  //系统中用于文件编码的数据块数量
	ParityNum                            int                                  //系统中用于文件编码的校验块数量
	pb.UnimplementedStorjACServiceServer                                      // 嵌入匿名字段
	SNRPCs                               map[string]pb.StorjSNACServiceClient //存储节点RPC对象列表，key:存储节点id
	SNRPCMutex                           sync.RWMutex                         //SNRPCs的读写锁
	SNpointer                            int                                  //指示当前分派的存储节点索引号，用于均匀选择（2f+1）个存储节点
	SNPMutex                             sync.Mutex                           //SNpointer的写锁
	PendingFileRootMap                   map[string]map[string][]byte         //存储节点传回的根节点哈希，key:clientID,value:subkey:filename-i，subvalue:根节点哈希值
	PendingFileSNMap                     map[string]map[string]string         //文件副本所在的存储节点，key:clientID,value:subkey:filename-i,subvalue:snid
	PFRMMutex                            sync.RWMutex                         //PendingFileRootMap的读写锁
}

// 新建一个审计方，持续监听消息
func NewStorjAC(ipaddr string, snaddrfilename string, dn int, pn int) *StorjAC {
	snaddrmap := util.ReadSNAddrFile(snaddrfilename)
	cfrmap := make(map[string]map[string]string)
	fromap := make(map[string]map[string][]byte)
	framap := make(map[string]map[string][]int32)
	pfrmap := make(map[string]map[string][]byte)
	pfsnmap := make(map[string]map[string]string)
	//设置连接存储节点服务器的地址
	snrpcs := make(map[string]pb.StorjSNACServiceClient)
	for key, value := range *snaddrmap {
		snconn, err := grpc.Dial(value, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
		if err != nil {
			log.Fatalf("did not connect to storage node: %v", err)
		}
		sc := pb.NewStorjSNACServiceClient(snconn)
		snrpcs[key] = sc
	}
	auditor := &StorjAC{ipaddr, *snaddrmap, cfrmap, sync.RWMutex{}, fromap, framap, sync.RWMutex{}, dn, pn, pb.UnimplementedStorjACServiceServer{}, snrpcs, sync.RWMutex{}, 1, sync.Mutex{}, pfrmap, pfsnmap, sync.RWMutex{}}
	//设置监听地址
	lis, err := net.Listen("tcp", ipaddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterStorjACServiceServer(s, auditor)
	// log.Println("Server listening on port 50051")
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	//启动持续审计
	// go auditor.KeepAuditing(20)
	return auditor
}

// 【供Client使用的RPC】选择存储节点，给客户端回复消息
func (ac *StorjAC) StorjSelectSNs(ctx context.Context, sreq *pb.StorjStorageRequest) (*pb.StorjStorageResponse, error) {
	// log.Printf("Received storage message from client: %s\n", sreq.ClientId)
	// 为该客户端文件随机选择存储节点
	snids := ac.StorjSelectStorNodes(sreq.Filename)
	//启动线程通知各存储节点待存储的文件分片序号,等待存储完成
	go ac.StorjPutFileNoticeToSN(sreq.ClientId, sreq.Filename, snids)
	return &pb.StorjStorageResponse{Filename: sreq.Filename, SnsForFd: snids}, nil
}

// AC选择(2f+1)个存储节点
func (ac *StorjAC) StorjSelectStorNodes(filename string) []string {
	snids := make([]string, 0)
	snnum := ac.ParityNum + 1
	ac.SNPMutex.Lock()
	snp := ac.SNpointer
	for i := 0; i < snnum; i++ {
		snp = ac.SNpointer + i
		if snp > 31 {
			snp = snp - 31
		}
		snids = append(snids, "sn"+strconv.Itoa(snp))
	}
	ac.SNpointer = snp
	ac.SNPMutex.Unlock()
	return snids
}

// 【SelectSNs-RPC被调用时自动触发】通知存储节点客户端待存放的文件分片序号，等待SN存储结果
func (ac *StorjAC) StorjPutFileNoticeToSN(cid string, fn string, snids []string) {
	for i := 0; i < len(snids); i++ {
		go func(snid string, i int) {
			//构造请求消息
			pds_req := &pb.StorjClientStorageRequest{
				ClientId: cid,
				Filename: fn,
				Repno:    int32(i),
			}
			//发送请求消息给存储节点
			pds_res, err := ac.SNRPCs[snid].StorjPutFileNotice(context.Background(), pds_req)
			if err != nil {
				log.Println("storagenode could not process request:", err)
			}
			//处理存储节点回复消息：暂存存储节点返回的文件根哈希
			indexKey := pds_res.Filename + "-" + strconv.Itoa(int(pds_res.Repno))
			ac.PFRMMutex.Lock()
			if ac.PendingFileRootMap[pds_res.ClientId] == nil {
				ac.PendingFileRootMap[pds_res.ClientId] = make(map[string][]byte)
			}
			ac.PendingFileRootMap[pds_res.ClientId][indexKey] = pds_res.Root
			if ac.PendingFileSNMap[pds_res.ClientId] == nil {
				ac.PendingFileSNMap[pds_res.ClientId] = make(map[string]string)
			}
			ac.PendingFileSNMap[pds_req.ClientId][indexKey] = pds_res.Snid
			ac.PFRMMutex.Unlock()
			log.Println("已完成对存储节点", snid, "的通知")
		}(snids[i], i)
	}
}

// 【供client使用的RPC】文件存放确认:确认文件已在存储节点上完成存放,确认元信息一致
func (ac *StorjAC) StorjPutFileCommit(ctx context.Context, req *pb.StorjPFCRequest) (*pb.StorjPFCResponse, error) {
	message := ""
	cid := req.ClientId
	// 遍历收到的每个副本信息，验证根哈希的一致性，保存随机数集
	for key, _ := range req.Randmap {
		// 等待获取存储节点传来的根哈希
		for {
			ac.PFRMMutex.RLock()
			if ac.PendingFileRootMap[cid] != nil {
				root := ac.PendingFileRootMap[cid][key]
				if root != nil {
					ac.PFRMMutex.RUnlock()
					break
				}
			}
			ac.PFRMMutex.RUnlock()
		}
		ac.PFRMMutex.RLock()
		pendingroot := ac.PendingFileRootMap[cid][key]
		pendingSN := ac.PendingFileSNMap[cid][key]
		log.Println(cid, key, "pendingroot:", pendingroot, "pendingSN:", pendingSN)
		ac.PFRMMutex.RUnlock()
		// 验证根哈希的一致性
		ac.FRRMMutex.Lock()
		if bytes.Equal(req.Rootmap[key], pendingroot) {
			// 保存根节点哈希和随机数集
			if ac.FileRootMap[cid] == nil {
				ac.FileRootMap[cid] = make(map[string][]byte)
			}
			ac.FileRootMap[cid][key] = req.Rootmap[key]
			if ac.FileRandMap[cid] == nil {
				ac.FileRandMap[cid] = make(map[string][]int32)
			}
			for i := 0; i < len(req.Randmap[key].Values); i++ {
				ac.FileRandMap[cid][key] = append(ac.FileRandMap[cid][key], req.Randmap[key].Values[i])
			}
			ac.CFRMMutex.Lock()
			if ac.ClientFileRepMap[cid] == nil {
				ac.ClientFileRepMap[cid] = make(map[string]string)
			}
			ac.ClientFileRepMap[cid][key] = pendingSN
			ac.CFRMMutex.Unlock()
			// 删除pendinglist中该key对应的根哈希
			ac.PFRMMutex.Lock()
			delete(ac.PendingFileRootMap[cid], key)
			delete(ac.PendingFileSNMap[cid], key)
			ac.PFRMMutex.Unlock()
		} else {
			// 不一致则返回错误
			ac.FRRMMutex.Unlock()
			message = key + " storage error"
			e := errors.New("root hash not consist")
			return &pb.StorjPFCResponse{Filename: req.Filename, Message: message}, e
		}
		ac.FRRMMutex.Unlock()
	}
	message = req.Filename + " complete storage"
	return &pb.StorjPFCResponse{Filename: req.Filename, Message: message}, nil
}

// 打印auditor
func (ac *StorjAC) PrintStorjAuditor() {
	str := "Auditor:{IpAddr:" + ac.IpAddr + ",SNAddrMap:{"
	for key, value := range ac.SNAddrMap {
		str = str + key + ":" + value + ","
	}
	str = str + "},ClientFileShardMap:{"
	for key, value := range ac.ClientFileRepMap {
		str = str + key + ":{"
		for k, v := range value {
			str = str + k + ":" + v + ","
		}
		str = str + "},"
	}
	str = str + "},DataNum:" + strconv.Itoa(ac.DataNum) + ",ParityNum:" + strconv.Itoa(ac.ParityNum) + "}"
	log.Println(str)
}
