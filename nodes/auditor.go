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
	auditor := &Auditor{ipaddr, cpis, sync.RWMutex{}, *snaddrmap, cfsmap, sync.RWMutex{}, metaFileMap, sync.RWMutex{}, dn, pn, pb.UnimplementedACServiceServer{}, snrpcs}
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
		cpi := util.NewPublicInfo(req.G, req.PK)
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
	count := 0
	ac.CFSMMutex.RLock()
	for key, value := range ac.ClientFileShardsMap[cid_fn] {
		if strings.HasPrefix(key, "p") && blacksns[value] != "h" && blacksns[value] != "b" {
			dsnosnmap[key] = value
			count++
			if count == len(req.Errdssn) {
				break
			}
		}
	}
	ac.CFSMMutex.RUnlock()
	return &pb.GDSEResponse{Filename: req.Filename, Snsds: dsnosnmap}, nil
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
