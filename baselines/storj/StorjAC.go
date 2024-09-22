package baselines

import (
	pb "ECDS/proto" // 根据实际路径修改
	"ECDS/util"
	"bytes"
	"context"
	"errors"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

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
	FileVersionMap                       map[string]map[string]int            //当前文件版本号,key:clientID,value:subkey:filename-i,subvalue:版本号
	FRRMMutex                            sync.RWMutex                         //FileRootMap和FileRandMap的读写锁
	DataNum                              int                                  //系统中用于文件编码的数据块数量
	ParityNum                            int                                  //系统中用于文件编码的校验块数量
	pb.UnimplementedStorjACServiceServer                                      // 嵌入匿名字段
	SNRPCs                               map[string]pb.StorjSNACServiceClient //存储节点RPC对象列表，key:存储节点id
	SNRPCMutex                           sync.RWMutex                         //SNRPCs的读写锁
	SNpointer                            int                                  //指示当前分派的存储节点索引号，用于均匀选择（2f+1）个存储节点
	SNPMutex                             sync.Mutex                           //SNpointer的写锁
	PendingFileRootMap                   map[string]map[string][]byte         //待存储的存储节点传回的根节点哈希，key:clientID,value:subkey:filename-i，subvalue:根节点哈希值
	PendingFileSNMap                     map[string]map[string]string         //待存储的文件副本所在的存储节点，key:clientID,value:subkey:filename-i,subvalue:snid
	PendingFileVMap                      map[string]map[string]int            //待存储的文件副本的版本号，key:clientID,value:subkey:filename-i,subvalue:版本号
	PFRMMutex                            sync.RWMutex                         //PendingFileRootMap的读写锁
	PendingUpdateFileMap                 map[string]map[string]int            //待更新的文件副本列表，1-等待更新，2-更新完成
	PendingUpdateFileVersionMap          map[string]map[string]int            //待更新的文件副本版本号列表，key:cid,value:subkey:filename-i,subvalue:version
	PUFMMutex                            sync.RWMutex                         //PendingUpdateFileMap的读写锁
	IsAudit                              bool                                 //是否开始审计的标识
	IAMutex                              sync.RWMutex                         //IsAudit的读写锁
	MulVFileRootMap                      map[string]map[string][]byte         //多版本文件根节点哈希值表，key:cid-fn-i,subkey:版本号,subvalue:根节点哈希值
	MulVFileRandMap                      map[string]map[string][]int32        //多版本文件随机数组表，key:cid-fn-i,subkey:版本号,subvalue:随机数组
	MFRRMutex                            sync.RWMutex                         //MulVFileRootMap,MulVFileRandMap的读写锁
}

// 新建一个审计方，持续监听消息
func NewStorjAC(ipaddr string, snaddrfilename string, dn int, pn int) *StorjAC {
	snaddrmap := util.ReadSNAddrFile(snaddrfilename)
	cfrmap := make(map[string]map[string]string)
	fromap := make(map[string]map[string][]byte)
	framap := make(map[string]map[string][]int32)
	fvmap := make(map[string]map[string]int)
	pfrmap := make(map[string]map[string][]byte)
	pfsnmap := make(map[string]map[string]string)
	pfvmap := make(map[string]map[string]int)
	pufmap := make(map[string]map[string]int)
	pufvmap := make(map[string]map[string]int)
	mvfrtmap := make(map[string]map[string][]byte)
	mvfrdmap := make(map[string]map[string][]int32)
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
	auditor := &StorjAC{ipaddr, *snaddrmap, cfrmap, sync.RWMutex{}, fromap, framap, fvmap, sync.RWMutex{}, dn, pn, pb.UnimplementedStorjACServiceServer{}, snrpcs, sync.RWMutex{}, 1, sync.Mutex{}, pfrmap, pfsnmap, pfvmap, sync.RWMutex{}, pufmap, pufvmap, sync.RWMutex{}, false, sync.RWMutex{}, mvfrtmap, mvfrdmap, sync.RWMutex{}}
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
	go auditor.KeepAuditing(20)
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

// 【StorjSelectSNs-RPC被调用时自动触发】通知存储节点客户端待存放的文件分片序号，等待SN存储结果
func (ac *StorjAC) StorjPutFileNoticeToSN(cid string, fn string, snids []string) {
	for i := 0; i < len(snids); i++ {
		go func(snid string, i int) {
			//构造请求消息
			pds_req := &pb.StorjClientStorageRequest{
				ClientId: cid,
				Filename: fn,
				Repno:    int32(i),
				Version:  int32(1),
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
			if ac.PendingFileVMap[pds_res.ClientId] == nil {
				ac.PendingFileVMap[pds_res.ClientId] = make(map[string]int)
			}
			ac.PendingFileVMap[pds_res.ClientId][indexKey] = 1
			ac.PFRMMutex.Unlock()
			// log.Println("已完成对存储节点", snid, "的通知")
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
		pendingV := ac.PendingFileVMap[cid][key]
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
			if ac.FileVersionMap[cid] == nil {
				ac.FileVersionMap[cid] = make(map[string]int)
			}
			ac.FileVersionMap[cid][key] = pendingV
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
			delete(ac.PendingFileVMap[cid], key)
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

// 【供client使用的RPC】获取文件数据分片所在的存储节点id
func (ac *StorjAC) StorjGetFileSNs(ctx context.Context, req *pb.StorjGFACRequest) (*pb.StorjGFACResponse, error) {
	cid := req.ClientId
	snsds := make(map[string]string)
	//为客户端找到文件的所有数据分片对应的存储节点
	ac.CFRMMutex.RLock()
	if ac.ClientFileRepMap[cid] == nil {
		ac.CFRMMutex.RUnlock()
		e := errors.New("client filename not exist")
		return &pb.StorjGFACResponse{Filename: req.Filename, Snsds: snsds}, e
	} else {
		//遍历只返回以filename为前缀的
		for key, value := range ac.ClientFileRepMap[cid] {
			splitkey := strings.Split(key, "-")
			if splitkey[0] == req.Filename {
				snsds[key] = value
			}
		}
		ac.CFRMMutex.RUnlock()
	}
	return &pb.StorjGFACResponse{Filename: req.Filename, Snsds: snsds}, nil
}

// 【供client使用的RPC】获取副本的随机数集和默克尔树根
func (ac *StorjAC) StorjGetRandRoot(ctx context.Context, req *pb.StorjGRRRequest) (*pb.StorjGRRResponse, error) {
	fn_rep := req.Filename + "-" + req.Rep
	ac.FRRMMutex.RLock()
	if ac.FileRandMap[req.ClientId] == nil {
		ac.FRRMMutex.RUnlock()
		err := errors.New("clientid not in filerandmap")
		return nil, err
	} else if ac.FileRootMap[req.ClientId] == nil {
		ac.FRRMMutex.RUnlock()
		err := errors.New("clientid not in filerootmap")
		return nil, err
	} else if ac.FileRandMap[req.ClientId][fn_rep] == nil {
		ac.FRRMMutex.RUnlock()
		err := errors.New("file rep rands not exist")
		return nil, err
	} else if ac.FileRootMap[req.ClientId][fn_rep] == nil {
		ac.FRRMMutex.RUnlock()
		err := errors.New("file rep root not exist")
		return nil, err
	}
	randIntarray := &pb.Int32ArrayAC{Values: ac.FileRandMap[req.ClientId][fn_rep]}
	root := ac.FileRootMap[req.ClientId][fn_rep]
	ac.FRRMMutex.RUnlock()
	return &pb.StorjGRRResponse{Rands: randIntarray, Root: root}, nil
}

// 【供client使用的RPC】更新存储文件请求，添加更新至待定区，返回文件副本所在的存储节点id
func (ac *StorjAC) StorjUpdateFileReq(ctx context.Context, req *pb.StorjUFRequest) (*pb.StorjUFResponse, error) {
	snsds := make(map[string]string)
	ac.CFRMMutex.RLock()
	if ac.ClientFileRepMap[req.ClientId] == nil {
		ac.CFRMMutex.RUnlock()
		e := errors.New("client id not exsist")
		return nil, e
	} else {
		//遍历只返回以filename为前缀的
		for key, value := range ac.ClientFileRepMap[req.ClientId] {
			splitkey := strings.Split(key, "-")
			if splitkey[0] == req.Filename {
				snsds[key] = value
				//加入待更新列表
				ac.PUFMMutex.Lock()
				if ac.PendingUpdateFileMap[req.ClientId] == nil {
					ac.PendingUpdateFileMap[req.ClientId] = make(map[string]int)
				}
				ac.PendingUpdateFileMap[req.ClientId][key] = 1
				if ac.PendingUpdateFileVersionMap[req.ClientId] == nil {
					ac.PendingUpdateFileVersionMap[req.ClientId] = make(map[string]int)
				}
				ac.PendingUpdateFileVersionMap[req.ClientId][key] = int(req.Version)
				ac.PUFMMutex.Unlock()
			}
		}
		ac.CFRMMutex.RUnlock()
	}
	//通知所有相关存储节点待更新的文件副本
	go ac.StorjUpdateFileNoticeToSN(req.ClientId, req.Filename, req.Version, snsds)
	return &pb.StorjUFResponse{Filename: req.Filename, Snsds: snsds}, nil
}

// 【StorjUpdateFileReq-RPC被调用时自动触发】通知存储节点客户端待更新的文件副本序号，等待SN更新结果
func (ac *StorjAC) StorjUpdateFileNoticeToSN(cid string, fn string, nv int32, snids map[string]string) {
	for fni, snid := range snids {
		splitfni := strings.Split(fni, "-")
		rep, _ := strconv.Atoi(splitfni[1])
		go func(snid string, rep int) {
			//构造请求消息
			ufn_req := &pb.StorjClientUFRequest{
				ClientId: cid,
				Filename: fn,
				Rep:      int32(rep),
				Version:  nv,
			}
			//发送请求消息给存储节点
			ufn_res, err := ac.SNRPCs[snid].StorjUpdateFileNotice(context.Background(), ufn_req)
			if err != nil {
				log.Println("storagenode could not process request:", err)
			}
			//处理存储节点回复消息：暂存存储节点返回的文件根哈希
			indexKey := ufn_res.Filename + "-" + strconv.Itoa(int(ufn_res.Repno))
			ac.PFRMMutex.Lock()
			ac.PendingFileRootMap[ufn_res.ClientId][indexKey] = ufn_res.Root
			ac.PendingFileVMap[ufn_res.ClientId][indexKey] = int(nv)
			ac.PFRMMutex.Unlock()
			//修改待更新信息为2
			ac.PUFMMutex.Lock()
			ac.PendingUpdateFileMap[ufn_res.ClientId][indexKey] = 2
			ac.PendingUpdateFileVersionMap[ufn_req.ClientId][indexKey] = int(nv)
			ac.PUFMMutex.Unlock()
			// log.Println("已完成对存储节点", snid, "的通知")
		}(snid, rep)
	}
}

// 【供client使用的RPC】文件更新确认:确认文件已在存储节点上完成更新,确认元信息一致
func (ac *StorjAC) StorjUpdateFileCommit(ctx context.Context, req *pb.StorjUFCRequest) (*pb.StorjUFCResponse, error) {
	message := ""
	cid := req.ClientId
	// 遍历收到的每个副本信息，验证根哈希的一致性，保存随机数集
	for key, _ := range req.Randmap {
		// 等待存储节点更新完成
		for {
			ac.PUFMMutex.RLock()
			if ac.PendingUpdateFileMap[cid] != nil {
				if ac.PendingUpdateFileMap[cid][key] == 2 {
					ac.PUFMMutex.RUnlock()
					break
				}
			}
			ac.PUFMMutex.RUnlock()
		}
		ac.PFRMMutex.RLock()
		pendingroot := ac.PendingFileRootMap[cid][key]
		pendingversion := ac.PendingFileVMap[cid][key]
		ac.PFRMMutex.RUnlock()
		// 验证根哈希的一致性
		ac.FRRMMutex.Lock()
		if bytes.Equal(req.Rootmap[key], pendingroot) {
			//如果此时开启审计，则把历史版本和所有更新版本存下来
			ac.IAMutex.RLock()
			isa := ac.IsAudit
			ac.IAMutex.RUnlock()
			if isa {
				cidfni := cid + "-" + key
				oldV := ac.FileVersionMap[cid][key]
				ac.MFRRMutex.Lock()
				if ac.MulVFileRootMap[cidfni] == nil {
					ac.MulVFileRootMap[cidfni] = make(map[string][]byte)
				}
				ac.MulVFileRootMap[cidfni][strconv.Itoa(oldV)] = ac.FileRootMap[cid][key]
				if ac.MulVFileRandMap[cidfni] == nil {
					ac.MulVFileRandMap[cidfni] = make(map[string][]int32)
				}
				ac.MulVFileRandMap[cidfni][strconv.Itoa(oldV)] = ac.FileRandMap[cid][key]
				ac.MFRRMutex.Unlock()
			}
			// 保存根节点哈希和随机数集
			ac.FileRootMap[cid][key] = req.Rootmap[key]
			ac.FileRandMap[cid][key] = make([]int32, 0)
			for i := 0; i < len(req.Randmap[key].Values); i++ {
				ac.FileRandMap[cid][key] = append(ac.FileRandMap[cid][key], req.Randmap[key].Values[i])
			}
			ac.FileVersionMap[cid][key] = pendingversion
			// 删除pendinglist中该key对应的根哈希
			ac.PFRMMutex.Lock()
			delete(ac.PendingFileRootMap[cid], key)
			delete(ac.PendingFileVMap[cid], key)
			ac.PFRMMutex.Unlock()
		} else {
			// 不一致则返回错误
			ac.FRRMMutex.Unlock()
			message = key + " update error"
			e := errors.New("root hash not consist")
			return &pb.StorjUFCResponse{Filename: req.Filename, Message: message}, e
		}
		ac.FRRMMutex.Unlock()
	}
	message = req.Filename + " complete update"
	return &pb.StorjUFCResponse{Filename: req.Filename, Message: message}, nil
}

// 【在生成Auditor对象时启动】审计方每隔sleepSeconds秒对每个文件的副本进行审计
func (ac *StorjAC) KeepAuditing(sleepSeconds int) {
	time.Sleep(time.Duration(sleepSeconds) * time.Second)
	auditNo := 0
	for {
		// 构建每个存储节点上的审计文件表
		snfnimap := make(map[string][]string)          //key:snid,value:cid-fn-i
		snfnivmap := make(map[string]map[string]int32) //key:snid,subkey:cid-fn-i,subvalue:version
		var snfnivmutex sync.RWMutex
		snfnirandmap := make(map[string]map[string]*pb.Int32Array) //key:snid,subkey:cid-fn-i,subvalue:rands
		snfnirootmap := make(map[string]map[string][]byte)         //key:snid,subkey:cid-fn-i,subvalue:root哈希
		var snfnirmutex sync.RWMutex
		ac.FRRMMutex.RLock()
		ac.CFRMMutex.RLock()
		for cid, fnisnmap := range ac.ClientFileRepMap {
			for fni, snid := range fnisnmap {
				cid_fni := cid + "-" + fni
				if snfnimap[snid] == nil {
					snfnimap[snid] = make([]string, 0)
				}
				snfnimap[snid] = append(snfnimap[snid], cid_fni)
				if snfnivmap[snid] == nil {
					snfnivmap[snid] = make(map[string]int32)
				}
				snfnivmap[snid][cid_fni] = int32(ac.FileVersionMap[cid][fni])
				// 将文件版本号替换为待更新版本
				ac.PUFMMutex.RLock()
				if ac.PendingUpdateFileVersionMap[cid][fni] != 0 {
					snfnivmap[snid][cid_fni] = int32(ac.PendingUpdateFileVersionMap[cid][fni])
				}
				ac.PUFMMutex.RUnlock()
			}
		}
		ac.CFRMMutex.RUnlock()
		ac.FRRMMutex.RUnlock()
		//启动审计，即启动记录多版本
		ac.IAMutex.Lock()
		ac.IsAudit = true
		log.Println("start auditing:", auditNo)
		ac.IAMutex.Unlock()
		// 对每个存储节点进行审计
		done := make(chan struct{})
		for snid, _ := range snfnimap {
			go func(snId string) {
				// log.Println(snId, "len(snfnimap[snId]):", len(snfnimap[snId]))
				//构造预审计请求消息(每3000个cdifnis为一个RPC)
				auditNostr := "audit-" + strconv.Itoa(auditNo)
				prea_rpc_num := (len(snfnimap[snId]) / 3000) + 1
				for i := 0; i < prea_rpc_num; i++ {
					start := i * 3000
					var end int
					if start+3000 > len(snfnimap[snId]) {
						// 如果从start开始的3000个元素超出数组长度，只取剩余的元素
						end = len(snfnimap[snId])
					} else {
						end = start + 3000
					}
					//构造Cidfniv的子map
					snfnivmutex.RLock()
					subcidfniv := make(map[string]int32)
					for j := start; j < end; j++ {
						subcidfniv[snfnimap[snId][j]] = snfnivmap[snId][snfnimap[snId][j]]
					}
					snfnivmutex.RUnlock()
					prea_req := &pb.StorjPASNRequest{Auditno: auditNostr, Snid: snId, Cidfnis: snfnimap[snId][start:end], Cidfniv: subcidfniv, Totalrpcs: int32(prea_rpc_num), Currpcno: int32(i)}
					//发送预审计请求
					prea_res, err := ac.SNRPCs[snId].StorjPreAuditSN(context.Background(), prea_req)
					if err != nil {
						log.Fatalf(snId, "storagenode could not process preaudit request: %v", err)
					}
					//如果未准备好，则更新待审计版本与sn中审计快照一致
					if !prea_res.Isready {
						for cid_fni, v := range prea_res.Fversion {
							snfnivmutex.Lock()
							snfnivmap[snId][cid_fni] = v
							snfnivmutex.Unlock()
						}
					}
				}
				// 根据多版本表构建挑战随机数表
				snfnivmutex.RLock()
				cidfnivmap := snfnivmap[snId]
				snfnivmutex.RUnlock()
				for cidfni, version := range cidfnivmap {
					cidfnisplit := strings.Split(cidfni, "-")
					fni := cidfnisplit[1] + "-" + cidfnisplit[2]
					//如果当前版本等于version，则用当前版本的随机数
					//如果当前版本大于version，则去多版本表中找对应版本
					//如果当前版本小于version，则需等待完成更新
					ac.FRRMMutex.RLock()
					currV := ac.FileVersionMap[cidfnisplit[0]][fni]
					ac.FRRMMutex.RUnlock()
					if currV == int(version) {
						snfnirmutex.Lock()
						if snfnirandmap[snId] == nil {
							snfnirandmap[snId] = make(map[string]*pb.Int32Array)
						}
						if snfnirootmap[snId] == nil {
							snfnirootmap[snId] = make(map[string][]byte)
						}
						ac.FRRMMutex.RLock()
						snfnirandmap[snId][cidfni] = &pb.Int32Array{Values: ac.FileRandMap[cidfnisplit[0]][fni]}
						snfnirootmap[snId][cidfni] = ac.FileRootMap[cidfnisplit[0]][fni]
						ac.FRRMMutex.RUnlock()
						snfnirmutex.Unlock()
					} else if currV > int(version) {
						snfnirmutex.Lock()
						if snfnirandmap[snId] == nil {
							snfnirandmap[snId] = make(map[string]*pb.Int32Array)
						}
						if snfnirootmap[snId] == nil {
							snfnirootmap[snId] = make(map[string][]byte)
						}
						ac.MFRRMutex.RLock()
						snfnirandmap[snId][cidfni] = &pb.Int32Array{Values: ac.MulVFileRandMap[cidfni][strconv.Itoa(int(version))]}
						snfnirootmap[snId][cidfni] = ac.MulVFileRootMap[cidfni][strconv.Itoa(int(version))]
						ac.MFRRMutex.RUnlock()
						snfnirmutex.Unlock()
					} else {
						for {
							ac.FRRMMutex.RLock()
							currV = ac.FileVersionMap[cidfnisplit[0]][fni]
							ac.FRRMMutex.RUnlock()
							if currV == int(version) {
								snfnirmutex.Lock()
								if snfnirandmap[snId] == nil {
									snfnirandmap[snId] = make(map[string]*pb.Int32Array)
								}
								if snfnirootmap[snId] == nil {
									snfnirootmap[snId] = make(map[string][]byte)
								}
								ac.FRRMMutex.RLock()
								snfnirandmap[snId][cidfni] = &pb.Int32Array{Values: ac.FileRandMap[cidfnisplit[0]][fni]}
								snfnirootmap[snId][cidfni] = ac.FileRootMap[cidfnisplit[0]][fni]
								ac.FRRMMutex.RUnlock()
								snfnirmutex.Unlock()
								break
							}
						}
					}
				}
				// 发起审计(每3000个cdifnis为一个RPC)
				for i := 0; i < prea_rpc_num; i++ {
					start := i * 3000
					var end int
					if start+3000 > len(snfnimap[snId]) {
						// 如果从start开始的3000个元素超出数组长度，只取剩余的元素
						end = len(snfnimap[snId])
					} else {
						end = start + 3000
					}
					//构造Cidfnirands的子map
					snfnirmutex.RLock()
					subcidfnirands := make(map[string]*pb.Int32Array)
					for j := start; j < end; j++ {
						subcidfnirands[snfnimap[snId][j]] = snfnirandmap[snId][snfnimap[snId][j]]
					}
					snfnirmutex.RUnlock()
					gapos_req := &pb.StorjGAPSNRequest{Auditno: auditNostr, Cidfnirands: subcidfnirands, Totalrpcs: int32(prea_rpc_num), Currpcno: int32(i)}
					//RPC获取存储证明
					gapos_res, err := ac.SNRPCs[snId].StorjGetPosSN(context.Background(), gapos_req)
					if err != nil {
						log.Fatalf("storagenode could not process request: %v", err)
					}
					//验证存储证明
					for cidfni, preleafsArray := range gapos_res.Preleafs {
						preleafs := make([][]byte, 0)
						for i := 0; i < len(preleafsArray.Values); i++ {
							preleafs = append(preleafs, preleafsArray.Values[i])
						}
						builtRoot := util.GenerateMTRootByPreleafs(preleafs)
						snfnirmutex.RLock()
						currRoot := snfnirootmap[snId][cidfni]
						snfnirmutex.RUnlock()
						if !bytes.Equal(builtRoot, currRoot) {
							log.Println("auditing", snId, cidfni, "error: root not consist")
						}
					}
				}
				// 通知主线程任务完成
				done <- struct{}{}
			}(snid)
		}
		// 等待所有协程完成
		for i := 0; i < len(snfnimap); i++ {
			<-done
		}
		//关闭审计
		ac.IAMutex.Lock()
		ac.IsAudit = false
		log.Println("end auditing:", auditNo)
		ac.IAMutex.Unlock()
		auditNo++
		//清空多版本元信息列表
		ac.MFRRMutex.Lock()
		ac.MulVFileRandMap = make(map[string]map[string][]int32)
		ac.MulVFileRootMap = make(map[string]map[string][]byte)
		ac.MFRRMutex.Unlock()
		time.Sleep(time.Duration(sleepSeconds) * time.Second)
	}
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
