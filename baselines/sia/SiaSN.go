package baselines

import (
	pb "ECDS/proto" // 根据实际路径修改
	"ECDS/util"
	"context"
	"errors"
	"log"
	"net"
	"strings"
	"sync"

	"google.golang.org/grpc"
)

type SiaSN struct {
	SNId                                 string                               //存储节点id
	SNAddr                               string                               //存储节点ip地址
	ClientDSHashMap                      map[string][][]byte                  //为客户端存储的文件列表，key:clientID,value:数据分片的哈希值列表
	ClientDSHashIndexMap                 map[string]int                       //分片在ClientDSHashMap某个列表中的索引号，key:clientID-filename-i,value:索引号
	FileShardsMap                        map[string][]int32                   //文件的数据分片列表，key:clientID-filename-i,value:分片
	FileVersionMap                       map[string]int                       //文件的版本号，key:clientID-filename-i,value:版本号
	ClientMerkleRootMap                  map[string][]byte                    //客户端所有分片构建成的Merkel树根最新哈希值，key:clientID,value:hash value of root
	ClientMerkleRootTimeMap              map[string]int                       //最新根节点哈希值的时间戳，key:clientID,value:时间戳
	CFMMutex                             sync.RWMutex                         //ClientFileMap的读写锁
	pb.UnimplementedSiaSNServiceServer                                        // 面向客户端的服务器嵌入匿名字段
	pb.UnimplementedSiaSNACServiceServer                                      // 面向审计方的服务器嵌入匿名字段
	PendingACPutFNotice                  map[string]int                       //用于暂存来自AC的文件存储通知，key:clientid-filename-i,value:1表示该文件在等待存储，2表示该文件完成存储
	PendingACPutPath                     map[string][][]byte                  //用于暂存待确认的分片验证路径，key:client-filename-i,value:paths
	PendingACPutRoot                     map[string][]byte                    //用于暂存待确认的分片验证根节点哈希，key:client-filename-i,value:roothash
	PendingACPutRootTime                 map[string]int                       //用于暂存待确认的分片验证根节点时间戳，key:client-filename-i,value:roottime
	PACNMutex                            sync.RWMutex                         //用于限制PendingACPutDSNotice访问的锁
	PendingACUpdFNotice                  map[string]int                       //用于暂存来自AC的文件更新通知，key:clientid-filename-i,value:1表示该分片在等待更新，2表示该分片完成更新
	PendingACUpdPath                     map[string][][]byte                  //用于暂存待确认的分片验证路径，key:client-filename-i,value:paths
	PendingACUpdRoot                     map[string][]byte                    //用于暂存待确认的分片验证根节点哈希，key:client-filename-i,value:roothash
	PendingACUpdRootTime                 map[string]int                       //用于暂存待确认的分片验证根节点时间戳，key:client-filename-i,value:roottime
	PACUFNMutex                          sync.RWMutex                         //用于限制PendingACUpdFNotice访问的锁
	AuditorFileQueue                     map[string]map[string]*SiaAuditInfor //待审计的文件分片，key:审计号，subkey:currpcno,subsubkey:cid-fn-i,subsubvalue:文件分片
	AFQMutex                             sync.RWMutex                         //AuditorFileQueue的读写锁
}

// 新建存储分片
func NewSiaSN(snid string, snaddr string) *SiaSN {
	clientdshMap := make(map[string][][]byte)
	clientdshiMap := make(map[string]int)
	clientmrMap := make(map[string][]byte)
	clientmrtMap := make(map[string]int)
	fileShardsMap := make(map[string][]int32)
	fileversionMap := make(map[string]int)
	pacpfn := make(map[string]int)
	pacpp := make(map[string][][]byte)
	pacpt := make(map[string][]byte)
	pacptr := make(map[string]int)
	pacufn := make(map[string]int)
	pacupp := make(map[string][][]byte)
	pacut := make(map[string][]byte)
	pacutr := make(map[string]int)
	afq := make(map[string]map[string]*SiaAuditInfor)
	sn := &SiaSN{snid, snaddr, clientdshMap, clientdshiMap, fileShardsMap, fileversionMap, clientmrMap, clientmrtMap, sync.RWMutex{}, pb.UnimplementedSiaSNServiceServer{}, pb.UnimplementedSiaSNACServiceServer{}, pacpfn, pacpp, pacpt, pacptr, sync.RWMutex{}, pacufn, pacupp, pacut, pacutr, sync.RWMutex{}, afq, sync.RWMutex{}} //设置监听地址
	lis, err := net.Listen("tcp", snaddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterSiaSNServiceServer(s, sn)
	pb.RegisterSiaSNACServiceServer(s, sn)
	log.Println("Server listening on " + snaddr)
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	return sn
}

// 【供客户端使用的RPC】存储节点验证分片序号是否与审计方通知的一致，验签，存放数据分片，告知审计方已存储或存储失败，给客户端回复消息
func (sn *SiaSN) SiaPutFileDS(ctx context.Context, preq *pb.SiaPutFRequest) (*pb.SiaPutFResponse, error) {
	clientId := preq.ClientId
	filename := preq.Filename
	dsno := preq.Dsno
	message := ""
	cid_fn := clientId + "-" + filename + "-" + dsno
	// fmt.Println(sn.SNId, cid_fn, "开始SiaPutFileDS")
	//1-阻塞等待收到审计方通知
	for {
		sn.PACNMutex.RLock()
		_, ok1 := sn.PendingACPutFNotice[cid_fn]
		if ok1 {
			sn.PACNMutex.RUnlock()
			break
		}
		sn.PACNMutex.RUnlock()
	}
	//2-存放数据分片
	//2-2-放置文件分片和版本号
	sn.CFMMutex.Lock()
	if sn.FileShardsMap[cid_fn] != nil {
		sn.CFMMutex.Unlock()
		message = "Filename Already Exist!"
		e := errors.New("filename already exist")
		return &pb.SiaPutFResponse{Filename: preq.Filename, Dsno: preq.Dsno, Message: message}, e
	}
	sn.FileShardsMap[cid_fn] = preq.DataShard
	sn.FileVersionMap[cid_fn] = int(preq.Version)
	//2-3-放置客户端文件名列表
	if sn.ClientDSHashMap[clientId] == nil {
		sn.ClientDSHashMap[clientId] = make([][]byte, 0)
	}
	//对分片取哈希
	dshash := util.Hash([]byte(util.Int32SliceToStr(preq.DataShard)))
	sn.ClientDSHashMap[clientId] = append(sn.ClientDSHashMap[clientId], dshash)
	sn.ClientDSHashIndexMap[cid_fn] = len(sn.ClientDSHashMap[clientId]) - 1
	//计算Merkel根节点哈希，并获取dsno分片对应的验证路径
	leafHashes := sn.ClientDSHashMap[preq.ClientId]
	index := sn.ClientDSHashIndexMap[cid_fn]
	// sn.CFMMutex.Unlock()
	root, paths := util.BuildMerkleTreeAndGeneratePath(leafHashes, index)
	// sn.CFMMutex.Lock()
	oldtime := sn.ClientMerkleRootTimeMap[preq.ClientId]
	sn.ClientMerkleRootMap[preq.ClientId] = root
	sn.ClientMerkleRootTimeMap[preq.ClientId] = oldtime + 1
	sn.CFMMutex.Unlock()
	message = "Put File Success!"
	//3-修改PendingACPutDSNotice
	sn.PACNMutex.Lock()
	sn.PendingACPutFNotice[cid_fn] = 2
	sn.PendingACPutPath[cid_fn] = paths
	sn.PendingACPutRoot[cid_fn] = root
	sn.PendingACPutRootTime[cid_fn] = oldtime + 1
	sn.PACNMutex.Unlock()
	// fmt.Println(sn.SNId, cid_fn, "结束SiaPutFileDS")
	// 4-告知审计方分片放置结果
	return &pb.SiaPutFResponse{Filename: preq.Filename, Dsno: preq.Dsno, Message: message}, nil
}

// 【供审计方使用的RPC】存储节点接收审计方文件存放通知，阻塞等待客户端存放文件，完成后回复审计方
func (sn *SiaSN) SiaPutFileNotice(ctx context.Context, preq *pb.SiaClientStorageRequest) (*pb.SiaClientStorageResponse, error) {
	clientId := preq.ClientId
	filename := preq.Filename
	dsno := preq.Dsno
	cid_fn := clientId + "-" + filename + "-" + dsno
	// fmt.Println(sn.SNId, cid_fn, "开始SiaPutFileNotice")
	//写来自审计方的分片存储通知
	sn.PACNMutex.Lock()
	sn.PendingACPutFNotice[cid_fn] = 1
	sn.PACNMutex.Unlock()
	//阻塞监测分片是否已完成存储
	iscomplete := 1
	for {
		sn.PACNMutex.RLock()
		iscomplete = sn.PendingACPutFNotice[cid_fn]
		sn.PACNMutex.RUnlock()
		if iscomplete == 2 {
			break
		}
	}
	sn.CFMMutex.RLock()
	index := sn.ClientDSHashIndexMap[cid_fn]
	sn.CFMMutex.RUnlock()
	//文件完成存储，则删除pending元素，给审计方返回消息
	sn.PACNMutex.Lock()
	timestamp := sn.PendingACPutRootTime[cid_fn]
	paths := sn.PendingACPutPath[cid_fn]
	root := sn.PendingACPutRoot[cid_fn]
	delete(sn.PendingACPutFNotice, cid_fn)
	delete(sn.PendingACPutPath, cid_fn)
	delete(sn.PendingACPutRoot, cid_fn)
	delete(sn.PendingACPutRootTime, cid_fn)
	sn.PACNMutex.Unlock()
	return &pb.SiaClientStorageResponse{ClientId: clientId, Filename: filename, Dsno: dsno, Version: 1, Merklepath: paths, Root: root, Index: int32(index), Timestamp: int32(timestamp)}, nil
}

// 【供客户端使用的RPC】
func (sn *SiaSN) SiaGetFileDS(ctx context.Context, req *pb.SiaGetFRequest) (*pb.SiaGetFResponse, error) {
	cid_fni := req.ClientId + "-" + req.Filename + "-" + req.Dsno
	sn.CFMMutex.RLock()
	if sn.FileShardsMap[cid_fni] == nil {
		sn.CFMMutex.RUnlock()
		e := errors.New("datashard not exist")
		return nil, e
	} else {
		seds := sn.FileShardsMap[cid_fni]
		version := sn.FileVersionMap[cid_fni]
		//构建数据分片的存储证明
		leafHashes := sn.ClientDSHashMap[req.ClientId]
		index := sn.ClientDSHashIndexMap[cid_fni]
		sn.CFMMutex.RUnlock()
		_, paths := util.BuildMerkleTreeAndGeneratePath(leafHashes, index)
		return &pb.SiaGetFResponse{Filename: req.Filename, Version: int32(version), DataShard: seds, Merklepath: paths, Index: int32(index)}, nil
	}
}

func (sn *SiaSN) SiaUpdateFileDS(ctx context.Context, req *pb.SiaUpdDSRequest) (*pb.SiaUpdDSResponse, error) {
	clientId := req.ClientId
	filename := req.Filename
	dsno := req.Dsno
	message := ""
	cid_fn := clientId + "-" + filename + "-" + dsno
	//1-阻塞等待收到审计方通知
	for {
		sn.PACUFNMutex.RLock()
		_, ok1 := sn.PendingACUpdFNotice[cid_fn]
		if ok1 {
			sn.PACUFNMutex.RUnlock()
			break
		}
		sn.PACUFNMutex.RUnlock()
	}
	//2-更新数据分片
	//2-1-提取数据分片对象
	ds := req.Datashard
	//2-2-更新文件到各个列表中
	//2-存放数据分片
	//2-2-放置文件分片和版本号
	sn.CFMMutex.Lock()
	if sn.FileShardsMap[cid_fn] == nil {
		sn.CFMMutex.Unlock()
		message = "Filename Not Exist!"
		e := errors.New("filename not exist")
		return nil, e
	}
	sn.FileShardsMap[cid_fn] = ds
	sn.FileVersionMap[cid_fn] = int(req.Newversion)
	//2-3-放置客户端文件名列表
	if sn.ClientDSHashMap[clientId] == nil {
		sn.ClientDSHashMap[clientId] = make([][]byte, 0)
	}
	//对分片取哈希后更新至列表
	dshash := util.Hash([]byte(util.Int32SliceToStr(ds)))
	index := sn.ClientDSHashIndexMap[cid_fn]
	sn.ClientDSHashMap[clientId][index] = dshash
	leafHashes := sn.ClientDSHashMap[clientId]
	// sn.CFMMutex.Unlock()
	root, paths := util.BuildMerkleTreeAndGeneratePath(leafHashes, index)
	// sn.CFMMutex.Lock()
	oldtime := sn.ClientMerkleRootTimeMap[clientId]
	sn.ClientMerkleRootMap[clientId] = root
	sn.ClientMerkleRootTimeMap[clientId] = oldtime + 1
	sn.CFMMutex.Unlock()
	message = "Update DataShard Success!"
	//3-修改PendingACUpdFNotice
	sn.PACUFNMutex.Lock()
	sn.PendingACUpdFNotice[cid_fn] = 2
	sn.PendingACUpdPath[cid_fn] = paths
	sn.PendingACUpdRoot[cid_fn] = root
	sn.PendingACUpdRootTime[cid_fn] = oldtime + 1
	sn.PACUFNMutex.Unlock()
	// 4-告知审计方文件更新结果
	return &pb.SiaUpdDSResponse{Filename: req.Filename, Dsno: dsno, Message: message}, nil
}

// 【供审计方使用的RPC】存储节点接收审计方文件存放通知，阻塞等待客户端存放文件，完成后回复审计方
func (sn *SiaSN) SiaUpdateDataShardNotice(ctx context.Context, preq *pb.SiaClientUpdDSRequest) (*pb.SiaClientUpdDSResponse, error) {
	clientId := preq.ClientId
	filename := preq.Filename
	dsno := preq.Dsno
	cid_fn := clientId + "-" + filename + "-" + dsno
	//写来自审计方的分片存储通知
	sn.PACUFNMutex.Lock()
	sn.PendingACUpdFNotice[cid_fn] = 1
	sn.PACUFNMutex.Unlock()
	//阻塞监测分片是否已完成存储
	iscomplete := 1
	for {
		sn.PACUFNMutex.RLock()
		iscomplete = sn.PendingACUpdFNotice[cid_fn]
		sn.PACUFNMutex.RUnlock()
		if iscomplete == 2 {
			break
		}
	}
	sn.CFMMutex.RLock()
	index := sn.ClientDSHashIndexMap[cid_fn]
	version := sn.FileVersionMap[cid_fn]
	sn.CFMMutex.RUnlock()
	//文件完成存储，则删除pending元素，给审计方返回消息
	sn.PACUFNMutex.Lock()
	timestamp := sn.PendingACUpdRootTime[cid_fn]
	paths := sn.PendingACUpdPath[cid_fn]
	root := sn.PendingACUpdRoot[cid_fn]
	delete(sn.PendingACUpdFNotice, cid_fn)
	delete(sn.PendingACUpdPath, cid_fn)
	delete(sn.PendingACUpdRoot, cid_fn)
	delete(sn.PendingACUpdRootTime, cid_fn)
	sn.PACUFNMutex.Unlock()
	return &pb.SiaClientUpdDSResponse{ClientId: clientId, Filename: filename, Dsno: dsno, Version: int32(version), Merklepath: paths, Root: root, Index: int32(index), Timestamp: int32(timestamp)}, nil
}

type SiaAuditInfor struct {
	Key           string   //识别分片的key:clientId-filename-dsno
	Data          []int32  //分片数据
	Version       int32    //分片版本号
	RootHash      []byte   //根节点哈希值
	RootTimestamp int      //根节点哈希值时间戳
	Path          [][]byte //审计路径
	Index         int      //分片哈希的索引号
}

// 【供审计方使用的RPC】预审计请求处理
func (sn *SiaSN) SiaPreAuditSN(ctx context.Context, req *pb.SiaPASNRequest) (*pb.SiaPASNResponse, error) {
	if req.Snid != sn.SNId {
		e := errors.New("snid in preaudit request not consist with " + sn.SNId)
		return nil, e
	}
	readyDSMap := make(map[string]*SiaAuditInfor) //已经准备好的分片，key:clientId-filename-dsno;value:审计信息
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
			// fmt.Println(sn.SNId, req.Auditno, cid_fn_dsno, "basicRT:", basicRT, "basicDSHs:", basicDSHs)
			var dssnapshot *SiaAuditInfor
			for {
				//获取一个版本号不小于预审计要求的快照
				dssnapshot, _ = sn.SiaGetDSSTNoLessV(cid, fn, dsno, version)
				//版本与预审请求一致，则加入ready列表；大于预审请求，则同时加入ready和unready列表；小于预审请求，则等待执行到与预审一致
				if dssnapshot != nil {
					rDSMMutex.Lock()
					readyDSMap[cid_fn_dsno] = dssnapshot
					rDSMMutex.Unlock()
					if dssnapshot.Version > version {
						urDSVMMutex.Lock()
						unreadyDSVMap[cid_fn_dsno] = dssnapshot.Version
						urDSVMMutex.Unlock()
					}
					// fmt.Println(sn.SNId, dssnapshot.Key, dssnapshot.RootTimestamp)
					break
				}
				// fmt.Println(sn.SNId, req.Auditno, cid_fn_dsno, "dssnapshot==nil")
			}
			// 通知主线程任务完成
			done <- struct{}{}
		}(cfdsplit[0], cfdsplit[1], dsno, version)
	}
	// 等待所有协程完成
	for i := 0; i < len(req.Dsversion); i++ {
		<-done
	}
	sn.AFQMutex.Lock()
	sn.AuditorFileQueue[req.Auditno] = readyDSMap
	// fmt.Println(sn.SNId, req.Auditno, "sn.AuditorFileQueue[req.Auditno]")
	sn.AFQMutex.Unlock()
	// log.Println(sn.SNId, req.Auditno, "len(unreadyDSVMap):", len(unreadyDSVMap))
	return &pb.SiaPASNResponse{Isready: len(unreadyDSVMap) == 0, Dsversion: unreadyDSVMap}, nil
}

// 获取一个不小于给定版本的分片快照
func (sn *SiaSN) SiaGetDSSTNoLessV(cid string, fn string, dsno string, version int32) (*SiaAuditInfor, error) {
	cid_fni := cid + "-" + fn + "-" + dsno
	sn.CFMMutex.RLock()
	if sn.FileShardsMap[cid_fni] == nil {
		sn.CFMMutex.RUnlock()
		return nil, nil
	}
	v := sn.FileVersionMap[cid_fni]
	if v < int(version) {
		sn.CFMMutex.RUnlock()
		return nil, nil
	}
	index := sn.ClientDSHashIndexMap[cid_fni]
	rt := sn.ClientMerkleRootTimeMap[cid]
	ds := make([]int32, len(sn.FileShardsMap[cid_fni]))
	copy(ds, sn.FileShardsMap[cid_fni])
	hashes := sn.ClientDSHashMap[cid]
	root, path := util.BuildMerkleTreeAndGeneratePath(hashes, index)
	sn.CFMMutex.RUnlock()
	return &SiaAuditInfor{Key: cid_fni, Data: ds, Version: int32(v), RootHash: root, RootTimestamp: rt, Path: path, Index: index}, nil
}

// 【供审计方使用的RPC】获取存储节点上所有存储分片的聚合存储证明
func (sn *SiaSN) SiaGetPosSN(ctx context.Context, req *pb.SiaGAPSNRequest) (*pb.SiaGAPSNResponse, error) {
	//获取dssmap map[string]map[string][]*util.DataShard
	sn.AFQMutex.RLock()
	dssmap := sn.AuditorFileQueue[req.Auditno]
	sn.AFQMutex.RUnlock()
	if len(dssmap) == 0 {
		e := errors.New("auditno not exist")
		return nil, e
	}
	cid_fn_dsno := req.Cidfni
	if dssmap[cid_fn_dsno] == nil {
		e := errors.New("datashard audit infor not exist")
		return nil, e
	}
	//返回验证信息
	return &pb.SiaGAPSNResponse{Cidfni: dssmap[cid_fn_dsno].Key, Data: dssmap[cid_fn_dsno].Data, Version: dssmap[cid_fn_dsno].Version, Roothash: dssmap[cid_fn_dsno].RootHash, Roottimestamp: int32(dssmap[cid_fn_dsno].RootTimestamp), Path: dssmap[cid_fn_dsno].Path, Index: int32(dssmap[cid_fn_dsno].Index)}, nil
}
