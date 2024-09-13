package baselines

import (
	pb "ECDS/proto" // 根据实际路径修改
	"ECDS/util"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type SiaAC struct {
	IpAddr                             string                             //审计方的IP地址
	SNAddrMap                          map[string]string                  //存储节点的地址表，key:存储节点id，value:存储节点地址
	ClientDSSNMap                      map[string]string                  //客户端文件每个分片所在的存储节点表，key:clientID-filename-i,value:snid
	CDSSNMMutex                        sync.RWMutex                       //ClientFileShardsMap的读写锁
	ClientDSVersionMap                 map[string]int                     //当前文件版本号,key:clientID-filename,value:版本号
	FRRMMutex                          sync.RWMutex                       //FileRootMap和FileRandMap的读写锁
	DataNum                            int                                //系统中用于文件编码的数据块数量
	ParityNum                          int                                //系统中用于文件编码的校验块数量
	pb.UnimplementedSiaACServiceServer                                    // 嵌入匿名字段
	SNRPCs                             map[string]pb.SiaSNACServiceClient //存储节点RPC对象列表，key:存储节点id
	SNRPCMutex                         sync.RWMutex                       //SNRPCs的读写锁
	PendingClientRootMap               map[string][]byte                  //待存储的存储节点传回的根节点哈希，key:clientID-filename-i,value:根节点哈希值
	PendingFileSNMap                   map[string]string                  //待存储的文件分片所在的存储节点，key:clientID-filename-i,value:snid
	PendingDSMerklePathMap             map[string][][]byte                //待存储的文件分片的有效性证明，key:clientID-filename-i,value:merkle path
	PendingDSIndexMap                  map[string]int                     //待存储的文件分片在Merkel树叶子列表中的索引号，key:clientID-filename-i,value:索引号
	PFRMMutex                          sync.RWMutex                       //PendingFileRootMap的读写锁
	PendingUpdateFileMap               map[string]map[string]int          //待更新的文件副本列表，1-等待更新，2-更新完成
	PendingUpdateFileVersionMap        map[string]map[string]int          //待更新的文件副本版本号列表，key:cid,value:subkey:filename-i,subvalue:version
	PUFMMutex                          sync.RWMutex                       //PendingUpdateFileMap的读写锁
	IsAudit                            bool                               //是否开始审计的标识
	IAMutex                            sync.RWMutex                       //IsAudit的读写锁
	MulVFileRootMap                    map[string]map[string][]byte       //多版本文件根节点哈希值表，key:cid-fn-i,subkey:版本号,subvalue:根节点哈希值
	MulVFileRandMap                    map[string]map[string][]int32      //多版本文件随机数组表，key:cid-fn-i,subkey:版本号,subvalue:随机数组
	MFRRMutex                          sync.RWMutex                       //MulVFileRootMap,MulVFileRandMap的读写锁
}

// 新建一个审计方，持续监听消息
func NewSiaAC(ipaddr string, snaddrfilename string, dn int, pn int) *SiaAC {
	snaddrmap := util.ReadSNAddrFile(snaddrfilename)
	cdssnmap := make(map[string]string)
	cdsvmap := make(map[string]int)
	pcrmap := make(map[string][]byte)
	pfsnmap := make(map[string]string)
	pdsmpmap := make(map[string][][]byte)
	pdsimap := make(map[string]int)
	pufmap := make(map[string]map[string]int)
	pufvmap := make(map[string]map[string]int)
	mvfrtmap := make(map[string]map[string][]byte)
	mvfrdmap := make(map[string]map[string][]int32)
	//设置连接存储节点服务器的地址
	snrpcs := make(map[string]pb.SiaSNACServiceClient)
	for key, value := range *snaddrmap {
		snconn, err := grpc.Dial(value, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
		if err != nil {
			log.Fatalf("did not connect to storage node: %v", err)
		}
		sc := pb.NewSiaSNACServiceClient(snconn)
		snrpcs[key] = sc
	}
	auditor := &SiaAC{ipaddr, *snaddrmap, cdssnmap, sync.RWMutex{}, cdsvmap, sync.RWMutex{}, dn, pn, pb.UnimplementedSiaACServiceServer{}, snrpcs, sync.RWMutex{}, pcrmap, pfsnmap, pdsmpmap, pdsimap, sync.RWMutex{}, pufmap, pufvmap, sync.RWMutex{}, false, sync.RWMutex{}, mvfrtmap, mvfrdmap, sync.RWMutex{}}
	//设置监听地址
	lis, err := net.Listen("tcp", ipaddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterSiaACServiceServer(s, auditor)
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
func (ac *SiaAC) SiaSelectSNs(ctx context.Context, sreq *pb.SiaStorageRequest) (*pb.SiaStorageResponse, error) {
	// log.Printf("Received storage message from client: %s\n", sreq.ClientId)
	// 为该客户端文件随机选择存储节点
	dssnids, pssnids := ac.SiaSelectStorNodes(sreq.Filename)
	//启动线程通知各存储节点待存储的文件分片序号,等待存储完成
	go ac.SiaPutFileNoticeToSN(sreq.ClientId, sreq.Filename, dssnids, pssnids)
	return &pb.SiaStorageResponse{Filename: sreq.Filename, SnsForDs: dssnids, SnsForPs: pssnids}, nil
}

// AC选择存储节点
func (ac *SiaAC) SiaSelectStorNodes(filename string) ([]string, []string) {
	dssnids := make([]string, 0)
	pssnids := make([]string, 0)
	seed := time.Now().UnixNano()
	randor := rand.New(rand.NewSource(seed))
	selectednum := make(map[string]bool)
	for {
		randomNum := randor.Intn(ac.DataNum+ac.ParityNum) + 1 // 生成 1 到 (ac.DataNum+ac.ParityNum) 之间的随机数
		selectednum[strconv.Itoa(randomNum)] = true
		if len(selectednum) == ac.DataNum {
			break
		}
	}
	for i := 1; i < ac.DataNum+ac.ParityNum+1; i++ {
		if selectednum[strconv.Itoa(i)] {
			dssnids = append(dssnids, "sn"+strconv.Itoa(i))
		} else {
			pssnids = append(pssnids, "sn"+strconv.Itoa(i))
		}
	}
	return dssnids, pssnids
}

// 【SelectSNs-RPC被调用时自动触发】通知存储节点客户端待存放的文件分片序号，等待SN存储结果
func (ac *SiaAC) SiaPutFileNoticeToSN(cid string, fn string, dssnids []string, pssnids []string) {
	for i := 0; i < len(dssnids); i++ {
		go func(snid string, i int) {
			//构造请求消息
			pds_req := &pb.SiaClientStorageRequest{
				ClientId: cid,
				Filename: fn,
				Dsno:     "d-" + strconv.Itoa(i),
			}
			//发送请求消息给存储节点
			pds_res, err := ac.SNRPCs[snid].SiaPutFileNotice(context.Background(), pds_req)
			if err != nil {
				log.Fatalf("storagenode could not process request: %v", err)
			}
			//处理存储节点回复消息：
			// log.Println("recieved putds respones:", pds_res.Message)
			//1-将存储节点放入正式列表中
			cid_fn := pds_res.ClientId + "-" + pds_res.Filename
			dsno := pds_res.Dsno
			ac.PFRMMutex.Lock()
			ac.PendingFileSNMap[cid_fn+"-"+dsno] = snid
			ac.PendingClientRootMap[cid_fn+"-"+dsno] = pds_res.Root
			ac.PendingDSMerklePathMap[cid_fn+"-"+dsno] = pds_res.Merklepath
			ac.PendingDSIndexMap[cid_fn+"-"+dsno] = int(pds_res.Index)
			ac.PFRMMutex.Unlock()
		}(dssnids[i], i)
	}

	for i := 0; i < len(pssnids); i++ {
		go func(snid string, i int) {
			//构造请求消息
			pds_req := &pb.SiaClientStorageRequest{
				ClientId: cid,
				Filename: fn,
				Dsno:     "p-" + strconv.Itoa(i),
			}
			//发送请求消息给存储节点
			pds_res, err := ac.SNRPCs[snid].SiaPutFileNotice(context.Background(), pds_req)
			if err != nil {
				log.Fatalf("storagenode could not process request: %v", err)
			}
			//处理存储节点回复消息：
			// log.Println("recieved putds respones:", pds_res.Message)
			//1-将存储节点放入正式列表中
			cid_fn := pds_res.ClientId + "-" + pds_res.Filename
			dsno := pds_res.Dsno
			ac.PFRMMutex.Lock()
			ac.PendingFileSNMap[cid_fn+"-"+dsno] = snid
			ac.PendingClientRootMap[cid_fn+"-"+dsno] = pds_res.Root
			ac.PendingDSMerklePathMap[cid_fn+"-"+dsno] = pds_res.Merklepath
			ac.PendingDSIndexMap[cid_fn+"-"+dsno] = int(pds_res.Index)
			ac.PFRMMutex.Unlock()
		}(pssnids[i], i)
	}
}

// 【供client使用的RPC】文件存放确认:确认文件已在存储节点上完成存放,确认元信息一致
func (ac *SiaAC) SiaPutFileCommit(ctx context.Context, req *pb.SiaPFCRequest) (*pb.SiaPFCResponse, error) {
	message := ""
	fn := req.Filename
	dshashMap := req.Dshashmap
	isValid := true
	// 遍历收到的每个分片的哈希值，验证Merkel路径是否正确
	for key, dshash := range dshashMap {
		var snid string
		// 等待获取存储节点传来的存储回复
		for {
			ac.PFRMMutex.RLock()
			// fmt.Println(key, "ac.PendingFileSNMap[key]:", ac.PendingFileSNMap[key])
			if ac.PendingFileSNMap[key] != "" {
				snid = ac.PendingFileSNMap[key]
				ac.PFRMMutex.RUnlock()
				break
			}
			ac.PFRMMutex.RUnlock()
		}
		//获取根节点哈希，Merkle路径，索引号
		ac.PFRMMutex.RLock()
		pendingRoot := ac.PendingClientRootMap[key]
		pendingMerklePath := ac.PendingDSMerklePathMap[key]
		pendingIndex := ac.PendingDSIndexMap[key]
		ac.PFRMMutex.RUnlock()
		// 验证Merkle路径的有效性
		newRoot := util.GenerateRootByPaths(dshash, pendingIndex, pendingMerklePath)
		if !bytes.Equal(newRoot, pendingRoot) {
			isValid = false
			message = "Faild"
			e := errors.New("merkle path verify failed")
			return nil, e
		} else {
			//验证有效，则将信息永久记录，并在pending列表中删除
			ac.CDSSNMMutex.Lock()
			ac.ClientDSSNMap[key] = snid
			ac.CDSSNMMutex.Unlock()
			ac.FRRMMutex.Lock()
			ac.ClientDSVersionMap[key] = 1
			ac.FRRMMutex.Unlock()
			ac.PFRMMutex.Lock()
			delete(ac.PendingFileSNMap, key)
			delete(ac.PendingDSMerklePathMap, key)
			delete(ac.PendingDSIndexMap, key)
			delete(ac.PendingClientRootMap, key)
			ac.PFRMMutex.Unlock()
		}
		// fmt.Println(key, snid, "Merkle Path Verify:", isValid)
	}
	if isValid {
		fmt.Println(req.ClientId, fn, "Merkle Path Verify:", isValid)
		message = "OK"
	}
	return &pb.SiaPFCResponse{Filename: fn, Message: message}, nil
}
