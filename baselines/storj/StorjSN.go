package storjnodes

import (
	pb "ECDS/proto" // 根据实际路径修改
	"ECDS/util"
	"context"
	"errors"
	"log"
	"net"
	"strconv"
	"sync"

	"google.golang.org/grpc"
)

type StorjSN struct {
	SNId                                   string               //存储节点id
	SNAddr                                 string               //存储节点ip地址
	ClientFileMap                          map[string][]string  //为客户端存储的文件列表，key:clientID,value:filename
	CFMMutex                               sync.RWMutex         //ClientFileMap的读写锁
	FileShardsMap                          map[string][][]int32 //文件的数据分片列表，key:clientID-filename-i,value:分片
	FileLeavesMap                          map[string][][]byte  //文件的叶子节点列表，key:clientID-filename-i,value:叶子节点
	FileRootMap                            map[string][]byte    //文件的默克尔树根节点哈希值列表，key:clientID-filename-i,value:根节点哈希值
	FSLRMMMutex                            sync.RWMutex         //FileShardsMap，FileLeavesMap，FileRootMap的读写锁
	pb.UnimplementedStorjSNServiceServer                        // 面向客户端的服务器嵌入匿名字段
	pb.UnimplementedStorjSNACServiceServer                      // 面向审计方的服务器嵌入匿名字段
	PendingACPutFNotice                    map[string]int       //用于暂存来自AC的文件存储通知，key:clientid-filename-i,value:1表示该文件在等待存储，2表示该文件完成存储
	PACNMutex                              sync.RWMutex         //用于限制PendingACPutDSNotice访问的锁
}

// 新建存储分片
func NewStorjSN(snid string, snaddr string) *StorjSN {
	clientFileMap := make(map[string][]string)
	fileShardsMap := make(map[string][][]int32)
	fileleavesMap := make(map[string][][]byte)
	filerootMap := make(map[string][]byte)
	pacpfn := make(map[string]int)
	sn := &StorjSN{snid, snaddr, clientFileMap, sync.RWMutex{}, fileShardsMap, fileleavesMap, filerootMap, sync.RWMutex{}, pb.UnimplementedStorjSNServiceServer{}, pb.UnimplementedStorjSNACServiceServer{}, pacpfn, sync.RWMutex{}}
	//设置监听地址
	lis, err := net.Listen("tcp", snaddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterStorjSNServiceServer(s, sn)
	pb.RegisterStorjSNACServiceServer(s, sn)
	log.Println("Server listening on " + snaddr)
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	return sn
}

// 【供客户端使用的RPC】存储节点验证分片序号是否与审计方通知的一致，验签，存放数据分片，告知审计方已存储或存储失败，给客户端回复消息
func (sn *StorjSN) StorjPutFile(ctx context.Context, preq *pb.StorjPutFRequest) (*pb.StorjPutFResponse, error) {
	// log.Printf("Received message from client: %s\n", preq.ClientId)
	clientId := preq.ClientId
	filename := preq.Filename
	repno := preq.Repno
	message := ""
	cid_fn := clientId + "-" + filename + "-" + strconv.Itoa(int(repno))
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
	//2-1-提取数据分片对象
	dss := make([][]int32, 0)
	for i := 0; i < len(preq.DataShards); i++ {
		dsarray := preq.DataShards[i]
		ds := make([]int32, 0)
		for j := 0; j < len(dsarray.Values); j++ {
			ds = append(ds, dsarray.Values[j])
		}
		dss = append(dss, ds)
	}
	//2-2-放置文件到各个列表中
	sn.FSLRMMMutex.Lock()
	if sn.FileShardsMap[cid_fn] != nil {
		message = "Filename Already Exist!"
		e := errors.New("filename already exist")
		return &pb.StorjPutFResponse{Filename: preq.Filename, Message: message}, e
	}
	sn.FileShardsMap[cid_fn] = dss
	sn.FileLeavesMap[cid_fn] = preq.MerkleLeaves
	root := util.BuildMerkleTree(preq.MerkleLeaves)
	sn.FileRootMap[cid_fn] = root
	sn.FSLRMMMutex.Unlock()
	//2-3-放置客户端文件名列表
	sn.CFMMutex.Lock()
	if sn.ClientFileMap[clientId] == nil {
		sn.ClientFileMap[clientId] = make([]string, 0)
	}
	sn.ClientFileMap[clientId] = append(sn.ClientFileMap[clientId], filename)
	sn.CFMMutex.Unlock()
	message = "Put File Success!"
	//3-修改PendingACPutDSNotice
	sn.PACNMutex.Lock()
	sn.PendingACPutFNotice[cid_fn] = 2
	sn.PACNMutex.Unlock()
	// 4-告知审计方分片放置结果
	return &pb.StorjPutFResponse{Filename: preq.Filename, Root: root, Message: message}, nil
}

// 【供审计方使用的RPC】存储节点接收审计方文件存放通知，阻塞等待客户端存放文件，完成后回复审计方
func (sn *StorjSN) StorjPutFileNotice(ctx context.Context, preq *pb.StorjClientStorageRequest) (*pb.StorjClientStorageResponse, error) {
	clientId := preq.ClientId
	filename := preq.Filename
	repno := preq.Repno
	cid_fn := clientId + "-" + filename + "-" + strconv.Itoa(int(repno))
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
		log.Println(cid_fn, "iscomplete:", iscomplete)
		if iscomplete == 2 {
			break
		}
	}
	//文件完成存储，则删除pending元素，给审计方返回消息
	sn.PACNMutex.Lock()
	delete(sn.PendingACPutFNotice, cid_fn)
	sn.PACNMutex.Unlock()
	//获取文件根节点哈希
	sn.FSLRMMMutex.RLock()
	root := sn.FileRootMap[cid_fn]
	sn.FSLRMMMutex.RUnlock()
	log.Println(sn.SNId, "已接收通知", cid_fn)
	return &pb.StorjClientStorageResponse{ClientId: clientId, Filename: filename, Repno: repno, Root: root, Snid: sn.SNId, Message: sn.SNId + " completes the storage of " + cid_fn + "."}, nil
}
