package nodes

import (
	"ECDS/pdp"
	pb "ECDS/proto" // 根据实际路径修改
	"ECDS/util"
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

	"github.com/Nik-U/pbc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Auditor struct {
	IpAddr                          string                                 //审计方的IP地址
	Params                          string                                 //用于构建pairing的参数
	G                               []byte                                 //用于验签和验证存储证明的公钥
	ClientPK                        map[string][]byte                      //客户端的公钥信息，key:clientID,value:公钥
	CPKMutex                        sync.RWMutex                           //ClientPK的读写锁
	SNAddrMap                       map[string]string                      //存储节点的地址表，key:存储节点id，value:存储节点地址
	ClientFileShardsMap             map[string]map[string]string           //客户端文件的分片所在的存储节点表，key:客户端id-文件名，value:数据分片序号对应的存储节点id列表
	CFSMMutex                       sync.RWMutex                           //ClientFileShardsMap的读写锁
	MetaFileMap                     map[string]*util.Meta4File             // 用于记录每个文件的元数据,key:客户端id-文件名
	MFMutex                         sync.RWMutex                           //MetaFileMap的读写锁
	SNDSVMap                        map[string]map[string]map[string]int32 //用于记录每个存储节点所存储的客户端文件分片及其版本号，key:snid,clientid-fn,dsno;value:version
	SNDSVMutex                      sync.RWMutex                           //SNDSVMap的读写锁
	DataNum                         int                                    //系统中用于文件编码的数据块数量
	ParityNum                       int                                    //系统中用于文件编码的校验块数量
	pb.UnimplementedACServiceServer                                        // 嵌入匿名字段
	SNRPCs                          map[string]pb.SNACServiceClient        //存储节点RPC对象列表，key:存储节点id
	PendingUpdDSMap                 map[string]map[string]map[string]int   //等待存储节点更新的列表，key:clientID-filename, subkey:dsno,subsubkey:version,value:1-等待更新,2-更新完成
	PUDSMMutex                      sync.RWMutex                           //PendingUpdDSMap的读写锁
	IsAudit                         bool                                   //标识当前是否处于审计状态
	IAMutex                         sync.RWMutex                           //IsAudit的读写锁
	MulVMetaFileMap                 map[string]map[string]*util.Meta4File  //多版本分片元信息，key:clientId-filename,subkey:version
	MVMFMMutex                      sync.RWMutex                           //MulVMetaFileMap的读写锁
	PendingUpdDSLatestVMap          map[string]map[string]int32            //正在等待更新的存储分片最新版本
	PUDSLVMMutex                    sync.RWMutex                           //PendingUpdDSLatestVMap的读写锁
}

// 新建一个审计方，持续监听消息
func NewAuditor(ipaddr string, snaddrfilename string, dn int, pn int) *Auditor {
	params := pbc.GenerateA(160, 512).String()
	pairing, _ := pbc.NewPairingFromString(params)
	g := pairing.NewG2().Rand().Bytes()
	cpis := make(map[string][]byte)
	snaddrmap := util.ReadSNAddrFile(snaddrfilename)
	cfsmap := make(map[string]map[string]string)
	metaFileMap := make(map[string]*util.Meta4File)
	sndsvm := make(map[string]map[string]map[string]int32)
	pudslvmap := make(map[string]map[string]int32)
	//设置连接存储节点服务器的地址
	snrpcs := make(map[string]pb.SNACServiceClient)
	for key, value := range *snaddrmap {
		snconn, err := grpc.Dial(value, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
		if err != nil {
			log.Fatalf("did not connect to storage node: %v", err)
		}
		sc := pb.NewSNACServiceClient(snconn)
		snrpcs[key] = sc
	}
	pudsm := make(map[string]map[string]map[string]int)
	mvmfm := make(map[string]map[string]*util.Meta4File)
	auditor := &Auditor{ipaddr, params, g, cpis, sync.RWMutex{}, *snaddrmap, cfsmap, sync.RWMutex{}, metaFileMap, sync.RWMutex{}, sndsvm, sync.RWMutex{}, dn, pn, pb.UnimplementedACServiceServer{}, snrpcs, pudsm, sync.RWMutex{}, false, sync.RWMutex{}, mvmfm, sync.RWMutex{}, pudslvmap, sync.RWMutex{}}
	//设置监听地址
	lis, err := net.Listen("tcp", ipaddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterACServiceServer(s, auditor)
	// log.Println("Server listening on port 50051")
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	//向所有存储节点传递params和g
	for _, value := range auditor.SNRPCs {
		//构造请求消息
		pg_req := &pb.ACRegistSNRequest{Params: params, G: g}
		//向存储节点发送请求
		_, err := value.ACRegisterSN(context.Background(), pg_req)
		if err != nil {
			log.Fatalf("storagenode could not process request: %v", err)
		}
		// log.Println(pg_res.Message)
	}
	//启动持续审计
	go auditor.KeepAuditing(20)
	return auditor
}

// 【供client使用的RPC】客户端获取参数信息
func (ac *Auditor) GetParamsG(ctx context.Context, req *pb.GetPGRequest) (*pb.GetPGResponse, error) {
	// log.Printf("Received getparamsg message from client: %s\n", req.ClientId)
	return &pb.GetPGResponse{Params: ac.Params, G: ac.G}, nil
}

// 【供client使用的RPC】客户端注册，存储方记录客户端公开信息
func (ac *Auditor) RegisterAC(ctx context.Context, req *pb.RegistACRequest) (*pb.RegistACResponse, error) {
	// log.Printf("Received regist message from client: %s\n", req.ClientId)
	message := ""
	var e error
	ac.CPKMutex.Lock()
	if ac.ClientPK[req.ClientId] != nil {
		message = "client information has already exist!"
		e = errors.New("client already exist")
	} else {
		ac.ClientPK[req.ClientId] = req.PK
		message = "client registration successful!"
		e = nil
	}
	ac.CPKMutex.Unlock()
	return &pb.RegistACResponse{ClientId: req.ClientId, Message: message}, e
}

// 【供Client使用的RPC】选择存储节点，给客户端回复消息
func (ac *Auditor) SelectSNs(ctx context.Context, sreq *pb.StorageRequest) (*pb.StorageResponse, error) {
	// log.Printf("Received storage message from client: %s\n", sreq.ClientId)
	// 为该客户端文件随机选择存储节点
	dssnids, pssnids := ac.SelectStorNodes(sreq.Filename)
	//启动线程通知各存储节点待存储的文件分片序号,等待存储完成
	go ac.PutFileNoticeToSN(sreq.ClientId, sreq.Filename, dssnids, pssnids)
	return &pb.StorageResponse{Filename: sreq.Filename, SnsForDs: dssnids, SnsForPs: pssnids}, nil
}

// AC选择存储节点
func (ac *Auditor) SelectStorNodes(filename string) ([]string, []string) {
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
func (ac *Auditor) PutFileNoticeToSN(cid string, fn string, dssnids []string, pssnids []string) {
	for i := 0; i < len(dssnids); i++ {
		go func(snid string, i int) {
			//构造请求消息
			pds_req := &pb.ClientStorageRequest{
				ClientId: cid,
				Filename: fn,
				Dsno:     "d-" + strconv.Itoa(i),
			}
			//发送请求消息给存储节点
			pds_res, err := ac.SNRPCs[snid].PutDataShardNotice(context.Background(), pds_req)
			if err != nil {
				log.Fatalf("storagenode could not process request: %v", err)
			}
			//处理存储节点回复消息：
			// log.Println("recieved putds respones:", pds_res.Message)
			//1-将存储节点放入正式列表中
			cid_fn := pds_res.ClientId + "-" + pds_res.Filename
			dsno := pds_res.Dsno
			ac.CFSMMutex.Lock()
			if ac.ClientFileShardsMap[cid_fn] == nil {
				ac.ClientFileShardsMap[cid_fn] = make(map[string]string)
			}
			ac.ClientFileShardsMap[cid_fn][dsno] = snid
			ac.CFSMMutex.Unlock()
			//2-将分片元信息记录到列表中
			ac.MFMutex.Lock()
			if ac.MetaFileMap[cid_fn] == nil {
				metaInfo := util.NewMeta4File()
				ac.MetaFileMap[cid_fn] = metaInfo
			}
			ac.MetaFileMap[cid_fn].LatestVersionSlice[dsno] = pds_res.Version
			ac.MetaFileMap[cid_fn].LatestTimestampSlice[dsno] = pds_res.Timestamp
			ac.MFMutex.Unlock()
			//3-将信息记录至SNDSVMap中
			ac.SNDSVMutex.Lock()
			if ac.SNDSVMap[snid] == nil {
				ac.SNDSVMap[snid] = make(map[string]map[string]int32)
			}
			if ac.SNDSVMap[snid][cid_fn] == nil {
				ac.SNDSVMap[snid][cid_fn] = make(map[string]int32)
			}
			ac.SNDSVMap[snid][cid_fn][dsno] = pds_res.Version
			ac.SNDSVMutex.Unlock()
		}(dssnids[i], i)
	}

	for i := 0; i < len(pssnids); i++ {
		go func(snid string, i int) {
			//构造请求消息
			pds_req := &pb.ClientStorageRequest{
				ClientId: cid,
				Filename: fn,
				Dsno:     "p-" + strconv.Itoa(i),
			}
			//发送请求消息给存储节点
			pds_res, err := ac.SNRPCs[snid].PutDataShardNotice(context.Background(), pds_req)
			if err != nil {
				log.Fatalf("storagenode could not process request: %v", err)
			}
			//处理存储节点回复消息：
			// log.Println("recieved putds respones:", pds_res.Message)
			//1-将存储节点id记录到列表中
			cid_fn := pds_res.ClientId + "-" + pds_res.Filename
			dsno := pds_res.Dsno
			ac.CFSMMutex.Lock()
			if ac.ClientFileShardsMap[cid_fn] == nil {
				ac.ClientFileShardsMap[cid_fn] = make(map[string]string)
			}
			ac.ClientFileShardsMap[cid_fn][dsno] = snid
			ac.CFSMMutex.Unlock()
			//2-将分片元信息记录到列表中
			ac.MFMutex.Lock()
			if ac.MetaFileMap[cid_fn] == nil {
				metaInfo := util.NewMeta4File()
				ac.MetaFileMap[cid_fn] = metaInfo
			}
			ac.MetaFileMap[cid_fn].LatestVersionSlice[dsno] = pds_res.Version
			ac.MetaFileMap[cid_fn].LatestTimestampSlice[dsno] = pds_res.Timestamp
			ac.MFMutex.Unlock()
			//3-将信息记录至SNDSVMap中
			ac.SNDSVMutex.Lock()
			if ac.SNDSVMap[snid] == nil {
				ac.SNDSVMap[snid] = make(map[string]map[string]int32)
			}
			if ac.SNDSVMap[snid][cid_fn] == nil {
				ac.SNDSVMap[snid][cid_fn] = make(map[string]int32)
			}
			ac.SNDSVMap[snid][cid_fn][dsno] = pds_res.Version
			ac.SNDSVMutex.Unlock()
		}(pssnids[i], i)
	}
}

// 【供client使用的RPC】文件存放确认:确认文件已在存储节点上完成存放,确认元信息一致
func (ac *Auditor) PutFileCommit(ctx context.Context, req *pb.PFCRequest) (*pb.PFCResponse, error) {
	// log.Printf("Received putfilecommit message from client: %s\n", req.ClientId)
	cid_fn := req.ClientId + "-" + req.Filename
	versions := req.Versions
	timmestamps := req.Timestamps
	message := ""
	// 确认存储节点是否已经跟审计方确认存储
	for {
		ac.CFSMMutex.RLock()
		cfss := ac.ClientFileShardsMap[cid_fn]
		ac.CFSMMutex.RUnlock()
		if len(cfss) == ac.DataNum+ac.ParityNum {
			break
		}
	}
	// 确认存储节点接收的元信息与客户端一致
	var metainfo *util.Meta4File
	for {
		ac.MFMutex.RLock()
		if ac.MetaFileMap[cid_fn] != nil {
			metainfo = util.CopyMeta4File(ac.MetaFileMap[cid_fn])
			ac.MFMutex.RUnlock()
			break
		}
		ac.MFMutex.RUnlock()
	}
	for k, v := range metainfo.LatestVersionSlice {
		if versions[k] != v {
			message = "the version of " + k + " is not consistent."
			e := errors.New("version not consistent")
			return &pb.PFCResponse{Filename: req.Filename, Message: message}, e
		}
	}
	for k, v := range metainfo.LatestTimestampSlice {
		if timmestamps[k] != v {
			ac.MFMutex.Unlock()
			message = "the timestamp of " + k + "is not consistent."
			e := errors.New("timestamp not consistent")
			return &pb.PFCResponse{Filename: req.Filename, Message: message}, e
		}
	}
	message = "put file finished and metainfo is consistent."
	return &pb.PFCResponse{Filename: req.Filename, Message: message}, nil
}

// 【供client使用的RPC】获取文件数据分片所在的存储节点id
func (ac *Auditor) GetFileSNs(ctx context.Context, req *pb.GFACRequest) (*pb.GFACResponse, error) {
	// log.Printf("Received get file sns message from client: %s\n", req.ClientId)
	cid_fn := req.ClientId + "-" + req.Filename
	snsds := make(map[string]string)
	//为客户端找到文件的所有数据分片对应的存储节点
	ac.CFSMMutex.RLock()
	if ac.ClientFileShardsMap[cid_fn] == nil {
		ac.CFSMMutex.RUnlock()
		e := errors.New("client filename not exist")
		return &pb.GFACResponse{Filename: req.Filename, Snsds: snsds}, e
	} else {
		for key, value := range ac.ClientFileShardsMap[cid_fn] {
			if strings.HasPrefix(key, "d") {
				snsds[key] = value
			}
		}
		ac.CFSMMutex.RUnlock()
	}
	return &pb.GFACResponse{Filename: req.Filename, Snsds: snsds}, nil
}

// 【供client使用的RPC】报告获取DS错误，并请求获取校验块所在的存储节点id
func (ac *Auditor) GetDSErrReport(ctx context.Context, req *pb.GDSERequest) (*pb.GDSEResponse, error) {
	// log.Printf("Received get DS error message from client: %s\n", req.ClientId)
	cid_fn := req.ClientId + "-" + req.Filename
	blacksns := req.Blacksns
	dsnosnmap := make(map[string]string)
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
			ac.CFSMMutex.RLock()
			snid := ac.ClientFileShardsMap[cid_fn][psno]
			ac.CFSMMutex.RUnlock()
			if !exists && blacksns[snid] != "h" && blacksns[snid] != psno {
				dsnosnmap[psno] = snid
				break
			}
		}
	}
	return &pb.GDSEResponse{Filename: req.Filename, Snsds: dsnosnmap}, nil
}

// 【供client使用的RPC】获取某个分片所在的存储节点id
func (ac *Auditor) GetDSSn(ctx context.Context, req *pb.GDSSNRequest) (*pb.GDSSNResponse, error) {
	// log.Printf("Received get DSSN message from client: %s\n", req.ClientId)
	cid_fn := req.ClientId + "-" + req.Filename
	dssnmap := make(map[string]string)
	pssnmap := make(map[string]string)
	var e error
	var newv int32
	ac.CFSMMutex.RLock()
	_, exist := ac.ClientFileShardsMap[cid_fn][req.Dsno]
	if exist {
		//说明分片是独立的存在于一个存储节点上
		//返回数据块所在存储节点id
		dssnmap[req.Dsno] = ac.ClientFileShardsMap[cid_fn][req.Dsno]
		//获取新版本号
		ac.PUDSLVMMutex.Lock()
		if ac.PendingUpdDSLatestVMap[cid_fn] == nil {
			ac.PendingUpdDSLatestVMap[cid_fn] = make(map[string]int32)
		}
		if ov, ok := ac.PendingUpdDSLatestVMap[cid_fn][req.Dsno]; ok {
			newv = ov + 1
			ac.PendingUpdDSLatestVMap[cid_fn][req.Dsno] = newv
		} else {
			ac.MFMutex.RLock()
			oldv := ac.MetaFileMap[cid_fn].LatestVersionSlice[req.Dsno]
			ac.MFMutex.RUnlock()
			newv = oldv + 1
			ac.PendingUpdDSLatestVMap[cid_fn][req.Dsno] = newv
		}
		ac.PUDSLVMMutex.Unlock()
		//返回校验块所在存储节点id
		for i := 0; i < ac.ParityNum; i++ {
			key := "p-" + strconv.Itoa(i)
			pssnmap[key] = ac.ClientFileShardsMap[cid_fn][key]
		}
	} else {
		//说明分片是后加入的分散存储在多个存储节点上
		//返回数据块所在存储节点id
		for i := 0; i < ac.DataNum; i++ {
			key := req.Dsno + "-" + strconv.Itoa(i)
			_, exist2 := ac.ClientFileShardsMap[cid_fn][key]
			if exist2 {
				dssnmap[key] = ac.ClientFileShardsMap[cid_fn][key]
			} else {
				e = errors.New("datashard" + req.Dsno + "not exist")
			}
		}
		//解析分片序号
		dsnosplit := strings.Split(req.Dsno, "-")
		//返回校验块所在存储节点id
		for i := 0; i < ac.ParityNum; i++ {
			key := "p-" + dsnosplit[1] + "-" + strconv.Itoa(i)
			pssnmap[key] = ac.ClientFileShardsMap[cid_fn][key]
		}
	}
	ac.CFSMMutex.RUnlock()
	//如果需要更新则启动线程通知各SN分片更新的文件分片序号，等待更新完成
	if req.Isupdate {
		go ac.UpdateDSNoticeToSN(req.ClientId, req.Filename, newv, dssnmap, pssnmap)
	}
	return &pb.GDSSNResponse{Filename: req.Filename, Snsds: dssnmap, Snsps: pssnmap}, e
}

// 【GetDSSn-RPC被调用时自动触发】告知审计方待更新的存储节点id，从中出发审计方通知相应存储节点（自动包括校验块）
func (ac *Auditor) UpdateDSNoticeToSN(cid string, fn string, newv int32, dssnmap map[string]string, pssnmap map[string]string) {
	for key, value := range dssnmap {
		go func(dsno string, snid string) {
			//标记pending更新分片列表中的value为1
			cid_fn := cid + "-" + fn
			ac.PUDSMMutex.Lock()
			if ac.PendingUpdDSMap[cid_fn] == nil {
				ac.PendingUpdDSMap[cid_fn] = make(map[string]map[string]int)
			}
			if ac.PendingUpdDSMap[cid_fn][dsno] == nil {
				ac.PendingUpdDSMap[cid_fn][dsno] = make(map[string]int)
			}
			ac.PendingUpdDSMap[cid_fn][dsno][strconv.Itoa(int(newv))] = 1
			ac.PUDSMMutex.Unlock()
			//如果当前版本有更旧的版本未执行完毕则阻塞等待
			ac.MFMutex.RLock()
			latestV := ac.MetaFileMap[cid_fn].LatestVersionSlice[dsno]
			ac.MFMutex.RUnlock()
			for i := int(newv) - 1; i > int(latestV); i-- {
				ac.PUDSMMutex.RLock()
				if _, ok := ac.PendingUpdDSMap[cid_fn][dsno][strconv.Itoa(i)]; ok {
					ac.PUDSMMutex.RUnlock()
					for {
						ac.PUDSMMutex.RLock()
						udsstate := ac.PendingUpdDSMap[cid_fn][dsno][strconv.Itoa(i)]
						ac.PUDSMMutex.RUnlock()
						if udsstate == 2 {
							break
						}
					}
				} else {
					ac.PUDSMMutex.RUnlock()
				}
			}
			//构造请求消息
			udsn_req := &pb.ClientUpdDSRequest{
				ClientId: cid,
				Filename: fn,
				Dsno:     key,
			}
			//发送请求消息给存储节点
			udsn_res, err := ac.SNRPCs[snid].UpdateDataShardNotice(context.Background(), udsn_req)
			if err != nil {
				log.Fatalf("storagenode could not process request: %v", err)
			}
			//处理存储节点回复消息：
			// log.Println("recieved upddsn respones:", udsn_res.Message)
			//标记分片更新表中value为2
			cid_fn = udsn_res.ClientId + "-" + udsn_res.Filename
			udsno := udsn_res.Dsno
			ac.PUDSMMutex.Lock()
			ac.PendingUpdDSMap[cid_fn][udsno][strconv.Itoa(int(udsn_res.Version))] = 2
			ac.PUDSMMutex.Unlock() //在此处解除写锁，保证元信息同步更新
			//修改分片元信息
			ac.MFMutex.Lock()
			ac.MetaFileMap[cid_fn].LatestVersionSlice[udsno] = udsn_res.Version
			ac.MetaFileMap[cid_fn].LatestTimestampSlice[udsno] = udsn_res.Timestamp
			ac.MFMutex.Unlock()
			//如果处于审计状态，则需修改多版本元信息表
			ac.IAMutex.RLock()
			isa := ac.IsAudit
			if isa {
				ac.MVMFMMutex.Lock()
				if ac.MulVMetaFileMap[cid_fn] == nil {
					ac.MulVMetaFileMap[cid_fn] = make(map[string]*util.Meta4File)
				}
				nv := strconv.Itoa(int(udsn_res.Version))
				if ac.MulVMetaFileMap[cid_fn][nv] == nil {
					ac.MFMutex.RLock()
					ac.MulVMetaFileMap[cid_fn][nv] = util.CopyMeta4File(ac.MetaFileMap[cid_fn])
					ac.MFMutex.RUnlock()
				}
				ac.MulVMetaFileMap[cid_fn][nv].LatestVersionSlice[udsno] = udsn_res.Version
				ac.MulVMetaFileMap[cid_fn][nv].LatestTimestampSlice[udsno] = udsn_res.Timestamp
				ac.MVMFMMutex.Unlock()
			}
			ac.IAMutex.RUnlock()
			//修改存储节点分片版本表
			ac.SNDSVMutex.Lock()
			ac.SNDSVMap[snid][cid_fn][dsno] = udsn_res.Version
			ac.SNDSVMutex.Unlock()
		}(key, value)
	}

	for key, value := range pssnmap {
		go func(psno string, snid string) {
			//标记pending更新分片列表中value为1
			cid_fn := cid + "-" + fn
			ac.PUDSMMutex.Lock()
			if ac.PendingUpdDSMap[cid_fn] == nil {
				ac.PendingUpdDSMap[cid_fn] = make(map[string]map[string]int)
			}
			if ac.PendingUpdDSMap[cid_fn][psno] == nil {
				ac.PendingUpdDSMap[cid_fn][psno] = make(map[string]int)
			}
			ac.PendingUpdDSMap[cid_fn][psno][strconv.Itoa(int(newv))] = 1
			ac.PUDSMMutex.Unlock()
			//如果当前版本有更旧的版本未执行完毕则阻塞等待
			ac.MFMutex.RLock()
			latestV := ac.MetaFileMap[cid_fn].LatestVersionSlice[psno]
			ac.MFMutex.RUnlock()
			for i := int(newv) - 1; i > int(latestV); i-- {
				ac.PUDSMMutex.RLock()
				if _, ok := ac.PendingUpdDSMap[cid_fn][psno][strconv.Itoa(i)]; ok {
					ac.PUDSMMutex.RUnlock()
					for {
						ac.PUDSMMutex.RLock()
						udsstate := ac.PendingUpdDSMap[cid_fn][psno][strconv.Itoa(i)]
						ac.PUDSMMutex.RUnlock()
						if udsstate == 2 {
							break
						}
					}
				} else {
					ac.PUDSMMutex.RUnlock()
				}
			}
			//构造请求消息
			upsn_req := &pb.ClientUpdDSRequest{
				ClientId: cid,
				Filename: fn,
				Dsno:     psno,
			}
			//发送请求消息给存储节点
			upsn_res, err := ac.SNRPCs[snid].UpdateDataShardNotice(context.Background(), upsn_req)
			if err != nil {
				log.Fatalf("storagenode could not process request: %v", err)
			}
			//处理存储节点回复消息：
			// log.Println("recieved updatepsn respones:", upsn_res.Message)
			//标记分片更新表中value为2
			cid_fn = upsn_res.ClientId + "-" + upsn_res.Filename
			upsno := upsn_res.Dsno
			ac.PUDSMMutex.Lock()
			ac.PendingUpdDSMap[cid_fn][upsno][strconv.Itoa(int(upsn_res.Version))] = 2
			ac.PUDSMMutex.Unlock() //在此处解除写锁，保证元信息同步更新
			//修改分片元信息
			ac.MFMutex.Lock()
			ac.MetaFileMap[cid_fn].LatestVersionSlice[upsno] = upsn_res.Version
			ac.MetaFileMap[cid_fn].LatestTimestampSlice[upsno] = upsn_res.Timestamp
			ac.MFMutex.Unlock()
			//如果处于审计状态，则需修改多版本元信息表
			ac.IAMutex.RLock()
			isa := ac.IsAudit
			if isa {
				ac.MVMFMMutex.Lock()
				if ac.MulVMetaFileMap[cid_fn] == nil {
					ac.MulVMetaFileMap[cid_fn] = make(map[string]*util.Meta4File)
				}
				nv := strconv.Itoa(int(upsn_res.Version))
				if ac.MulVMetaFileMap[cid_fn][nv] == nil {
					ac.MFMutex.RLock()
					ac.MulVMetaFileMap[cid_fn][nv] = util.CopyMeta4File(ac.MetaFileMap[cid_fn])
					ac.MFMutex.RUnlock()
				}
				ac.MulVMetaFileMap[cid_fn][nv].LatestVersionSlice[upsno] = upsn_res.Version
				ac.MulVMetaFileMap[cid_fn][nv].LatestTimestampSlice[upsno] = upsn_res.Timestamp
				ac.MVMFMMutex.Unlock()
			}
			ac.IAMutex.RUnlock()
			//修改存储节点分片版本表
			ac.SNDSVMutex.Lock()
			ac.SNDSVMap[snid][cid_fn][upsno] = upsn_res.Version
			ac.SNDSVMutex.Unlock()
		}(key, value)
	}
}

// 【供client使用的RPC】文件存放确认:确认文件已在存储节点上完成存放,确认元信息一致
func (ac *Auditor) UpdateDSCommit(ctx context.Context, req *pb.UDSCRequest) (*pb.UDSCResponse, error) {
	// log.Printf("Received updateDScommit message from client: %s\n", req.ClientId)
	cid_fn := req.ClientId + "-" + req.Filename
	message := ""
	updatedMap := make(map[string]string)
	// 确认所有存储节点是否已经跟审计方确认更新
	for {
		updatedNum := 0
		for i := 0; i < len(req.Dsnos); i++ {
			_, exist := updatedMap[req.Dsnos[i]]
			if exist {
				updatedNum++
			} else {
				ac.PUDSMMutex.RLock()
				pv := ac.PendingUpdDSMap[cid_fn][req.Dsnos[i]][strconv.Itoa(int(req.Versions[req.Dsnos[i]]))]
				ac.PUDSMMutex.RUnlock()
				if pv == 2 {
					//阻塞等待存储节点返回的元信息与客户端发来的一致
					for {
						ac.MFMutex.RLock()
						nv := ac.MetaFileMap[cid_fn].LatestVersionSlice[req.Dsnos[i]]
						nt := ac.MetaFileMap[cid_fn].LatestTimestampSlice[req.Dsnos[i]]
						ac.MFMutex.RUnlock()
						if req.Versions[req.Dsnos[i]] == nv && req.Timestamps[req.Dsnos[i]] == nt {
							updatedNum++
							//加入已更新列表
							ac.CFSMMutex.RLock()
							updatedMap[req.Dsnos[i]] = ac.ClientFileShardsMap[cid_fn][req.Dsnos[i]]
							ac.CFSMMutex.RUnlock()
							//删除pendinglist中的记录
							ac.PUDSMMutex.Lock()
							delete(ac.PendingUpdDSMap[cid_fn][req.Dsnos[i]], strconv.Itoa(int(req.Versions[req.Dsnos[i]])))
							// 如果内层的map为空，可以继续删除空的map
							if len(ac.PendingUpdDSMap[cid_fn][req.Dsnos[i]]) == 0 {
								delete(ac.PendingUpdDSMap[cid_fn], req.Dsnos[i])
							}
							if len(ac.PendingUpdDSMap[cid_fn]) == 0 {
								delete(ac.PendingUpdDSMap, cid_fn)
							}
							ac.PUDSMMutex.Unlock()
							break
						}
					}
				}
			}
		}
		if updatedNum == len(req.Dsnos) {
			break
		}
	}
	message = "update datashard and parityshard finished and metainfo is consistent."
	return &pb.UDSCResponse{Filename: req.Filename, Dssnmap: updatedMap, Message: message}, nil
}

// 【在生成Auditor对象时启动】审计方每隔sleepSeconds秒随机选择dsnum个存储节点进行存储审计
func (ac *Auditor) KeepAuditing(sleepSeconds int) {
	time.Sleep(20 * time.Second)
	auditNo := 0
	seed := time.Now().UnixNano()
	randor := rand.New(rand.NewSource(seed))
	avgDuration := int64(0)
	totalVOSizeMap := make(map[string]int, len(ac.SNAddrMap))
	for {
		st := time.Now()
		ac.SNDSVMutex.RLock()
		sndsvNum := len(ac.SNDSVMap)
		ac.SNDSVMutex.RUnlock()
		if sndsvNum > 0 {
			//选择dsnum个存储节点
			selectednum := make(map[string]bool)
			for {
				randomNum := randor.Intn(ac.DataNum+ac.ParityNum) + 1 // 生成 1 到 (ac.DataNum+ac.ParityNum) 之间的随机数
				selectednum[strconv.Itoa(randomNum)] = true
				if len(selectednum) == ac.DataNum {
					break
				}
			}
			//启动审计，即启动记录多版本
			ac.IAMutex.Lock()
			ac.IsAudit = true
			ac.IAMutex.Unlock()
			//分别对选出的存储节点进行审计
			done := make(chan struct{})
			donenum := ac.DataNum
			for snno, _ := range selectednum {
				snid := "sn" + snno
				go func(snId string) {
					//获取该存储节点上所有ds最新版本号
					ldsvmap := ac.GetLatestDSVMap(snId)
					// log.Println(snId, "ldsvmap:", ldsvmap)
					if ldsvmap == nil {
						donenum--
						return
					}
					// for {
					// 	if len(ldsvmap) > 0 {
					// 		break
					// 	} else {
					// 		ldsvmap = ac.GetLatestDSVMap(snId)
					// 		// log.Println(snId, "ldsvmap:", ldsvmap)
					// 	}
					// }
					//构造预审计请求消息
					auditNostr := "audit-" + strconv.Itoa(auditNo)
					prea_req := &pb.PASNRequest{Auditno: auditNostr, Snid: snId, Dsversion: ldsvmap}
					//发送预审计请求
					prea_res, err := ac.SNRPCs[snId].PreAuditSN(context.Background(), prea_req)
					if err != nil {
						log.Fatalf(snId, "storagenode could not process preaudit request: %v", err)
					}
					// else {
					// 	log.Println(snId, "-preaudit:", auditNostr, "-isready:", prea_res.Isready)
					// }
					//如果未准备好，则更新mfmap中的版本与sn中审计快照一致
					if !prea_res.Isready {
						for cid_fn_dsno, v := range prea_req.Dsversion {
							ldsvmap[cid_fn_dsno] = v
						}
					}
					//组装MetaFileMap
					mfmap := make(map[string]*util.Meta4File)
					for cid_fn_dsno, v := range ldsvmap {
						cfdsnosplit := strings.Split(cid_fn_dsno, "-")
						cid_fn := cfdsnosplit[0] + "-" + cfdsnosplit[1]
						dsno := cfdsnosplit[2] + "-" + cfdsnosplit[3]
						ac.MFMutex.RLock()
						if ac.MetaFileMap[cid_fn].LatestVersionSlice[dsno] == v {
							//如果MFMap中存在该版本的元信息，则加入
							mfmap[cid_fn] = util.CopyMeta4File(ac.MetaFileMap[cid_fn])
							ac.MFMutex.RUnlock()
						} else {
							ac.MFMutex.RUnlock()
							for {
								ac.MVMFMMutex.RLock()
								if value, ok := ac.MulVMetaFileMap[cid_fn][strconv.Itoa(int(v))]; ok {
									// log.Println(snId, cid_fn_dsno, "v:", v, "ac.MulVMetaFileMap[cid_fn][strconv.Itoa(int(v))]:", ac.MulVMetaFileMap[cid_fn][strconv.Itoa(int(v))])
									if value.LatestVersionSlice[dsno] == v {
										mfmap[cid_fn] = util.CopyMeta4File(ac.MulVMetaFileMap[cid_fn][strconv.Itoa(int(v))])
										ac.MVMFMMutex.RUnlock()
										break
									} else if value.LatestVersionSlice[dsno] > v {
										log.Fatalf("version in ac is larger than audit request")
										ac.MVMFMMutex.RUnlock()
									}
								}
								ac.MVMFMMutex.RUnlock()
							}
						}
					}
					//构造审计请求消息
					random := randor.Int31()
					gapos_req := &pb.GAPSNRequest{Auditno: auditNostr, Random: random}
					//RPC获取聚合存储证明
					gapos_res, err := ac.SNRPCs[snid].GetAggPosSN(context.Background(), gapos_req)
					if gapos_res == nil {
						donenum--
						return
					}
					if err != nil {
						log.Fatalf("storagenode could not process request: %v", err)
					}
					//验证存储证明
					// log.Println("verifying", snId, "pos")
					if gapos_res.Aggpos == nil {
						log.Fatalln("gapos_res.Aggpos==nil")
					}
					aggpos, deseerr := pdp.DeserializeAggPOS(gapos_res.Aggpos)
					if deseerr != nil {
						log.Fatalf("aggpos deserialize error: %v", err)
					}
					totalVOSizeMap[snId] = totalVOSizeMap[snId] + len(gapos_res.Aggpos)
					//验证存储证明
					// pdp.VerifyAggPos(aggpos, ac.Params, ac.G, ac.ClientPK, mfmap, random)
					if pdp.VerifyAggPos(aggpos, ac.Params, ac.G, ac.ClientPK, mfmap, random) {
						log.Println("auditNo:", auditNo, snId, "sucessfully")
					} else {
						log.Println("auditNo:", auditNo, snId, "error")
					}
					// 通知主线程任务完成
					done <- struct{}{}
				}(snid)
			}
			// 等待所有协程完成
			for i := 0; i < donenum; i++ {
				<-done
			}
			auditNo++
			//关闭审计
			ac.IAMutex.Lock()
			ac.IsAudit = false
			ac.IAMutex.Unlock()
			//清空多版本元信息列表
			ac.MVMFMMutex.Lock()
			ac.MulVMetaFileMap = make(map[string]map[string]*util.Meta4File)
			ac.MVMFMMutex.Unlock()
			duration := time.Since(st)
			//计算平均延迟
			avgDuration = (avgDuration*int64(auditNo-1) + (duration.Milliseconds())) / int64(auditNo)
			//计算平均VO大小
			totalVOSize := 0
			for snid, _ := range totalVOSizeMap {
				totalVOSize = totalVOSize + totalVOSizeMap[snid]
			}
			avgVOSize := totalVOSize / auditNo
			//计算辅助信息所占字节数
			auditInfoSize := 0
			ac.MFMutex.RLock()
			mfmap := ac.MetaFileMap
			ac.MFMutex.RUnlock()
			for key, value := range mfmap {
				auditInfoSize = auditInfoSize + len([]byte(key)) + value.Sizeof()
			}
			util.LogToFile("/root/DSN/ECDS/data/outlog_ac", "audit-"+strconv.Itoa(auditNo)+" latency="+strconv.Itoa(int(duration.Milliseconds()))+" ms, avgLatency="+strconv.Itoa(int(avgDuration))+" ms, avgVOSize="+strconv.Itoa(avgVOSize/1024)+" KB, auditInforSize="+strconv.Itoa(auditInfoSize/1024)+" KB\n")
			fmt.Println("audit-" + strconv.Itoa(auditNo) + " latency=" + strconv.Itoa(int(duration.Milliseconds())) + " ms, avgLatency=" + strconv.Itoa(int(avgDuration)) + " ms, avgVOSize=" + strconv.Itoa(avgVOSize/1024) + " KB, auditInforSize=" + strconv.Itoa(auditInfoSize/1024) + " KB")
			time.Sleep(time.Duration(sleepSeconds) * time.Second)
		}
	}
}

// 获取某个存储节点上分片的最新版本列表key:clientid-fn-dsno;value:version，用于构建预审计请求
func (ac *Auditor) GetLatestDSVMap(snid string) map[string]int32 {
	ldsvmap := make(map[string]int32)
	//将sndsvmap中的信息加入
	ac.SNDSVMutex.RLock()
	for cid_fn, dsvmap := range ac.SNDSVMap[snid] {
		for dsno, v := range dsvmap {
			cid_fn_dsno := cid_fn + "-" + dsno
			ldsvmap[cid_fn_dsno] = v
			//同时将ldsvmap中对应版本的分片元信息加入多版本元信息表
			ac.MVMFMMutex.Lock()
			if ac.MulVMetaFileMap[cid_fn] == nil {
				ac.MulVMetaFileMap[cid_fn] = make(map[string]*util.Meta4File)
			}
			ac.MFMutex.RLock()
			if ac.MetaFileMap[cid_fn].LatestVersionSlice[dsno] > v {
				v = ac.MetaFileMap[cid_fn].LatestVersionSlice[dsno]
				ldsvmap[cid_fn_dsno] = v
			}
			if ac.MulVMetaFileMap[cid_fn][strconv.Itoa(int(v))] == nil {
				ac.MulVMetaFileMap[cid_fn][strconv.Itoa(int(v))] = util.CopyMeta4File(ac.MetaFileMap[cid_fn])
			} else {
				ac.MulVMetaFileMap[cid_fn][strconv.Itoa(int(v))].LatestVersionSlice[dsno] = ac.MetaFileMap[cid_fn].LatestVersionSlice[dsno]
				ac.MulVMetaFileMap[cid_fn][strconv.Itoa(int(v))].LatestTimestampSlice[dsno] = ac.MetaFileMap[cid_fn].LatestTimestampSlice[dsno]
			}
			ac.MFMutex.RUnlock()
			ac.MVMFMMutex.Unlock()
		}
	}
	ac.SNDSVMutex.RUnlock()
	//将pendinglist中的信息加入
	ac.PUDSMMutex.RLock()
	for cid_fn, dsnovmap := range ac.PendingUpdDSMap {
		for dsno, vmap := range dsnovmap {
			for v, _ := range vmap {
				v_int, _ := strconv.Atoi(v)
				v_int32 := int32(v_int)
				cid_fn_dsno := cid_fn + "-" + dsno
				if v, ok := ldsvmap[cid_fn_dsno]; ok {
					if v_int32 > v {
						ldsvmap[cid_fn_dsno] = v_int32
					}
				}

			}
		}
	}
	ac.PUDSMMutex.RUnlock()
	return ldsvmap
}

// 打印auditor
func (ac *Auditor) PrintAuditor() {
	str := "Auditor:{IpAddr:" + ac.IpAddr + ",SNAddrMap:{"
	for key, value := range ac.SNAddrMap {
		str = str + key + ":" + value + ","
	}
	str = str + "},ClientFileShardMap:{"
	for key, value := range ac.ClientFileShardsMap {
		str = str + key + ":{"
		for k, v := range value {
			str = str + k + ":" + v + ","
		}
		str = str + "},"
	}
	str = str + "},DataNum:" + strconv.Itoa(ac.DataNum) + ",ParityNum:" + strconv.Itoa(ac.ParityNum) + "}"
	log.Println(str)
}
