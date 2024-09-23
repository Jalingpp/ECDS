package nodes

import (
	"ECDS/encode"
	"ECDS/pdp"
	pb "ECDS/proto"
	"ECDS/util"
	"context"
	"errors"
	"log"
	"net"
	"strings"
	"sync"

	"google.golang.org/grpc"
)

type StorageNode struct {
	SNId                              string                                             //存储节点id
	SNAddr                            string                                             //存储节点ip地址
	Params                            string                                             //用于生成pairing对象的参数，由auditor提供，所有角色一致
	G                                 []byte                                             //用于签名、验签、举证、验证等的公钥，由auditor提供，所有角色一致
	ClientPK                          map[string][]byte                                  //客户端的公钥信息，key:clientID,value:公钥
	CPKMutex                          sync.RWMutex                                       //ClientPK的读写锁
	ClientFileMap                     map[string][]string                                //为客户端存储的文件列表，key:clientID,value:filename
	CFMMutex                          sync.RWMutex                                       //ClientFileMap的读写锁
	FileShardsMap                     map[string]map[string]*util.DataShard              //文件的数据分片列表，key:clientID-filename,value:(key:分片序号)
	FSMMMutex                         sync.RWMutex                                       //FileShardsMap的读写锁
	pb.UnimplementedSNServiceServer                                                      // 面向客户端的服务器嵌入匿名字段
	pb.UnimplementedSNACServiceServer                                                    // 面向审计方的服务器嵌入匿名字段
	PendingACPutDSNotice              map[string]map[string]int                          //用于暂存来自AC的分片存储通知，key:clientid-filename,value:分片序号dsno的列表,int:1表示该分片在等待存储，2表示该分片完成存储
	PACNMutex                         sync.RWMutex                                       //用于限制PendingACPutDSNotice访问的锁
	PendingACUpdDSNotice              map[string]map[string]int                          //用于暂存来自AC的更新分片通知，key:clientid-filename,value:分片序号dsno的列表,int:1表示该分片在等待存储，2表示该分片完成更新
	PACUDSNMutex                      sync.RWMutex                                       //用于限制PendingACUpdDSNotice访问的锁
	AuditorDSQueue                    map[string]map[string]map[string][]*util.DataShard //用于记录预审计请求中选中举证的分片，key:auditNo,clientId,filename;value:ds列表
	ADSQMutex                         sync.RWMutex                                       //AuditorDSQueue的读写锁
}

// 新建存储分片
func NewStorageNode(snid string, snaddr string) *StorageNode {
	clientFileMap := make(map[string][]string)
	fileShardsMap := make(map[string]map[string]*util.DataShard)
	pacpdsn := make(map[string]map[string]int)
	pacudsn := make(map[string]map[string]int)
	cpi := make(map[string][]byte)
	adsq := make(map[string]map[string]map[string][]*util.DataShard)
	sn := &StorageNode{snid, snaddr, "", nil, cpi, sync.RWMutex{}, clientFileMap, sync.RWMutex{}, fileShardsMap, sync.RWMutex{}, pb.UnimplementedSNServiceServer{}, pb.UnimplementedSNACServiceServer{}, pacpdsn, sync.RWMutex{}, pacudsn, sync.RWMutex{}, adsq, sync.RWMutex{}}
	// sn := &StorageNode{snid, snaddr, "", nil, cpi, sync.RWMutex{}, clientFileMap, sync.RWMutex{}, fileShardsMap, sync.RWMutex{}, pb.UnimplementedSNServiceServer{}, pb.UnimplementedSNACServiceServer{}, adsq, sync.RWMutex{}}
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
func (sn *StorageNode) ClientRegisterSN(ctx context.Context, req *pb.CRegistSNRequest) (*pb.CRegistSNResponse, error) {
	log.Printf("Received regist message from client: %s\n", req.ClientId)
	message := ""
	var e error
	sn.CPKMutex.Lock()
	if sn.ClientPK[req.ClientId] != nil {
		message = "client's publickey has already exist!"
		e = errors.New("client's publickey already exist")
	} else {
		sn.ClientPK[req.ClientId] = req.PK
		message = "client registration successful!"
		e = nil
	}
	sn.CPKMutex.Unlock()
	return &pb.CRegistSNResponse{ClientId: req.ClientId, Message: message}, e
}

// 【供auditor使用的RPC】审计方注册，存储方记录审计方的公开信息，包括构建pairing的参数和公钥
func (sn *StorageNode) ACRegisterSN(ctx context.Context, req *pb.ACRegistSNRequest) (*pb.ACRegistSNResponse, error) {
	log.Printf("Received regist message from auditor.\n")
	sn.Params = req.Params
	sn.G = req.G
	message := sn.SNId + " params and g already written"
	return &pb.ACRegistSNResponse{Message: message}, nil
}

// 【供客户端使用的RPC】存储节点验证分片序号是否与审计方通知的一致，验签，存放数据分片，告知审计方已存储或存储失败，给客户端回复消息
func (sn *StorageNode) PutDataShard(ctx context.Context, preq *pb.PutDSRequest) (*pb.PutDSResponse, error) {
	// log.Printf("Received message from client: %s\n", preq.ClientId)
	clientId := preq.ClientId
	filename := preq.Filename
	dsno := preq.Dsno
	message := ""
	cid_fn := clientId + "-" + filename
	//1-阻塞等待收到审计方通知
	for {
		sn.PACNMutex.RLock()
		v1, ok1 := sn.PendingACPutDSNotice[cid_fn]
		if ok1 {
			// 验证分片序号是否与审计方通知的一致
			_, ok2 := v1[dsno]
			if ok2 {
				sn.PACNMutex.RUnlock()
				break
			}
		}
		sn.PACNMutex.RUnlock()
	}
	//2-存放数据分片
	//2-1-提取数据分片对象
	ds, err := util.DeserializeDS(preq.DatashardSerialized)
	if err != nil {
		// log.Println("Deserialize DataShard Error!")
		message = "Deserialize DataShard Error!"
		e := errors.New("deserialize dataShard error")
		return &pb.PutDSResponse{Filename: preq.Filename, Dsno: dsno, Message: message}, e
	}
	//2-2-验签
	// pairing, _ := pbc.NewPairingFromString(sn.Params)
	var pk []byte
	sn.CPKMutex.RLock()
	if sn.ClientPK[clientId] == nil {
		sn.CPKMutex.RUnlock()
		message = "client not regist!"
		e := errors.New("client not regist")
		return &pb.PutDSResponse{Filename: preq.Filename, Dsno: dsno, Message: message}, e
	} else {
		pk = sn.ClientPK[clientId]
		sn.CPKMutex.RUnlock()
	}
	if !pdp.VerifySig(sn.Params, sn.G, pk, ds.Data, ds.Sig, ds.Version, ds.Timestamp) {
		message = "signature verify error!"
		log.Println(filename, ds.DSno, message)
		// e := errors.New("signature verify error")
		// return &pb.PutDSResponse{Filename: preq.Filename, Dsno: dsno, Message: message}, e
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
	// 4-告知审计方分片放置结果
	// log.Println("Completed record datashard ", dsno, " of file ", filename, ".")
	return &pb.PutDSResponse{Filename: preq.Filename, Dsno: dsno, Message: message}, nil
}

// 【供审计方使用的RPC】存储节点接收审计方分片存放通知，阻塞等待客户端分片存放，完成后回复审计方
func (sn *StorageNode) PutDataShardNotice(ctx context.Context, preq *pb.ClientStorageRequest) (*pb.ClientStorageResponse, error) {
	// log.Printf("Received put datashard notice message from auditor.\n")
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
	// log.Printf("Received get datashard message from client: %s\n", req.ClientId)
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

// 【供客户端使用的RPC】存储节点验证分片序号是否与审计方通知的一致，验签，更新数据分片，告知审计方已更新或更新失败，给客户端回复消息
func (sn *StorageNode) UpdateDataShards(ctx context.Context, req *pb.UpdDSsRequest) (*pb.UpdDSsResponse, error) {
	// log.Printf("Received update datashard message from client: %s\n", req.ClientId)
	clientId := req.ClientId
	filename := req.Filename
	message := ""
	cid_fn := clientId + "-" + filename
	var updatedDSno []string
	var pk []byte
	// pairing, _ := pbc.NewPairingFromString(sn.Params)
	sn.CPKMutex.RLock()
	if sn.ClientPK[clientId] == nil {
		sn.CPKMutex.RUnlock()
		message = "client not regist!"
		e := errors.New("client not regist")
		return &pb.UpdDSsResponse{Filename: req.Filename, Dsnos: updatedDSno, Message: message}, e
	} else {
		pk = sn.ClientPK[clientId]
		sn.CPKMutex.RUnlock()
	}
	//分别更新每个分片
	for i := 0; i < len(req.Dsnos); i++ {
		dsno := req.Dsnos[i]
		//1-阻塞等待收到审计方通知
		for {
			sn.PACUDSNMutex.RLock()
			v1, ok1 := sn.PendingACUpdDSNotice[cid_fn]
			if ok1 {
				// 验证分片序号是否与审计方通知的一致
				_, ok2 := v1[dsno]
				if ok2 {
					sn.PACUDSNMutex.RUnlock()
					break
				}
			}
			sn.PACUDSNMutex.RUnlock()
		}
		//2-更新数据分片
		//2-1-提取新数据分片对象
		newds, err := util.DeserializeDS(req.DatashardsSerialized[i])
		if err != nil {
			// log.Println("Deserialize DataShard Error!")
			message = "Deserialize DataShard Error!"
			e := errors.New("deserialize dataShard error")
			return &pb.UpdDSsResponse{Filename: req.Filename, Dsnos: updatedDSno, Message: message}, e
		}
		//2-2-验签
		if !pdp.VerifySig(sn.Params, sn.G, pk, newds.Data, newds.Sig, newds.Version, newds.Timestamp) {
			message = "signature verify error!"
			e := errors.New("signature verify error")
			return &pb.UpdDSsResponse{Filename: req.Filename, Dsnos: updatedDSno, Message: message}, e
		}
		//2-3-更新文件分片列表
		sn.FSMMMutex.Lock()
		if sn.FileShardsMap[cid_fn] == nil {
			message = "file not exist!"
			e := errors.New("file not exist")
			return &pb.UpdDSsResponse{Filename: req.Filename, Dsnos: updatedDSno, Message: message}, e
		} else if sn.FileShardsMap[cid_fn][dsno] == nil {
			message = "datashard not exist!"
			e := errors.New("datashard not exist")
			return &pb.UpdDSsResponse{Filename: req.Filename, Dsnos: updatedDSno, Message: message}, e
		}
		sn.FileShardsMap[cid_fn][dsno] = newds
		updatedDSno = append(updatedDSno, dsno)
		sn.FSMMMutex.Unlock()
		//3-修改PendingACUpdDSNotice
		sn.PACUDSNMutex.Lock()
		sn.PendingACUpdDSNotice[cid_fn][dsno] = 2
		sn.PACUDSNMutex.Unlock()
	}
	message = "Update datashards Success!"
	//4-告知审计方分片更新结果
	// log.Println("Completed datashards update of file", filename, ".")
	return &pb.UpdDSsResponse{Filename: req.Filename, Dsnos: updatedDSno, Message: message}, nil
}

// 【供审计方使用的RPC】存储节点接收审计方分片存放通知，阻塞等待客户端分片存放，完成后回复审计方
func (sn *StorageNode) UpdateDataShardNotice(ctx context.Context, preq *pb.ClientUpdDSRequest) (*pb.ClientUpdDSResponse, error) {
	// log.Printf("Received update datashard notice message from auditor.\n")
	clientId := preq.ClientId
	filename := preq.Filename
	dsno := preq.Dsno
	cid_fn := clientId + "-" + filename
	//写来自审计方的分片更新通知
	sn.PACUDSNMutex.Lock()
	if sn.PendingACUpdDSNotice[cid_fn] == nil {
		sn.PendingACUpdDSNotice[cid_fn] = make(map[string]int)
	}
	sn.PendingACUpdDSNotice[cid_fn][dsno] = 1
	sn.PACUDSNMutex.Unlock()
	//阻塞监测分片是否已完成更新
	iscomplete := 1
	for {
		sn.PACUDSNMutex.RLock()
		iscomplete = sn.PendingACUpdDSNotice[cid_fn][dsno]
		// log.Println("【UpdateDataShardNotice】", sn.SNId, "iscomplete:", iscomplete)
		sn.PACUDSNMutex.RUnlock()
		if iscomplete == 2 {
			break
		}
	}
	//分片完成更新，则删除pending元素，给审计方返回消息
	sn.PACUDSNMutex.Lock()
	delete(sn.PendingACUpdDSNotice[cid_fn], dsno)
	sn.PACUDSNMutex.Unlock()
	//获取分片版本号和时间戳
	sn.FSMMMutex.RLock()
	version := sn.FileShardsMap[cid_fn][dsno].Version
	timestamp := sn.FileShardsMap[cid_fn][dsno].Timestamp
	// log.Println("【UpdateDataShardNotice】", sn.SNId, cid_fn, dsno, "version:", version, "timestamp:", timestamp)
	sn.FSMMMutex.RUnlock()
	return &pb.ClientUpdDSResponse{ClientId: clientId, Filename: filename, Dsno: dsno, Version: version, Timestamp: timestamp, Message: sn.SNId + " completes the update of datashard" + dsno + "."}, nil
}

// 【供客户端使用的RPC】存放校验分片的增量分片
func (sn *StorageNode) PutIncParityShards(ctx context.Context, req *pb.PutIPSRequest) (*pb.PutIPSResponse, error) {
	// log.Printf("Received inc parityshard message from client: %s\n", req.ClientId)
	cid_fn := req.ClientId + "-" + req.Filename
	var pk []byte
	sn.CPKMutex.RLock()
	if sn.ClientPK[req.ClientId] == nil {
		// log.Println("client publickey not exist")
		e := errors.New("client publickey not exist")
		return &pb.PutIPSResponse{Filename: req.Filename, Psno: req.Psno, PosSerialized: nil}, e
	} else {
		pk = sn.ClientPK[req.ClientId]
	}
	sn.CPKMutex.RUnlock()
	for i := 0; i < len(req.DatashardsSerialized); i++ {
		incPS, err := util.DeserializeDS(req.DatashardsSerialized[i])
		if err != nil {
			// log.Println("deserialize inc parityshard error:", err)
			e := errors.New("deserialize inc parityshard error")
			return &pb.PutIPSResponse{Filename: req.Filename, Psno: req.Psno, PosSerialized: nil}, e
		}
		//1-更新校验块
		//1.1-获取旧的校验块
		var oldPS *util.DataShard
		sn.FSMMMutex.RLock()
		if sn.FileShardsMap[cid_fn] == nil {
			sn.FSMMMutex.RUnlock()
			e := errors.New("client file not exist")
			return &pb.PutIPSResponse{Filename: req.Filename, Psno: req.Psno, PosSerialized: nil}, e
		}
		if sn.FileShardsMap[cid_fn][req.Psno] == nil {
			sn.FSMMMutex.RUnlock()
			e := errors.New("parityshard not exist")
			return &pb.PutIPSResponse{Filename: req.Filename, Psno: req.Psno, PosSerialized: nil}, e
		}
		oldPS = sn.FileShardsMap[cid_fn][req.Psno]
		sn.FSMMMutex.RUnlock()
		//1.2-更新校验块，指针就地更新
		sn.FSMMMutex.Lock()
		encode.UpdateParityShard(oldPS, incPS, sn.Params)
		sn.FSMMMutex.Unlock()
	}
	sn.FSMMMutex.RLock()
	newPS := sn.FileShardsMap[cid_fn][req.Psno]
	sn.FSMMMutex.RUnlock()
	//2-生成更新后的校验块的存储证明
	pos := pdp.ProvePos(sn.Params, sn.G, pk, newPS, req.Randomnum)
	//3-阻塞等待审计方修改审计，修改PendingACPutDSNotice
	for {
		sn.PACUDSNMutex.Lock()
		if sn.PendingACUpdDSNotice[cid_fn] != nil {
			if _, ok := sn.PendingACUpdDSNotice[cid_fn][req.Psno]; ok {
				sn.PendingACUpdDSNotice[cid_fn][req.Psno] = 2
				sn.PACUDSNMutex.Unlock()
				break
			}
		}
		sn.PACUDSNMutex.Unlock()
	}

	return &pb.PutIPSResponse{Filename: req.Filename, Psno: req.Psno, PosSerialized: pdp.SerializePOS(pos)}, nil
}

// 【供审计方使用的RPC】预审计请求处理
func (sn *StorageNode) PreAuditSN(ctx context.Context, req *pb.PASNRequest) (*pb.PASNResponse, error) {
	// log.Printf("Received preaudit message from auditor.\n")
	if req.Snid != sn.SNId {
		e := errors.New("snid in preaudit request not consist with " + sn.SNId)
		return nil, e
	}
	readyDSMap := make(map[string]map[string][]*util.DataShard) //已经准备好的分片，key:clientId,filename;value:分片列表
	var rDSMMutex sync.Mutex
	unreadyDSVMap := make(map[string]int32) //未准备好的分片，即审计方请求已过时，key:clientId-filename-dsno,value:版本号
	var urDSVMMutex sync.Mutex
	//遍历审计表，判断是否满足审计方的快照要求
	done := make(chan struct{})
	for cid_fn_dsno, version := range req.Dsversion {
		cfdsplit := strings.Split(cid_fn_dsno, "-") //clientId,filename,dsno
		dsno := cfdsplit[2] + "-" + cfdsplit[3]
		go func(cid string, fn string, dsno string, version int32) {
			cid_fn_dsno := cid + "-" + fn + "-" + dsno
			for {
				//获取一个版本号不小于预审计要求的快照
				dssnapshot, _ := sn.GetDSSTNoLessV(cid, fn, dsno, version)
				//版本与预审请求一致，则加入ready列表；大于预审请求，则同时加入ready和unready列表；小于预审请求，则等待执行到与预审一致
				if dssnapshot != nil {
					rDSMMutex.Lock()
					if readyDSMap[cid] == nil {
						readyDSMap[cid] = make(map[string][]*util.DataShard)
					}
					if readyDSMap[cid][fn] == nil {
						readyDSMap[cid][fn] = make([]*util.DataShard, 0)
					}
					readyDSMap[cid][fn] = append(readyDSMap[cid][fn], dssnapshot)
					rDSMMutex.Unlock()
					if dssnapshot.Version > version {
						urDSVMMutex.Lock()
						unreadyDSVMap[cid_fn_dsno] = dssnapshot.Version
						urDSVMMutex.Unlock()
					}
					break
				}
			}
			// 通知主线程任务完成
			done <- struct{}{}
		}(cfdsplit[0], cfdsplit[1], dsno, version)
	}
	// 等待所有协程完成
	for i := 0; i < len(req.Dsversion); i++ {
		<-done
	}
	sn.ADSQMutex.Lock()
	sn.AuditorDSQueue[req.Auditno] = readyDSMap
	sn.ADSQMutex.Unlock()
	return &pb.PASNResponse{Isready: len(unreadyDSVMap) == 0, Dsversion: unreadyDSVMap}, nil
}

// 获取一个不小于给定版本的分片快照
func (sn *StorageNode) GetDSSTNoLessV(cid string, fn string, dsno string, version int32) (*util.DataShard, error) {
	cid_fn := cid + "-" + fn
	sn.FSMMMutex.RLock()
	if sn.FileShardsMap[cid_fn] == nil {
		sn.FSMMMutex.RUnlock()
		e := errors.New("client-filename not exist")
		return nil, e
	}
	if sn.FileShardsMap[cid_fn][dsno] == nil {
		sn.FSMMMutex.RUnlock()
		e := errors.New("datashard" + cid_fn + "-" + dsno + " not exist")
		return nil, e
	}
	if sn.FileShardsMap[cid_fn][dsno].Version < version {
		sn.FSMMMutex.RUnlock()
		// log.Println("current ds version less than given version")
		return nil, nil
	}
	ds := sn.FileShardsMap[cid_fn][dsno]
	data := make([]int32, len(ds.Data))
	copy(data, ds.Data)
	sig := make([]byte, len(ds.Sig))
	copy(sig, ds.Sig)
	v := ds.Version
	t := ds.Timestamp
	sn.FSMMMutex.RUnlock()
	return util.NewDataShard(dsno, data, sig, v, t), nil
}

// 【供审计方使用的RPC】获取存储节点上所有存储分片的聚合存储证明
func (sn *StorageNode) GetAggPosSN(ctx context.Context, req *pb.GAPSNRequest) (*pb.GAPSNResponse, error) {
	// log.Printf("Received get aggpos message from auditor.\n")
	//获取dssmap map[string]map[string][]*util.DataShard
	sn.ADSQMutex.RLock()
	dssmap := sn.AuditorDSQueue[req.Auditno]
	sn.ADSQMutex.RUnlock()
	if len(dssmap) == 0 {
		return nil, nil
	}
	//生成聚合证明
	// log.Println(sn.Params, sn.G, sn.ClientPK, dssmap, req.Random)
	aggpos := pdp.ProveAggPos(sn.Params, sn.G, sn.ClientPK, dssmap, req.Random)
	return &pb.GAPSNResponse{Aggpos: pdp.SerializeAggPOS(aggpos)}, nil
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
