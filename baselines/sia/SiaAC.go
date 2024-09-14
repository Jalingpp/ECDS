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
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type SiaAC struct {
	IpAddr                             string                             //审计方的IP地址
	SNAddrMap                          map[string]string                  //存储节点的地址表，key:存储节点id，value:存储节点地址
	ClientDSSNMap                      map[string]map[string]string       //客户端文件每个分片所在的存储节点表，key:clientID-filename,subkey:dsno,subvalue:snid
	CDSSNMMutex                        sync.RWMutex                       //ClientDSSNMap的读写锁
	ClientSNRootMap                    map[string]map[string][]byte       //客户端所在存储节点上最新的Merkel根节点哈希，key:clientID,subkey:snid,subvalue:根节点哈希值
	ClientDSVersionMap                 map[string]int                     //当前文件版本号,key:clientID-filename-dsno,value:版本号
	FRRMMutex                          sync.RWMutex                       //ClientDSVersionMap的读写锁
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
	PendingUpdateClientRootMap         map[string][]byte                  //待更新的存储节点传回的根节点哈希，key:clientID-filename-i,value:根节点哈希值
	PendingUpdateFileSNMap             map[string]string                  //待更新的文件分片所在的存储节点，key:clientID-filename-i,value:snid
	PendingUpdateDSMerklePathMap       map[string][][]byte                //待更新的文件分片的有效性证明，key:clientID-filename-i,value:merkle path
	PendingUpdateDSIndexMap            map[string]int                     //待更新的文件分片在Merkel树叶子列表中的索引号，key:clientID-filename-i,value:索引号
	PUFMMutex                          sync.RWMutex                       //PendingUpdateMap的读写锁
	IsAudit                            bool                               //是否开始审计的标识
	IAMutex                            sync.RWMutex                       //IsAudit的读写锁
	MulVFileRootMap                    map[string]map[string][]byte       //多版本文件根节点哈希值表，key:cid-fn-i,subkey:版本号,subvalue:根节点哈希值
	MulVFileRandMap                    map[string]map[string][]int32      //多版本文件随机数组表，key:cid-fn-i,subkey:版本号,subvalue:随机数组
	MFRRMutex                          sync.RWMutex                       //MulVFileRootMap,MulVFileRandMap的读写锁
}

// 新建一个审计方，持续监听消息
func NewSiaAC(ipaddr string, snaddrfilename string, dn int, pn int) *SiaAC {
	snaddrmap := util.ReadSNAddrFile(snaddrfilename)
	cdssnmap := make(map[string]map[string]string)
	csnrmap := make(map[string]map[string][]byte)
	cdsvmap := make(map[string]int)
	pcrmap := make(map[string][]byte)
	pfsnmap := make(map[string]string)
	pdsmpmap := make(map[string][][]byte)
	pdsimap := make(map[string]int)
	pucrmap := make(map[string][]byte)
	pufsnmap := make(map[string]string)
	pudsmpmap := make(map[string][][]byte)
	pudsimap := make(map[string]int)
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
	auditor := &SiaAC{ipaddr, *snaddrmap, cdssnmap, sync.RWMutex{}, csnrmap, cdsvmap, sync.RWMutex{}, dn, pn, pb.UnimplementedSiaACServiceServer{}, snrpcs, sync.RWMutex{}, pcrmap, pfsnmap, pdsmpmap, pdsimap, sync.RWMutex{}, pucrmap, pufsnmap, pudsmpmap, pudsimap, sync.RWMutex{}, false, sync.RWMutex{}, mvfrtmap, mvfrdmap, sync.RWMutex{}}
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
	cid := req.ClientId
	fn := req.Filename
	cid_fn := cid + "-" + fn
	dshashMap := req.Dshashmap
	isValid := true
	// 遍历收到的每个分片的哈希值，验证Merkel路径是否正确
	for key, dshash := range dshashMap {
		var snid string
		// 等待获取存储节点传来的存储回复
		for {
			ac.PFRMMutex.RLock()
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
			//记录分片所在的存储节点
			dsno := strings.TrimPrefix(key, cid_fn+"-")
			ac.CDSSNMMutex.Lock()
			if ac.ClientDSSNMap[cid_fn] == nil {
				ac.ClientDSSNMap[cid_fn] = make(map[string]string)
			}
			ac.ClientDSSNMap[cid_fn][dsno] = snid
			ac.CDSSNMMutex.Unlock()
			//记录最新的根节点哈希
			ac.FRRMMutex.Lock()
			if ac.ClientSNRootMap[cid] == nil {
				ac.ClientSNRootMap[cid] = make(map[string][]byte)
			}
			ac.ClientDSVersionMap[key] = 1
			ac.ClientSNRootMap[cid][snid] = pendingRoot
			ac.FRRMMutex.Unlock()
			//删除pending列表中的相关记录
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
		fmt.Println(cid, fn, "Merkle Path Verify:", isValid)
		message = "OK"
	}
	return &pb.SiaPFCResponse{Filename: fn, Message: message}, nil
}

// 【供client使用的RPC】获取文件数据分片所在的存储节点id
func (ac *SiaAC) SiaGetFileSNs(ctx context.Context, req *pb.SiaGFACRequest) (*pb.SiaGFACResponse, error) {
	cid_fn := req.ClientId + "-" + req.Filename
	vmap := make(map[string]int32)
	snsds := make(map[string]string)
	snroots := make(map[string][]byte)
	//为客户端找到文件的所有数据分片对应的存储节点
	ac.CDSSNMMutex.RLock()
	if ac.ClientDSSNMap[cid_fn] == nil {
		ac.CDSSNMMutex.RUnlock()
		e := errors.New("client filename not exist")
		return nil, e
	} else {
		for key, value := range ac.ClientDSSNMap[cid_fn] {
			if strings.HasPrefix(key, "d") {
				snsds[key] = value
				cid_fn_dsno := cid_fn + "-" + key
				//为客户端找到该存储节点对应的Merkel根节点哈希值
				ac.FRRMMutex.RLock()
				snroots[value] = ac.ClientSNRootMap[req.ClientId][value]
				vmap[cid_fn_dsno] = int32(ac.ClientDSVersionMap[cid_fn_dsno])
				ac.FRRMMutex.RUnlock()
			}
		}
		ac.CDSSNMMutex.RUnlock()
	}
	return &pb.SiaGFACResponse{Filename: req.Filename, Versions: vmap, Snsds: snsds, Roots: snroots}, nil
}

// 【供client使用的RPC】报告获取DS错误，并请求获取校验块所在的存储节点id
func (ac *SiaAC) SiaGetDSErrReport(ctx context.Context, req *pb.SiaGDSERequest) (*pb.SiaGDSEResponse, error) {
	cid_fn := req.ClientId + "-" + req.Filename
	blacksns := req.Blacksns
	dsnosnmap := make(map[string]string)
	snroots := make(map[string][]byte)
	for key, _ := range req.Errdssn {
		//获取待请求校验块的前缀
		targetPrefix := ""
		if len(key) < 5 {
			targetPrefix = "p-"
		} else {
			for i := 0; i < len(req.Errdssn); i++ {
				if strings.HasPrefix(key, "d-"+strconv.Itoa(i+ac.DataNum)+"-") || strings.HasPrefix(key, "p-"+strconv.Itoa(i+ac.DataNum)+"-") {
					targetPrefix = "p-" + strconv.Itoa(i+ac.DataNum) + "-"
					break
				}
			}
		}
		//挑选带有相应前缀的校验块存储节点
		for i := 0; i < ac.ParityNum; i++ {
			psno := targetPrefix + strconv.Itoa(i)
			_, exists := dsnosnmap[psno]
			ac.CDSSNMMutex.RLock()
			snid := ac.ClientDSSNMap[cid_fn][psno]
			ac.CDSSNMMutex.RUnlock()
			if !exists && blacksns[snid] != "h" && blacksns[snid] != psno {
				dsnosnmap[psno] = snid
				//获取客户端在该存储节点上的Merkel根节点哈希值
				ac.FRRMMutex.RLock()
				snroots[snid] = ac.ClientSNRootMap[cid_fn][snid]
				ac.FRRMMutex.RUnlock()
				break
			}
		}
	}
	return &pb.SiaGDSEResponse{Filename: req.Filename, Snsds: dsnosnmap}, nil
}

// 【供client使用的RPC】获取Dsno和所有校验块所在的存储节点id及其当前版本号
func (ac *SiaAC) SiaUpdateFileReq(ctx context.Context, req *pb.SiaUFRequest) (*pb.SiaUFResponse, error) {
	cid_fn := req.Clientid + "-" + req.Filename
	dsno := req.Dsno
	dssn := ""
	dsv := 0
	pssns := make(map[string]string)
	psvs := make(map[string]int32)
	ac.CDSSNMMutex.RLock()
	if ac.ClientDSSNMap[cid_fn] == nil {
		ac.CDSSNMMutex.RUnlock()
		e := errors.New("client filename not exist")
		return nil, e
	} else if ac.ClientDSSNMap[cid_fn][dsno] == "" {
		ac.CDSSNMMutex.RUnlock()
		e := errors.New("datashard not exist")
		return nil, e
	} else {
		dssn = ac.ClientDSSNMap[cid_fn][dsno]
		ac.FRRMMutex.RLock()
		dsv = ac.ClientDSVersionMap[cid_fn+"-"+dsno]
		ac.FRRMMutex.RUnlock()
		//获取所有校验块所在的存储节点id
		for key, value := range ac.ClientDSSNMap[cid_fn] {
			if strings.HasPrefix(key, "p") {
				cid_fn_psno := cid_fn + "-" + key
				pssns[cid_fn_psno] = value
				ac.FRRMMutex.RLock()
				psvs[cid_fn_psno] = int32(ac.ClientDSVersionMap[cid_fn_psno])
				ac.FRRMMutex.RUnlock()
			}
		}
		ac.CDSSNMMutex.RUnlock()
	}
	go ac.SiaUpdateFileNoticeToSN(req.Clientid, req.Filename, dsno, dssn, dsv, pssns, psvs)
	return &pb.SiaUFResponse{Filename: req.Filename, Dssn: dssn, Dsversion: int32(dsv), Paritysns: pssns, Parityversions: psvs}, nil
}

// 【SelectSNs-RPC被调用时自动触发】通知存储节点客户端待存放的文件分片序号，等待SN存储结果
func (ac *SiaAC) SiaUpdateFileNoticeToSN(cid string, fn string, dsno string, dssn string, dsv int, pssnids map[string]string, psvs map[string]int32) {
	//构造请求消息
	uds_req := &pb.SiaClientUpdDSRequest{
		ClientId: cid,
		Filename: fn,
		Dsno:     dsno,
	}
	//发送请求消息给存储节点
	uds_res, err := ac.SNRPCs[dssn].SiaUpdateDataShardNotice(context.Background(), uds_req)
	if err != nil {
		log.Fatalf("storagenode could not process request: %v", err)
	}
	//处理存储节点回复消息：
	//1-将存储节点放入待确认列表中
	cid_fn := uds_res.ClientId + "-" + uds_res.Filename
	ac.PUFMMutex.Lock()
	ac.PendingUpdateFileSNMap[cid_fn+"-"+dsno] = dssn
	ac.PendingUpdateClientRootMap[cid_fn+"-"+dsno] = uds_res.Root
	ac.PendingUpdateDSMerklePathMap[cid_fn+"-"+dsno] = uds_res.Merklepath
	ac.PendingUpdateDSIndexMap[cid_fn+"-"+dsno] = int(uds_res.Index)
	ac.PUFMMutex.Unlock()

	for key, value := range pssnids {
		psno := strings.TrimPrefix(key, cid_fn+"-")
		go func(snid string, psno string, cid_fni string) {
			//构造请求消息
			ups_req := &pb.SiaClientUpdDSRequest{
				ClientId: cid,
				Filename: fn,
				Dsno:     psno,
			}
			//发送请求消息给存储节点
			ups_res, err := ac.SNRPCs[snid].SiaUpdateDataShardNotice(context.Background(), ups_req)
			if err != nil {
				log.Fatalf("storagenode could not process request: %v", err)
			}
			//处理存储节点回复消息：
			//1-将存储节点放入待确认列表中
			cid_fn := ups_res.ClientId + "-" + ups_res.Filename
			dsno := ups_res.Dsno
			ac.PUFMMutex.Lock()
			ac.PendingUpdateFileSNMap[cid_fn+"-"+dsno] = snid
			ac.PendingUpdateClientRootMap[cid_fn+"-"+dsno] = ups_res.Root
			ac.PendingUpdateDSMerklePathMap[cid_fn+"-"+dsno] = ups_res.Merklepath
			ac.PendingUpdateDSIndexMap[cid_fn+"-"+dsno] = int(ups_res.Index)
			ac.PUFMMutex.Unlock()
		}(value, psno, key)
	}
}

// 【供client使用的RPC】文件存放确认:确认文件已在存储节点上完成存放,确认元信息一致
func (ac *SiaAC) SiaUpdateFileCommit(ctx context.Context, req *pb.SiaUFCRequest) (*pb.SiaUFCResponse, error) {
	message := ""
	cid := req.ClientId
	fn := req.Filename
	cid_fn := cid + "-" + fn
	dshashMap := req.Dshashmap
	isValid := true
	// 遍历收到的每个分片的哈希值，验证Merkel路径是否正确
	for key, dshash := range dshashMap {
		var snid string
		// 等待获取存储节点传来的存储回复
		for {
			ac.PUFMMutex.RLock()
			if ac.PendingUpdateFileSNMap[key] != "" {
				snid = ac.PendingUpdateFileSNMap[key]
				ac.PUFMMutex.RUnlock()
				break
			}
			ac.PUFMMutex.RUnlock()
		}
		//获取根节点哈希，Merkle路径，索引号
		ac.PUFMMutex.RLock()
		pendingRoot := ac.PendingUpdateClientRootMap[key]
		pendingMerklePath := ac.PendingUpdateDSMerklePathMap[key]
		pendingIndex := ac.PendingUpdateDSIndexMap[key]
		ac.PUFMMutex.RUnlock()
		// 验证Merkle路径的有效性
		newRoot := util.GenerateRootByPaths(dshash, pendingIndex, pendingMerklePath)
		if !bytes.Equal(newRoot, pendingRoot) {
			isValid = false
			message = "Faild"
			e := errors.New("merkle path verify failed")
			return nil, e
		} else {
			//验证有效，则将信息永久记录，并在pending列表中删除
			//记录分片所在的存储节点
			dsno := strings.TrimPrefix(key, cid_fn+"-")
			ac.CDSSNMMutex.Lock()
			if ac.ClientDSSNMap[cid_fn] == nil {
				ac.ClientDSSNMap[cid_fn] = make(map[string]string)
			}
			ac.ClientDSSNMap[cid_fn][dsno] = snid
			ac.CDSSNMMutex.Unlock()
			//记录最新的根节点哈希
			ac.FRRMMutex.Lock()
			if ac.ClientSNRootMap[cid] == nil {
				ac.ClientSNRootMap[cid] = make(map[string][]byte)
			}
			ac.ClientDSVersionMap[key] = 1
			ac.ClientSNRootMap[cid][snid] = pendingRoot
			ac.FRRMMutex.Unlock()
			//删除pending列表中的相关记录
			ac.PUFMMutex.Lock()
			delete(ac.PendingUpdateFileSNMap, key)
			delete(ac.PendingUpdateDSMerklePathMap, key)
			delete(ac.PendingUpdateDSIndexMap, key)
			delete(ac.PendingUpdateClientRootMap, key)
			ac.PUFMMutex.Unlock()
		}
	}
	if isValid {
		fmt.Println(cid, fn, "Merkle Path Verify:", isValid)
		message = "OK"
	}
	return &pb.SiaUFCResponse{Filename: fn, Message: message}, nil
}
