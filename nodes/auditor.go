package nodes

import (
	"ECDS/encode"
	pb "ECDS/proto" // 根据实际路径修改
	"ECDS/util"
	"context"
	"errors"
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

type Auditor struct {
	IpAddr                          string                          //审计方的IP地址
	ClientPublicInfor               map[string]*util.PublicInfo     //客户端的公钥信息，key:clientID,value:公钥信息指针
	CPIMutex                        sync.RWMutex                    //ClientPublicInfor的读写锁
	SNAddrMap                       map[string]string               //存储节点的地址表，key:存储节点id，value:存储节点地址
	ClientFileShardsMap             map[string]map[string]string    //客户端文件的分片所在的存储节点表，key:客户端id-文件名，value:数据分片序号对应的存储节点id列表
	CFSMMutex                       sync.RWMutex                    //ClientFileShardsMap的读写锁
	MetaFileMap                     map[string]*encode.Meta4File    // 用于记录每个文件的元数据,key:客户端id-文件名
	MFMutex                         sync.RWMutex                    //MetaFileMap的读写锁
	DataNum                         int                             //系统中用于文件编码的数据块数量
	ParityNum                       int                             //系统中用于文件编码的校验块数量
	pb.UnimplementedACServiceServer                                 // 嵌入匿名字段
	SNRPCs                          map[string]pb.SNACServiceClient //存储节点RPC对象列表，key:存储节点id
	PendingUpdDSMap                 map[string]map[string]int       //等待存储节点更新的列表，key:clientID-filename, subkey:dsno,value:1-等待更新,2-更新完成
	PUDSMMutex                      sync.RWMutex                    //PendingUpdDSMap的读写锁
}

// 新建一个审计方，持续监听消息
func NewAuditor(ipaddr string, snaddrfilename string, dn int, pn int) *Auditor {
	cpis := make(map[string]*util.PublicInfo)
	snaddrmap := util.ReadSNAddrFile(snaddrfilename)
	cfsmap := make(map[string]map[string]string)
	metaFileMap := make(map[string]*encode.Meta4File)
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
	pudsm := make(map[string]map[string]int)
	auditor := &Auditor{ipaddr, cpis, sync.RWMutex{}, *snaddrmap, cfsmap, sync.RWMutex{}, metaFileMap, sync.RWMutex{}, dn, pn, pb.UnimplementedACServiceServer{}, snrpcs, pudsm, sync.RWMutex{}}
	//设置监听地址
	lis, err := net.Listen("tcp", ipaddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterACServiceServer(s, auditor)
	log.Println("Server listening on port 50051")
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	return auditor
}

// 【供client使用的RPC】客户端注册，存储方记录客户端公开信息
func (ac *Auditor) RegisterAC(ctx context.Context, req *pb.RegistACRequest) (*pb.RegistACResponse, error) {
	log.Printf("Received regist message from client: %s\n", req.ClientId)
	message := ""
	var e error
	ac.CPIMutex.Lock()
	if ac.ClientPublicInfor[req.ClientId] != nil {
		message = "client information has already exist!"
		e = errors.New("client already exist")
	} else {
		cpi := util.NewPublicInfo(req.Params, req.G, req.PK)
		ac.ClientPublicInfor[req.ClientId] = cpi
		message = "client registration successful!"
		e = nil
	}
	ac.CPIMutex.Unlock()
	return &pb.RegistACResponse{ClientId: req.ClientId, Message: message}, e
}

// 【供Client使用的RPC】选择存储节点，给客户端回复消息
func (ac *Auditor) SelectSNs(ctx context.Context, sreq *pb.StorageRequest) (*pb.StorageResponse, error) {
	log.Printf("Received storage message from client: %s\n", sreq.ClientId)
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
			log.Println("recieved putds respones:", pds_res.Message)
			//1-将存储节点放入正式列表中
			cid_fn := pds_res.ClientId + "-" + pds_res.Filename
			dsno := pds_res.Dsno
			ac.CFSMMutex.Lock()
			if ac.ClientFileShardsMap[cid_fn] == nil {
				ac.ClientFileShardsMap[cid_fn] = make(map[string]string)
			}
			ac.ClientFileShardsMap[cid_fn][dsno] = snid
			ac.CFSMMutex.Unlock()
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
			log.Println("recieved putds respones:", pds_res.Message)
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
				metaInfo := encode.NewMeta4File()
				ac.MetaFileMap[cid_fn] = metaInfo
			}
			ac.MetaFileMap[cid_fn].LatestVersionSlice[dsno] = pds_res.Version
			ac.MetaFileMap[cid_fn].LatestTimestampSlice[dsno] = pds_res.Timestamp
			ac.MFMutex.Unlock()
		}(pssnids[i], i)
	}
}

// 【供client使用的RPC】文件存放确认:确认文件已在存储节点上完成存放,确认元信息一致
func (ac *Auditor) PutFileCommit(ctx context.Context, req *pb.PFCRequest) (*pb.PFCResponse, error) {
	log.Printf("Received putfilecommit message from client: %s\n", req.ClientId)
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
	ac.MFMutex.RLock()
	defer ac.MFMutex.RUnlock() // 在函数返回时释放读锁
	metainfo := ac.MetaFileMap[cid_fn]
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
	log.Printf("Received get file sns message from client: %s\n", req.ClientId)
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
	log.Printf("Received get DS error message from client: %s\n", req.ClientId)
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
	log.Printf("Received get DSSN message from client: %s\n", req.ClientId)
	cid_fn := req.ClientId + "-" + req.Filename
	dssnmap := make(map[string]string)
	pssnmap := make(map[string]string)
	var e error
	ac.CFSMMutex.RLock()
	_, exist := ac.ClientFileShardsMap[cid_fn][req.Dsno]
	if exist {
		//说明分片是独立的存在于一个存储节点上
		//返回数据块所在存储节点id
		dssnmap[req.Dsno] = ac.ClientFileShardsMap[cid_fn][req.Dsno]
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
		go ac.UpdateDSNoticeToSN(req.ClientId, req.Filename, dssnmap, pssnmap)
	}
	return &pb.GDSSNResponse{Filename: req.Filename, Snsds: dssnmap, Snsps: pssnmap}, e
}

// 【GetDSSn-RPC被调用时自动触发】告知审计方待更新的存储节点id，从中出发审计方通知相应存储节点（自动包括校验块）
func (ac *Auditor) UpdateDSNoticeToSN(cid string, fn string, dssnmap map[string]string, pssnmap map[string]string) {
	for key, value := range dssnmap {
		go func(dsno string, snid string) {
			//标记pending更新分片列表中的value为1
			cid_fn := cid + "-" + fn
			log.Println("cid-fn:", cid_fn, ", dsno:", dsno, ",snid:", snid)
			ac.PUDSMMutex.Lock()
			if ac.PendingUpdDSMap[cid_fn] == nil {
				ac.PendingUpdDSMap[cid_fn] = make(map[string]int)
			}
			ac.PendingUpdDSMap[cid_fn][dsno] = 1
			ac.PUDSMMutex.Unlock()
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
			log.Println("recieved upddsn respones:", udsn_res.Message)
			//标记分片更新表中value为2
			cid_fn = udsn_res.ClientId + "-" + udsn_res.Filename
			udsno := udsn_res.Dsno
			ac.PUDSMMutex.Lock()
			ac.PendingUpdDSMap[cid_fn][udsno] = 2
			ac.PUDSMMutex.Unlock()
			//修改分片元信息
			ac.MFMutex.Lock()
			ac.MetaFileMap[cid_fn].LatestVersionSlice[udsno] = udsn_res.Version
			ac.MetaFileMap[cid_fn].LatestTimestampSlice[udsno] = udsn_res.Timestamp
			ac.MFMutex.Unlock()
		}(key, value)
	}

	for key, value := range pssnmap {
		go func(psno string, snid string) {
			//标记pending更新分片列表中value为1
			cid_fn := cid + "-" + fn
			ac.PUDSMMutex.Lock()
			if ac.PendingUpdDSMap[cid_fn] == nil {
				ac.PendingUpdDSMap[cid_fn] = make(map[string]int)
			}
			ac.PendingUpdDSMap[cid_fn][psno] = 1
			ac.PUDSMMutex.Unlock()
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
			log.Println("recieved updatepsn respones:", upsn_res.Message)
			//标记分片更新表中value为2
			cid_fn = upsn_res.ClientId + "-" + upsn_res.Filename
			upsno := upsn_res.Dsno
			ac.PUDSMMutex.Lock()
			ac.PendingUpdDSMap[cid_fn][upsno] = 2
			ac.PUDSMMutex.Unlock()
			//修改分片元信息
			ac.MFMutex.Lock()
			ac.MetaFileMap[cid_fn].LatestVersionSlice[upsno] = upsn_res.Version
			ac.MetaFileMap[cid_fn].LatestTimestampSlice[upsno] = upsn_res.Timestamp
			ac.MFMutex.Unlock()
		}(key, value)
	}
}

// 【供client使用的RPC】文件存放确认:确认文件已在存储节点上完成存放,确认元信息一致
func (ac *Auditor) UpdateDSCommit(ctx context.Context, req *pb.UDSCRequest) (*pb.UDSCResponse, error) {
	log.Printf("Received updateDScommit message from client: %s\n", req.ClientId)
	cid_fn := req.ClientId + "-" + req.Filename
	message := ""
	updatedMap := make(map[string]string)
	// 确认所有存储节点是否已经跟审计方确认更新
	for {
		updatedNum := 0
		for i := 0; i < len(req.Dsnos); i++ {
			ac.PUDSMMutex.RLock()
			pv := ac.PendingUpdDSMap[cid_fn][req.Dsnos[i]]
			ac.PUDSMMutex.RUnlock()
			if pv == 2 {
				_, exist := updatedMap[req.Dsnos[i]]
				if exist {
					updatedNum++
				} else {
					//确认存储节点返回的元信息与客户端发来的一致
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
					} else {
						message = "the metainfor of " + req.Dsnos[i] + " not consistent."
						e := errors.New("metainfor not consistent")
						return &pb.UDSCResponse{Filename: req.Filename, Dssnmap: updatedMap, Message: message}, e
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
