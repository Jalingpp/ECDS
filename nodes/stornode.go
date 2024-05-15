package nodes

import (
	"ECDS/pdp"
	pb "ECDS/proto"
	"ECDS/util"
	"context"
	"errors"
	"log"
	"net"
	"sync"

	"github.com/Nik-U/pbc"
	"google.golang.org/grpc"
)

type StorageNode struct {
	SNId                              string                                //存储节点id
	SNAddr                            string                                //存储节点ip地址
	Pairing                           *pbc.Pairing                          //双线性映射
	ClientPublicInfor                 map[string]*util.PublicInfo           //客户端的公钥信息，key:clientID,value:公钥信息指针
	CPIMutex                          sync.RWMutex                          //ClientPublicInfor的读写锁
	ClientFileMap                     map[string][]string                   //为客户端存储的文件列表，key:clientID,value:filename
	CFMMutex                          sync.RWMutex                          //ClientFileMap的读写锁
	FileShardsMap                     map[string]map[string]*util.DataShard //文件的数据分片列表，key:clientID-filename,value:(key:分片序号)
	FSMMMutex                         sync.RWMutex                          //FileShardsMap的读写锁
	pb.UnimplementedSNServiceServer                                         // 面向客户端的服务器嵌入匿名字段
	pb.UnimplementedSNACServiceServer                                       // 面向审计方的服务器嵌入匿名字段
	PendingACPutDSNotice              map[string]map[string]int             //用于暂存来自AC的分片存储通知，key:clientid-filename,value:分片序号dsno的列表,int:1表示该分片在等待存储，2表示该分片完成存储
	PACNMutex                         sync.RWMutex                          //用于限制PendingACPutDSNotice访问的锁
}

// 新建存储分片
func NewStorageNode(snid string, snaddr string) *StorageNode {
	params := pbc.GenerateA(160, 512).String()
	pairing, _ := pbc.NewPairingFromString(params)
	clientFileMap := make(map[string][]string)
	fileShardsMap := make(map[string]map[string]*util.DataShard)
	pacpdsn := make(map[string]map[string]int)
	cpi := make(map[string]*util.PublicInfo)
	sn := &StorageNode{snid, snaddr, pairing, cpi, sync.RWMutex{}, clientFileMap, sync.RWMutex{}, fileShardsMap, sync.RWMutex{}, pb.UnimplementedSNServiceServer{}, pb.UnimplementedSNACServiceServer{}, pacpdsn, sync.RWMutex{}}

	//设置监听地址
	lis, err := net.Listen("tcp", snaddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterSNServiceServer(s, sn)
	pb.RegisterSNACServiceServer(s, sn)
	log.Println("Server listening on " + snaddr)
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	return sn
}

// 【供client使用的RPC】客户端注册，存储方记录客户端公开信息
func (sn *StorageNode) RegisterSN(ctx context.Context, req *pb.RegistSNRequest) (*pb.RegistSNResponse, error) {
	log.Printf("Received regist message from client: %s\n", req.ClientId)
	message := ""
	var e error
	sn.CPIMutex.Lock()
	if sn.ClientPublicInfor[req.ClientId] != nil {
		message = "client information has already exist!"
		e = errors.New("client already exist")
	} else {
		cpi := util.NewPublicInfo(req.G, req.PK)
		sn.ClientPublicInfor[req.ClientId] = cpi
		message = "client registration successful!"
		e = nil
	}
	sn.CPIMutex.Unlock()
	return &pb.RegistSNResponse{ClientId: req.ClientId, Message: message}, e
}

// 【供客户端使用的RPC】存储节点验证分片序号是否与审计方通知的一致，验签，存放数据分片，告知审计方已存储或存储失败，给客户端回复消息
func (sn *StorageNode) PutDataShard(ctx context.Context, preq *pb.PutDSRequest) (*pb.PutDSResponse, error) {
	log.Printf("Received message from client: %s\n", preq.ClientId)
	clientId := preq.ClientId
	filename := preq.Filename
	dsno := preq.Dsno
	message := ""
	cid_fn := clientId + "-" + filename
	//1-验证分片序号是否与审计方通知的一致
	sn.PACNMutex.RLock()
	v1, ok1 := sn.PendingACPutDSNotice[cid_fn]
	sn.PACNMutex.RUnlock()
	if !ok1 {
		// 当前客户端文件不存在
		log.Println("No clientId-file received from auditor!")
		message = "No clientId-file received from auditor!"
		e := errors.New("no clientId-file received from auditor")
		return &pb.PutDSResponse{Filename: preq.Filename, Dsno: dsno, Message: message}, e
	}
	_, ok2 := v1[dsno]
	if !ok2 {
		// 分片序号不一致
		log.Println("dsno is inconsistent with auditor's notif!")
		message = "dsno is inconsistent with auditor's notif!"
		e := errors.New("dsno is inconsistent with auditor's notif")
		return &pb.PutDSResponse{Filename: preq.Filename, Dsno: dsno, Message: message}, e
	}
	//2-存放数据分片
	//2-1-提取数据分片对象
	ds, err := util.DeserializeDS(preq.DatashardSerialized)
	if err != nil {
		log.Println("Deserialize DataShard Error!")
		message = "Deserialize DataShard Error!"
		e := errors.New("deserialize dataShard error")
		return &pb.PutDSResponse{Filename: preq.Filename, Dsno: dsno, Message: message}, e
	}
	//2-2-验签
	var g []byte
	var pk []byte
	sn.CPIMutex.RLock()
	if sn.ClientPublicInfor[clientId] == nil {
		sn.CPIMutex.RUnlock()
		message = "client not regist!"
		e := errors.New("client not regist")
		return &pb.PutDSResponse{Filename: preq.Filename, Dsno: dsno, Message: message}, e
	} else {
		g = sn.ClientPublicInfor[clientId].G
		pk = sn.ClientPublicInfor[clientId].PK
		sn.CPIMutex.RUnlock()
	}
	if !pdp.VerifySig(sn.Pairing, g, pk, ds.Data, ds.Sig, ds.Version, ds.Timestamp) {
		message = "signature verify error!"
		e := errors.New("signature verify error")
		return &pb.PutDSResponse{Filename: preq.Filename, Dsno: dsno, Message: message}, e
	}
	//2-3-放置文件分片列表
	sn.FSMMMutex.Lock()
	if sn.FileShardsMap[cid_fn] != nil {
		message = "Filename Already Exist!"
		e := errors.New("filename already exist")
		return &pb.PutDSResponse{Filename: preq.Filename, Dsno: dsno, Message: message}, e
	}
	sn.FileShardsMap[cid_fn] = make(map[string]*util.DataShard)
	sn.FileShardsMap[cid_fn][dsno] = ds
	sn.FSMMMutex.Unlock()
	//2-4-放置客户端文件名列表
	sn.CFMMutex.Lock()
	if sn.ClientFileMap[clientId] == nil {
		sn.ClientFileMap[clientId] = make([]string, 0)
	}
	sn.ClientFileMap[clientId] = append(sn.ClientFileMap[clientId], filename)
	sn.CFMMutex.Unlock()
	message = "Put File Success!"
	//3-修改PendingACPutDSNotice
	sn.PACNMutex.Lock()
	sn.PendingACPutDSNotice[cid_fn][dsno] = 2
	sn.PACNMutex.Unlock()
	//4-告知审计方分片放置结果
	log.Println("Completed record datashard ", dsno, " of file ", filename, ".")
	return &pb.PutDSResponse{Filename: preq.Filename, Dsno: dsno, Message: message}, nil
}

// 【供审计方使用的RPC】存储节点接收审计方分片存放通知，阻塞等待客户端分片存放，完成后回复审计方
func (sn *StorageNode) PutDataShardNotice(ctx context.Context, preq *pb.ClientStorageRequest) (*pb.ClientStorageResponse, error) {
	log.Printf("Received put datashard notice message from auditor.\n")
	clientId := preq.ClientId
	filename := preq.Filename
	dsno := preq.Dsno
	cid_fn := clientId + "-" + filename
	//写来自审计方的分片存储通知
	sn.PACNMutex.Lock()
	if sn.PendingACPutDSNotice[cid_fn] == nil {
		sn.PendingACPutDSNotice[cid_fn] = make(map[string]int)
	}
	sn.PendingACPutDSNotice[cid_fn][dsno] = 1
	sn.PACNMutex.Unlock()
	//阻塞监测分片是否已完成存储
	iscomplete := 1
	for {
		sn.PACNMutex.RLock()
		iscomplete = sn.PendingACPutDSNotice[cid_fn][dsno]
		sn.PACNMutex.RUnlock()
		if iscomplete == 2 {
			break
		}
	}
	//分片完成存储，则删除pending元素，给审计方返回消息
	sn.PACNMutex.Lock()
	delete(sn.PendingACPutDSNotice[cid_fn], dsno)
	sn.PACNMutex.Unlock()
	//获取分片版本号和时间戳
	sn.FSMMMutex.RLock()
	version := sn.FileShardsMap[cid_fn][dsno].Version
	timestamp := sn.FileShardsMap[cid_fn][dsno].Timestamp
	sn.FSMMMutex.RUnlock()
	return &pb.ClientStorageResponse{ClientId: clientId, Filename: filename, Dsno: dsno, Version: version, Timestamp: timestamp, Message: sn.SNId + " completes the storage of datashard" + dsno + "."}, nil
}

// 【供客户端使用的RPC】
func (sn *StorageNode) GetDataShard(ctx context.Context, req *pb.GetDSRequest) (*pb.GetDSResponse, error) {
	log.Printf("Received get datashard message from client: %s\n", req.ClientId)
	cid_fn := req.ClientId + "-" + req.Filename
	sn.FSMMMutex.RLock()
	if sn.FileShardsMap[cid_fn] == nil {
		sn.FSMMMutex.RUnlock()
		e := errors.New("client file not exist")
		return &pb.GetDSResponse{Filename: req.Filename, DatashardSerialized: nil}, e
	} else if sn.FileShardsMap[cid_fn][req.Dsno] == nil {
		sn.FSMMMutex.RUnlock()
		e := errors.New("datashard not exist")
		return &pb.GetDSResponse{Filename: req.Filename, DatashardSerialized: nil}, e
	} else {
		// //制造一个故障：dsno为d2时沉默
		// if req.Dsno == "d-2" || req.Dsno == "d-5" {
		// 	sn.FSMMMutex.RUnlock()
		// 	return &pb.GetDSResponse{Filename: req.Filename, DatashardSerialized: nil}, nil
		// }
		// //制造一个故障：dsno为校验块序号且既不是p9也不是p10时沉默
		// if strings.HasPrefix(req.Dsno, "p") && (req.Dsno != "p-9" && req.Dsno != "p-10") {
		// 	sn.FSMMMutex.RUnlock()
		// 	return &pb.GetDSResponse{Filename: req.Filename, DatashardSerialized: nil}, nil
		// }
		seds := sn.FileShardsMap[cid_fn][req.Dsno].SerializeDS()
		sn.FSMMMutex.RUnlock()
		return &pb.GetDSResponse{Filename: req.Filename, DatashardSerialized: seds}, nil
	}
}

// 打印storage node
func (sn *StorageNode) PrintSN() {
	str := "StorageNode:{SNId:" + sn.SNId + ",SNAddr:" + sn.SNAddr + ",ClientFileMap:{"
	for key, value := range sn.ClientFileMap {
		str = str + key + ":{"
		for i := 0; i < len(value); i++ {
			str = str + value[i] + ","
		}
		str = str + "},"
	}
	str = str + "},FileShardsMap:{"
	for key, value := range sn.FileShardsMap {
		str = str + key + ":{"
		for skey, _ := range value {
			str = str + skey + ","
		}
		str = str + "},"
	}
	str = str + "}}"
	log.Println(str)
}
