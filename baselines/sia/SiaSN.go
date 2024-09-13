package baselines

import (
	pb "ECDS/proto" // 根据实际路径修改
	"ECDS/util"
	"context"
	"errors"
	"log"
	"net"
	"sync"

	"google.golang.org/grpc"
)

type SiaSN struct {
	SNId                                 string                                     //存储节点id
	SNAddr                               string                                     //存储节点ip地址
	ClientDSHashMap                      map[string][][]byte                        //为客户端存储的文件列表，key:clientID,value:数据分片的哈希值列表
	ClinetDSHashIndexMap                 map[string]int                             //分片在ClientDSHashMap某个列表中的索引号，key:clientID-filename-i,value:索引号
	ClientMerkleRootMap                  map[string][]byte                          //客户端所有分片构建成的Merkel树根最新哈希值，key:clientID,value:hash value of root
	CFMMutex                             sync.RWMutex                               //ClientFileMap的读写锁
	FileShardsMap                        map[string][]int32                         //文件的数据分片列表，key:clientID-filename-i,value:分片
	FileVersionMap                       map[string]int                             //文件的版本号，key:clientID-filename-i,value:版本号
	FSLRMMMutex                          sync.RWMutex                               //FileShardsMap，FileLeavesMap，FileRootMap的读写锁
	pb.UnimplementedSiaSNServiceServer                                              // 面向客户端的服务器嵌入匿名字段
	pb.UnimplementedSiaSNACServiceServer                                            // 面向审计方的服务器嵌入匿名字段
	PendingACPutFNotice                  map[string]int                             //用于暂存来自AC的文件存储通知，key:clientid-filename-i,value:1表示该文件在等待存储，2表示该文件完成存储
	PACNMutex                            sync.RWMutex                               //用于限制PendingACPutDSNotice访问的锁
	PendingACUpdFNotice                  map[string]int                             //用于暂存来自AC的文件更新通知，key:clientid-filename-i,value:1表示该文件在等待更新，2表示该文件完成更新
	PendingACUpdFV                       map[string]int                             //用于暂存来自AC的文件更新版本号，key:clientID-filename-i,value:版本号
	PACUFNMutex                          sync.RWMutex                               //用于限制PendingACUpdFNotice访问的锁
	AuditorFileQueue                     map[string]map[string]map[string][][]int32 //待审计的文件分片，key:审计号，subkey:currpcno,subsubkey:cid-fn-i,subsubvalue:文件分片
	AFQMutex                             sync.RWMutex                               //AuditorFileQueue的读写锁
}

// 新建存储分片
func NewSiaSN(snid string, snaddr string) *SiaSN {
	clientdshMap := make(map[string][][]byte)
	clientdshiMap := make(map[string]int)
	clientmrMap := make(map[string][]byte)
	fileShardsMap := make(map[string][]int32)
	fileversionMap := make(map[string]int)
	pacpfn := make(map[string]int)
	pacufn := make(map[string]int)
	pacufv := make(map[string]int)
	afq := make(map[string]map[string]map[string][][]int32)
	sn := &SiaSN{snid, snaddr, clientdshMap, clientdshiMap, clientmrMap, sync.RWMutex{}, fileShardsMap, fileversionMap, sync.RWMutex{}, pb.UnimplementedSiaSNServiceServer{}, pb.UnimplementedSiaSNACServiceServer{}, pacpfn, sync.RWMutex{}, pacufn, pacufv, sync.RWMutex{}, afq, sync.RWMutex{}} //设置监听地址
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
	sn.FSLRMMMutex.Lock()
	if sn.FileShardsMap[cid_fn] != nil {
		message = "Filename Already Exist!"
		e := errors.New("filename already exist")
		return &pb.SiaPutFResponse{Filename: preq.Filename, Dsno: preq.Dsno, Message: message}, e
	}
	sn.FileShardsMap[cid_fn] = preq.DataShard
	sn.FileVersionMap[preq.ClientId+"-"+preq.Filename] = int(preq.Version)
	sn.FSLRMMMutex.Unlock()
	//2-3-放置客户端文件名列表
	sn.CFMMutex.Lock()
	if sn.ClientDSHashMap[clientId] == nil {
		sn.ClientDSHashMap[clientId] = make([][]byte, 0)
	}
	//对分片取哈希
	dshash := util.Hash([]byte(util.Int32SliceToStr(preq.DataShard)))
	sn.ClientDSHashMap[clientId] = append(sn.ClientDSHashMap[clientId], dshash)
	sn.ClinetDSHashIndexMap[cid_fn] = len(sn.ClientDSHashMap[clientId]) - 1
	sn.CFMMutex.Unlock()
	message = "Put File Success!"
	//3-修改PendingACPutDSNotice
	sn.PACNMutex.Lock()
	sn.PendingACPutFNotice[cid_fn] = 2
	sn.PACNMutex.Unlock()
	// 4-告知审计方分片放置结果
	return &pb.SiaPutFResponse{Filename: preq.Filename, Dsno: preq.Dsno, Message: message}, nil
}

// 【供审计方使用的RPC】存储节点接收审计方文件存放通知，阻塞等待客户端存放文件，完成后回复审计方
func (sn *SiaSN) SiaPutFileNotice(ctx context.Context, preq *pb.SiaClientStorageRequest) (*pb.SiaClientStorageResponse, error) {
	clientId := preq.ClientId
	filename := preq.Filename
	dsno := preq.Dsno
	cid_fn := clientId + "-" + filename + "-" + dsno
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
	//文件完成存储，则删除pending元素，给审计方返回消息
	sn.PACNMutex.Lock()
	delete(sn.PendingACPutFNotice, cid_fn)
	sn.PACNMutex.Unlock()
	//计算Merkel根节点哈希，并获取dsno分片对应的验证路径
	sn.CFMMutex.Lock()
	leafHashes := sn.ClientDSHashMap[preq.ClientId]
	index := sn.ClinetDSHashIndexMap[cid_fn]
	// fmt.Println("leafHashes:", leafHashes)
	root, paths := util.BuildMerkleTreeAndGeneratePath(leafHashes, index)
	// fmt.Println("dsno:", dsno, "root:", root, "paths:", paths, "index:", index)
	sn.ClientMerkleRootMap[preq.ClientId] = root
	sn.CFMMutex.Unlock()
	return &pb.SiaClientStorageResponse{ClientId: clientId, Filename: filename, Dsno: dsno, Version: 1, Merklepath: paths, Root: root, Index: int32(index)}, nil
}

// 【供客户端使用的RPC】
func (sn *SiaSN) SiaGetFileDS(ctx context.Context, req *pb.SiaGetFRequest) (*pb.SiaGetFResponse, error) {
	cid_fni := req.ClientId + "-" + req.Filename + "-" + req.Dsno
	cid_fn := req.ClientId + "-" + req.Filename
	sn.FSLRMMMutex.RLock()
	if sn.FileShardsMap[cid_fni] == nil {
		sn.FSLRMMMutex.RUnlock()
		e := errors.New("datashard not exist")
		return nil, e
	} else {
		seds := sn.FileShardsMap[cid_fni]
		version := sn.FileVersionMap[cid_fn]
		sn.FSLRMMMutex.RUnlock()
		//构建数据分片的存储证明
		sn.CFMMutex.RLock()
		leafHashes := sn.ClientDSHashMap[req.ClientId]
		index := sn.ClinetDSHashIndexMap[cid_fni]
		_, paths := util.BuildMerkleTreeAndGeneratePath(leafHashes, index)
		sn.CFMMutex.RUnlock()
		return &pb.SiaGetFResponse{Filename: req.Filename, Version: int32(version), DataShard: seds, Merklepath: paths, Index: int32(index)}, nil
	}
}
