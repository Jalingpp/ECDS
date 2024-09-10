package baselines

import (
	pb "ECDS/proto" // 根据实际路径修改"
	"ECDS/util"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	prooftypes "github.com/filecoin-project/go-state-types/proof"
	"github.com/ipfs/go-cid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type FilecoinAC struct {
	IpAddr                                  string                                        //审计方的IP地址
	SNAddrMap                               map[string]string                             //存储节点的地址表，key:存储节点id，value:存储节点地址
	F                                       int                                           //阈值
	ClientFileRepSNMap                      map[string]string                             //客户端文件每个备份所在的存储节点表，key:客户端cid-filename-i,value:snid
	ClientFileVersionMap                    map[string]int                                //客户端文件对应的版本号，key:cid-filename,value:versionno
	ClientFileRepMinerMap                   map[string]abi.ActorID                        //客户端文件每个副本对应的MinerID，key:cid-filename-i,value:minerID
	ClientFileRepSectorInforMap             map[string]map[int]*SectorSealedInfor         //客户端文件每个副本所在扇区对应的封装ID，key:cid-filename-i,subkey:sectorNum,subvalue:sealedCID
	CFRMMutex                               sync.RWMutex                                  //ClientFileRepSNMap和ClientFileRepSectorInforMap的读写锁
	pb.UnimplementedFilecoinACServiceServer                                               // 嵌入匿名字段
	SNRPCs                                  map[string]pb.FilecoinSNACServiceClient       //存储节点RPC对象列表，key:存储节点id
	SNRPCMutex                              sync.RWMutex                                  //SNRPCs的读写锁
	SNpointer                               int                                           //指示当前分派的存储节点索引号，用于均匀选择（2f+1）个存储节点
	SNPMutex                                sync.RWMutex                                  //SNpointer的写锁
	PendingClientFileRepSNMap               map[string]string                             //待插入的客户端文件每个备份所在的存储节点表，key:客户端cid-filename-i,value:snid
	PendingClientFileRepMinerMap            map[string]abi.ActorID                        //待插入的客户端文件每个副本对应的MinerID，key:cid-filename-i,value:minerID
	PendingClientFileRepSectorInforMap      map[string]map[int]*SectorSealedInfor         //待插入的客户端文件每个副本所在扇区对应的封装ID，key:cid-filename-i,subkey:sectorNum,subvalue:sealedCID
	PCFRMMutex                              sync.RWMutex                                  //PClientFileRepSNMap和PClientFileRepSectorInforMap的读写锁
	PendingUpdClientFileRepSectorInforMap   map[string]map[int]*SectorSealedInfor         //待更新的客户端文件每个副本所在扇区对应的封装ID，key:cid-filename-i,subkey:sectorNum,subvalue:sealedCID
	PendingUpdateFileVersionMap             map[string]int                                //待更新的客户端文件版本号，key:cid-filename-i,value:version
	PUCFRMMutex                             sync.RWMutex                                  //PUpdClientFileRepSNMap和PUpdClientFileRepSectorInforMap的读写锁
	SealProofType                           abi.RegisteredSealProof                       //Filecoin sealproof类型
	Ticket                                  abi.SealRandomness                            //Filecoin存储证明票根
	Seed                                    abi.InteractiveSealRandomness                 //Filecoin存储证明种子
	IsAudit                                 bool                                          //审计开始标记
	IAMutex                                 sync.RWMutex                                  //IsAudit的读写锁
	MulVFileSectorInforMap                  map[string]map[int]map[int]*SectorSealedInfor //多版本文件扇区信息表，key:cid-fn-i,subkey:版本号,subvalue:扇区信息表
	MFRRMutex                               sync.RWMutex                                  //MulVFileSectorInforMap的读写锁
}

// 新建一个审计方，持续监听消息
func NewFilecoinAC(ipaddr string, snaddrfilename string, f int) *FilecoinAC {
	snaddrmap := util.ReadSNAddrFile(snaddrfilename)
	cfrsnmap := make(map[string]string)
	cfvmap := make(map[string]int)
	cfrmidmap := make(map[string]abi.ActorID)
	cfrsimap := make(map[string]map[int]*SectorSealedInfor)
	//设置连接存储节点服务器的地址
	snrpcs := make(map[string]pb.FilecoinSNACServiceClient)
	for key, value := range *snaddrmap {
		snconn, err := grpc.Dial(value, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
		if err != nil {
			log.Fatalf("did not connect to storage node: %v", err)
		}
		sc := pb.NewFilecoinSNACServiceClient(snconn)
		snrpcs[key] = sc
	}
	pcfrsnmap := make(map[string]string)
	pcfrmidmap := make(map[string]abi.ActorID)
	pcfrsimap := make(map[string]map[int]*SectorSealedInfor)
	pucfrsimap := make(map[string]map[int]*SectorSealedInfor)
	pucfvmap := make(map[string]int)
	sealProofType := abi.RegisteredSealProof_StackedDrg2KiBV1
	ticket := abi.SealRandomness{5, 4, 2}
	seed := abi.InteractiveSealRandomness{7, 4, 2}
	mvfsimap := make(map[string]map[int]map[int]*SectorSealedInfor)
	auditor := &FilecoinAC{ipaddr, *snaddrmap, f, cfrsnmap, cfvmap, cfrmidmap, cfrsimap, sync.RWMutex{}, pb.UnimplementedFilecoinACServiceServer{}, snrpcs, sync.RWMutex{}, 1, sync.RWMutex{}, pcfrsnmap, pcfrmidmap, pcfrsimap, sync.RWMutex{}, pucfrsimap, pucfvmap, sync.RWMutex{}, sealProofType, ticket, seed, false, sync.RWMutex{}, mvfsimap, sync.RWMutex{}}
	//设置监听地址
	lis, err := net.Listen("tcp", ipaddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterFilecoinACServiceServer(s, auditor)
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
func (ac *FilecoinAC) FilecoinSelectSNs(ctx context.Context, sreq *pb.FilecoinStorageRequest) (*pb.FilecoinStorageResponse, error) {
	// log.Printf("Received storage message from client: %s\n", sreq.ClientId)
	// 为该客户端文件随机选择存储节点
	snids := ac.FilecoinSelectStorNodes(sreq.Filename)
	//启动线程通知各存储节点待存储的文件分片序号,等待存储完成
	go ac.FilecoinPutFileNoticeToSN(sreq.ClientId, sreq.Filename, snids)
	return &pb.FilecoinStorageResponse{Filename: sreq.Filename, SnsForFd: snids}, nil
}

// AC选择(2f+1)个存储节点
func (ac *FilecoinAC) FilecoinSelectStorNodes(filename string) []string {
	snids := make([]string, 0)
	snnum := ac.F*2 + 1
	ac.SNPMutex.Lock()
	snp := ac.SNpointer
	for i := 0; i < snnum; i++ {
		snp = ac.SNpointer + i
		if snp > ac.F*3+1 {
			snp = snp - ac.F*3 - 1
		}
		snids = append(snids, "sn"+strconv.Itoa(snp))
	}
	ac.SNpointer = snp
	ac.SNPMutex.Unlock()
	return snids
}

// 【FilecoinSelectSNs-RPC被调用时自动触发】通知存储节点客户端待存放的文件分片序号，等待SN存储结果
func (ac *FilecoinAC) FilecoinPutFileNoticeToSN(cid string, fn string, snids []string) {
	for i := 0; i < len(snids); i++ {
		go func(snid string, i int) {
			//构造请求消息
			pds_req := &pb.FilecoinClientStorageRequest{
				ClientId: cid,
				Filename: fn,
				Repno:    int32(i),
				Version:  int32(1),
			}
			//发送请求消息给存储节点
			pds_res, err := ac.SNRPCs[snid].FilecoinPutFileNotice(context.Background(), pds_req)
			if err != nil {
				log.Println("storagenode could not process request:", err)
			}
			//处理存储节点回复消息：验证扇区封装证明，加入到待存储扇区信息表
			minerID := abi.ActorID(pds_res.MinerID)
			fileRepSectorSealedInfor := MessageToSectorSealedInfor(pds_res.SectorNum, pds_res.SealedCID, pds_res.UnsealedCID, pds_res.Proof)
			cid_fn := pds_res.ClientId + "-" + pds_res.Filename + "-" + strconv.Itoa(int(pds_res.Repno))
			ac.PCFRMMutex.Lock()
			ac.PendingClientFileRepSNMap[cid_fn] = pds_res.Snid
			ac.PendingClientFileRepMinerMap[cid_fn] = minerID
			ac.PendingClientFileRepSectorInforMap[cid_fn] = fileRepSectorSealedInfor
			ac.PCFRMMutex.Unlock()
		}(snids[i], i)
	}
}

func MessageToSectorSealedInfor(sectorNumList []int32, sealedCIDList []string, unsealedCIDList []string, proofList [][]byte) map[int]*SectorSealedInfor {
	fileRepSectorSealedInfor := make(map[int]*SectorSealedInfor)
	for i := 0; i < len(sectorNumList); i++ {
		var c cid.Cid
		var err error
		if c, err = cid.Decode(sealedCIDList[i]); err != nil {
			fmt.Println("Error decoding CID:", err)
			return nil
		}
		var uc cid.Cid
		if uc, err = cid.Decode(unsealedCIDList[i]); err != nil {
			fmt.Println("Error decoding CID:", err)
			return nil
		}
		ssi := &SectorSealedInfor{abi.SectorNumber(sectorNumList[i]), c, uc, proofList[i]}
		fileRepSectorSealedInfor[int(sectorNumList[i])] = ssi
	}
	return fileRepSectorSealedInfor
}

// 【供client使用的RPC】文件存放确认:确认文件已在存储节点上完成存放,确认元信息一致
func (ac *FilecoinAC) FilecoinPutFileCommit(ctx context.Context, req *pb.FilecoinPFCRequest) (*pb.FilecoinPFCResponse, error) {
	message := ""
	cid := req.ClientId
	fn := req.Filename
	// 遍历客户端文件所有副本对应的扇区信息，验证后返回给客户端
	for i := 0; i < 2*ac.F+1; i++ {
		cid_fn := cid + "-" + fn + "-" + strconv.Itoa(i)
		fileRepSectorSealedInfor := ac.PendingClientFileRepSectorInforMap[cid_fn]
		verified := true
		for _, value := range fileRepSectorSealedInfor {
			ac.PCFRMMutex.RLock()
			miner := ac.PendingClientFileRepMinerMap[cid_fn]
			ac.PCFRMMutex.RUnlock()
			// // 验证封装证明
			isValid, err := ffi.VerifySeal(prooftypes.SealVerifyInfo{
				SectorID: abi.SectorID{
					Miner:  miner,
					Number: value.SectorNum,
				},
				SealedCID:             value.SealedCID,
				SealProof:             ac.SealProofType,
				Proof:                 value.Proof,
				DealIDs:               []abi.DealID{},
				Randomness:            ac.Ticket,
				InteractiveRandomness: ac.Seed,
				UnsealedCID:           value.UnsealedCID,
			})
			if err != nil {
				fmt.Println("VerifySeal", err.Error())
			}
			log.Println(cid_fn, "verify seal sector", value.SectorNum, ":", isValid)
			//如果有一个sector未通过验证，则全部置为未通过
			if !isValid {
				verified = false
			}
		}
		//如果全部通过验证，则存放扇区信息
		if verified {
			ac.CFRMMutex.Lock()
			ac.ClientFileRepSNMap[cid_fn] = ac.PendingClientFileRepSNMap[cid_fn]
			ac.ClientFileVersionMap[req.ClientId+"-"+req.Filename] = 1
			ac.ClientFileRepMinerMap[cid_fn] = ac.PendingClientFileRepMinerMap[cid_fn]
			ac.ClientFileRepSectorInforMap[cid_fn] = fileRepSectorSealedInfor
			ac.CFRMMutex.Unlock()
			delete(ac.PendingClientFileRepSNMap, cid_fn)
			delete(ac.PendingClientFileRepMinerMap, cid_fn)
			delete(ac.PendingClientFileRepSectorInforMap, cid_fn)
		} else {
			fmt.Println(cid_fn, "storage verified failed.")
			e := errors.New("storage verified failed")
			return nil, e
		}
	}
	message = req.Filename + " complete storage"
	return &pb.FilecoinPFCResponse{Filename: req.Filename, Message: message}, nil
}

// 【供client使用的RPC】获取文件数据分片所在的存储节点id
func (ac *FilecoinAC) FilecoinGetFileSNs(ctx context.Context, req *pb.FilecoinGFACRequest) (*pb.FilecoinGFACResponse, error) {
	cid_fn := req.ClientId + "-" + req.Filename
	snsds := make(map[string]string)
	version := 0
	//为客户端找到文件的所有副本对应的存储节点
	ac.CFRMMutex.RLock()
	if _, ok := ac.ClientFileRepSNMap[cid_fn+"-0"]; !ok {
		ac.CFRMMutex.RUnlock()
		e := errors.New("client filename not exist")
		return &pb.FilecoinGFACResponse{Filename: req.Filename, Version: int32(version), Snsds: snsds}, e
	} else {
		//遍历只返回以cid-filename-为前缀的
		for i := 0; i < 2*ac.F+1; i++ {
			key := cid_fn + "-" + strconv.Itoa(i)
			snsds[key] = ac.ClientFileRepSNMap[key]
		}
		version = ac.ClientFileVersionMap[cid_fn]
		ac.CFRMMutex.RUnlock()
	}
	return &pb.FilecoinGFACResponse{Filename: req.Filename, Version: int32(version), Snsds: snsds}, nil
}

// 【供client使用的RPC】更新存储文件请求，添加更新至待定区，返回文件副本所在的存储节点id
func (ac *FilecoinAC) FilecoinUpdateFileReq(ctx context.Context, req *pb.FilecoinUFRequest) (*pb.FilecoinUFResponse, error) {
	snsds := make(map[string]string)
	version := 0
	cid_fn := req.ClientId + "-" + req.Filename
	ac.CFRMMutex.RLock()
	if _, ok := ac.ClientFileVersionMap[cid_fn]; !ok {
		ac.CFRMMutex.RUnlock()
		e := errors.New("client file not exsist")
		return nil, e
	} else {
		version = ac.ClientFileVersionMap[cid_fn]
		//遍历只返回以filename为前缀的
		for i := 0; i < ac.F*2+1; i++ {
			key := cid_fn + "-" + strconv.Itoa(i)
			snsds[key] = ac.ClientFileRepSNMap[key]
		}
		ac.CFRMMutex.RUnlock()
	}
	//通知所有相关存储节点待更新的文件副本
	ac.PUCFRMMutex.Lock()
	ac.PendingUpdateFileVersionMap[cid_fn] = version + 1
	ac.PUCFRMMutex.Unlock()
	go ac.FilecoinUpdateFileNoticeToSN(req.ClientId, req.Filename, int32(version+1), snsds)
	return &pb.FilecoinUFResponse{Filename: req.Filename, Version: int32(version), Snsds: snsds}, nil
}

// 【FilecoinUpdateFileReq-RPC被调用时自动触发】通知存储节点客户端待更新的文件副本序号，等待SN更新结果
func (ac *FilecoinAC) FilecoinUpdateFileNoticeToSN(cid string, fn string, nv int32, snids map[string]string) {
	for fni, snid := range snids {
		splitfni := strings.Split(fni, "-")
		rep, _ := strconv.Atoi(splitfni[2])
		go func(snid string, rep int) {
			//构造请求消息
			ufn_req := &pb.FilecoinClientUFRequest{
				ClientId: cid,
				Filename: fn,
				Rep:      int32(rep),
				Version:  nv,
			}
			//发送请求消息给存储节点
			ufn_res, err := ac.SNRPCs[snid].FilecoinUpdateFileNotice(context.Background(), ufn_req)
			if err != nil {
				log.Println("storagenode could not process request:", err)
			}
			//处理存储节点回复消息：验证扇区封装证明，加入到待存储扇区信息表
			fileRepSectorSealedInfor := MessageToSectorSealedInfor(ufn_res.SectorNum, ufn_res.SealedCID, ufn_res.UnsealedCID, ufn_res.Proof)
			cid_fn := ufn_res.ClientId + "-" + ufn_res.Filename + "-" + strconv.Itoa(int(ufn_res.Repno))
			ac.PUCFRMMutex.Lock()
			ac.PendingUpdClientFileRepSectorInforMap[cid_fn] = fileRepSectorSealedInfor
			ac.PUCFRMMutex.Unlock()
		}(snid, rep)
	}
}

// 【供client使用的RPC】文件更新确认:确认文件已在存储节点上完成更新,确认元信息一致
func (ac *FilecoinAC) FilecoinUpdateFileCommit(ctx context.Context, req *pb.FilecoinUFCRequest) (*pb.FilecoinUFCResponse, error) {
	message := ""
	cid := req.ClientId
	fn := req.Filename
	// 遍历客户端文件所有副本对应的扇区信息，验证后返回给客户端
	for i := 0; i < 2*ac.F+1; i++ {
		cid_fn := cid + "-" + fn + "-" + strconv.Itoa(i)
		fileRepSectorSealedInfor := ac.PendingUpdClientFileRepSectorInforMap[cid_fn]
		ac.CFRMMutex.RLock()
		miner := ac.ClientFileRepMinerMap[cid_fn]
		ac.CFRMMutex.RUnlock()
		verified := true
		for _, value := range fileRepSectorSealedInfor {
			// // 验证封装证明
			isValid, err := ffi.VerifySeal(prooftypes.SealVerifyInfo{
				SectorID: abi.SectorID{
					Miner:  miner,
					Number: value.SectorNum,
				},
				SealedCID:             value.SealedCID,
				SealProof:             ac.SealProofType,
				Proof:                 value.Proof,
				DealIDs:               []abi.DealID{},
				Randomness:            ac.Ticket,
				InteractiveRandomness: ac.Seed,
				UnsealedCID:           value.UnsealedCID,
			})
			if err != nil {
				fmt.Println("VerifySeal", err.Error())
			}
			log.Println(cid_fn, "verify seal sector", value.SectorNum, ":", isValid)
			//如果有一个sector未通过验证，则全部置为未通过
			if !isValid {
				verified = false
			}
		}
		//如果全部通过验证，则存放扇区信息
		if verified {
			//如果此时开启审计，则把历史版本和所有更新版本存下来
			ac.IAMutex.RLock()
			isa := ac.IsAudit
			ac.IAMutex.RUnlock()
			if isa {
				cidfn := req.ClientId + "-" + req.Filename
				ac.CFRMMutex.RLock()
				oldV := ac.ClientFileVersionMap[cidfn]
				ac.MFRRMutex.Lock()
				if ac.MulVFileSectorInforMap[cid_fn] == nil {
					ac.MulVFileSectorInforMap[cid_fn] = make(map[int]map[int]*SectorSealedInfor)
				}
				ac.MulVFileSectorInforMap[cid_fn][oldV] = ac.ClientFileRepSectorInforMap[cid_fn]
				ac.MFRRMutex.Unlock()
				ac.CFRMMutex.RUnlock()
			}
			//将最新的信息更新至列表中
			ac.CFRMMutex.Lock()
			ac.ClientFileVersionMap[req.ClientId+"-"+req.Filename] = int(req.Newversion)
			ac.ClientFileRepSectorInforMap[cid_fn] = fileRepSectorSealedInfor
			ac.CFRMMutex.Unlock()
			delete(ac.PendingUpdClientFileRepSectorInforMap, cid_fn)
		} else {
			fmt.Println(cid_fn, "update verified failed.")
		}
	}
	message = req.Filename + " complete update"
	return &pb.FilecoinUFCResponse{Filename: req.Filename, Message: message}, nil
}

// 【在生成Auditor对象时启动】审计方每隔sleepSeconds秒对每个文件的副本进行审计
func (ac *FilecoinAC) KeepAuditing(sleepSeconds int) {
	time.Sleep(50 * time.Second)
	auditNo := 0
	for {
		// 构建每个存储节点上的审计文件表
		snfnimap := make(map[string][]string)          //key:snid,value:cid-fn-i
		snfnivmap := make(map[string]map[string]int32) //key:snid,subkey:cid-fn-i,subvalue:version
		var snfnivmutex sync.RWMutex
		snfnisimap := make(map[string]map[string]map[int]*SectorSealedInfor) //key:snid,subkey:cid-fn-i,subvalue:扇区信息表int:扇区号
		var snfnisimutex sync.RWMutex
		for {
			ac.CFRMMutex.RLock()
			for cid_fni, snid := range ac.ClientFileRepSNMap {
				if snfnimap[snid] == nil {
					snfnimap[snid] = make([]string, 0)
				}
				snfnimap[snid] = append(snfnimap[snid], cid_fni)
				if snfnivmap[snid] == nil {
					snfnivmap[snid] = make(map[string]int32)
				}
				cid_fni_split := strings.Split(cid_fni, "-")
				cidfn := cid_fni_split[0] + "-" + cid_fni_split[1]
				snfnivmap[snid][cid_fni] = int32(ac.ClientFileVersionMap[cidfn])
				// // 将文件版本号替换为待更新版本
				// ac.PUCFRMMutex.RLock()
				// if ac.PendingUpdateFileVersionMap[cid_fni] != 0 {
				// 	snfnivmap[snid][cid_fni] = int32(ac.PendingUpdateFileVersionMap[cid_fni])
				// }
				// ac.PUCFRMMutex.RUnlock()
			}
			ac.CFRMMutex.RUnlock()
			if len(snfnivmap) > 0 {
				break
			}
		}
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
					prea_req := &pb.FilecoinPASNRequest{Auditno: auditNostr, Snid: snId, Cidfnis: snfnimap[snId][start:end], Cidfniv: subcidfniv, Totalrpcs: int32(prea_rpc_num), Currpcno: int32(i)}
					//发送预审计请求
					prea_res, err := ac.SNRPCs[snId].FilecoinPreAuditSN(context.Background(), prea_req)
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
					cidfn := cidfnisplit[0] + "-" + cidfnisplit[1]
					//如果当前版本等于version，则用当前版本的随机数
					//如果当前版本大于version，则去多版本表中找对应版本
					//如果当前版本小于version，则需等待完成更新
					ac.CFRMMutex.RLock()
					currV := ac.ClientFileVersionMap[cidfn]
					ac.CFRMMutex.RUnlock()
					if currV <= int(version) {
						snfnisimutex.Lock()
						if snfnisimap[snId] == nil {
							snfnisimap[snId] = make(map[string]map[int]*SectorSealedInfor)
						}
						ac.CFRMMutex.RLock()
						snfnisimap[snId][cidfni] = ac.ClientFileRepSectorInforMap[cidfni]
						ac.CFRMMutex.RUnlock()
						snfnisimutex.Unlock()
					} else if currV > int(version) {
						snfnisimutex.Lock()
						if snfnisimap[snId] == nil {
							snfnisimap[snId] = make(map[string]map[int]*SectorSealedInfor)
						}
						ac.MFRRMutex.RLock()
						snfnisimap[snId][cidfni] = ac.MulVFileSectorInforMap[cidfni][int(version)]
						ac.MFRRMutex.RUnlock()
						snfnisimutex.Unlock()
						// } else {
						// 	for {
						// 		ac.CFRMMutex.RLock()
						// 		currV = ac.ClientFileVersionMap[cidfn]
						// 		ac.CFRMMutex.RUnlock()
						// 		if currV == int(version) {
						// 			snfnisimutex.Lock()
						// 			if snfnisimap[snId] == nil {
						// 				snfnisimap[snId] = make(map[string]map[int]*SectorSealedInfor)
						// 			}
						// 			ac.CFRMMutex.RLock()
						// 			snfnisimap[snId][cidfni] = ac.ClientFileRepSectorInforMap[cidfni]
						// 			ac.CFRMMutex.RUnlock()
						// 			snfnisimutex.Unlock()
						// 			break
						// 		}
						// 	}
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
					snfnisimutex.RLock()
					subcidfnisi := make(map[string]map[int]*SectorSealedInfor)
					for j := start; j < end; j++ {
						subcidfnisi[snfnimap[snId][j]] = snfnisimap[snId][snfnimap[snId][j]]
					}
					snfnisimutex.RUnlock()
					// 生成存储证明审计挑战
					randomness := []byte{14, 8, 12}
					gapos_req := &pb.FilecoinGAPSNRequest{Auditno: auditNostr, Randomness: randomness, Totalrpcs: int32(prea_rpc_num), Currpcno: int32(i)}
					//RPC获取存储证明
					gapos_res, err := ac.SNRPCs[snId].FilecoinGetPosSN(context.Background(), gapos_req)
					if err != nil {
						log.Fatalf("storagenode could not process request: %v", err)
					}
					//验证存储证明
					for cidfnisen, proofsmess := range gapos_res.Proofs {
						cidfnisen_split := strings.Split(cidfnisen, "-")
						cid_fn_i := cidfnisen_split[0] + "-" + cidfnisen_split[1] + "-" + cidfnisen_split[2]
						snint, _ := strconv.Atoi(cidfnisen_split[3])
						sectorNum := abi.SectorNumber(snint)
						ac.CFRMMutex.RLock()
						minerID := ac.ClientFileRepMinerMap[cid_fn_i]
						sealedCID := subcidfnisi[cid_fn_i][snint].SealedCID
						ac.CFRMMutex.RUnlock()
						// 验证存储证明
						// 构建待证明的扇区信息集合
						provingSet := []prooftypes.SectorInfo{{
							SealProof:    ac.SealProofType,
							SectorNumber: sectorNum,
							SealedCID:    sealedCID,
						}}
						winningPostProofType := abi.RegisteredPoStProof_StackedDrgWinning2KiBV1
						indicesInProvingSet, err := ffi.GenerateWinningPoStSectorChallenge(winningPostProofType, minerID, randomness[:], uint64(len(provingSet)))
						if err != nil {
							fmt.Println("GenerateWinningPoStSectorChallenge", err.Error())
						}
						var challengedSectors []prooftypes.SectorInfo
						for idx := range indicesInProvingSet {
							challengedSectors = append(challengedSectors, provingSet[indicesInProvingSet[idx]])
						}
						isValid, err := ffi.VerifyWinningPoSt(prooftypes.WinningPoStVerifyInfo{
							Randomness:        randomness[:],
							Proofs:            MessageToProofs(proofsmess),
							ChallengedSectors: challengedSectors,
							Prover:            minerID,
						})
						if err != nil {
							fmt.Println(cid_fn_i, snint, "VerifyWinningPoSt", err.Error())
						}
						fmt.Println(cid_fn_i, snint, "verify winning post:", isValid)
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
		ac.MulVFileSectorInforMap = make(map[string]map[int]map[int]*SectorSealedInfor)
		ac.MFRRMutex.Unlock()
		time.Sleep(time.Duration(sleepSeconds) * time.Second)
	}
}

// 将proofs消息转换为[]prooftypes.PoStProof
func MessageToProofs(proofsmess *pb.FilecoinBytesArray) []prooftypes.PoStProof {
	proofs := make([]prooftypes.PoStProof, 0)
	for i := 0; i < len(proofsmess.Values); i++ {
		// 创建一个 bytes.Buffer 并用序列化后的字节填充它
		buf := bytes.NewReader(proofsmess.Values[i])
		// 创建一个空的 PoStProof 实例
		var postProof prooftypes.PoStProof
		// 反序列化数据到 PoStProof 实例
		err := postProof.UnmarshalCBOR(buf)
		if err != nil {
			log.Fatal(err)
		}
		proofs = append(proofs, postProof)
	}
	return proofs
}
