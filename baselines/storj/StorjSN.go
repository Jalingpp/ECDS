package baselines

import (
	pb "ECDS/proto" // 根据实际路径修改
	"ECDS/util"
	"context"
	"encoding/binary"
	"errors"
	"log"
	"net"
	"strconv"
	"sync"

	"google.golang.org/grpc"
)

type StorjSN struct {
	SNId                                   string                                     //存储节点id
	SNAddr                                 string                                     //存储节点ip地址
	ClientFileMap                          map[string][]string                        //为客户端存储的文件列表，key:clientID,value:filename
	CFMMutex                               sync.RWMutex                               //ClientFileMap的读写锁
	FileShardsMap                          map[string][][]int32                       //文件的数据分片列表，key:clientID-filename-i,value:分片
	FileLeavesMap                          map[string][][]byte                        //文件的叶子节点列表，key:clientID-filename-i,value:叶子节点
	FileRootMap                            map[string][]byte                          //文件的默克尔树根节点哈希值列表，key:clientID-filename-i,value:根节点哈希值
	FileVersionMap                         map[string]int                             //文件的版本号，key:clientID-filename-i,value:版本号
	FSLRMMMutex                            sync.RWMutex                               //FileShardsMap，FileLeavesMap，FileRootMap的读写锁
	pb.UnimplementedStorjSNServiceServer                                              // 面向客户端的服务器嵌入匿名字段
	pb.UnimplementedStorjSNACServiceServer                                            // 面向审计方的服务器嵌入匿名字段
	PendingACPutFNotice                    map[string]int                             //用于暂存来自AC的文件存储通知，key:clientid-filename-i,value:1表示该文件在等待存储，2表示该文件完成存储
	PendingACPutFV                         map[string]int                             //用于暂存来自AC的文件存储版本号：key:clientID-filename-i,value:版本号
	PACNMutex                              sync.RWMutex                               //用于限制PendingACPutDSNotice访问的锁
	PendingACUpdFNotice                    map[string]int                             //用于暂存来自AC的文件更新通知，key:clientid-filename-i,value:1表示该文件在等待更新，2表示该文件完成更新
	PendingACUpdFV                         map[string]int                             //用于暂存来自AC的文件更新版本号，key:clientID-filename-i,value:版本号
	PACUFNMutex                            sync.RWMutex                               //用于限制PendingACUpdFNotice访问的锁
	AuditorFileQueue                       map[string]map[string]map[string][][]int32 //待审计的文件分片，key:审计号，subkey:currpcno,subsubkey:cid-fn-i,subsubvalue:文件分片
	AFQMutex                               sync.RWMutex                               //AuditorFileQueue的读写锁
}

// 新建存储分片
func NewStorjSN(snid string, snaddr string) *StorjSN {
	clientFileMap := make(map[string][]string)
	fileShardsMap := make(map[string][][]int32)
	fileleavesMap := make(map[string][][]byte)
	filerootMap := make(map[string][]byte)
	fileversionMap := make(map[string]int)
	pacpfn := make(map[string]int)
	pacpfv := make(map[string]int)
	pacufn := make(map[string]int)
	pacufv := make(map[string]int)
	afq := make(map[string]map[string]map[string][][]int32)
	sn := &StorjSN{snid, snaddr, clientFileMap, sync.RWMutex{}, fileShardsMap, fileleavesMap, filerootMap, fileversionMap, sync.RWMutex{}, pb.UnimplementedStorjSNServiceServer{}, pb.UnimplementedStorjSNACServiceServer{}, pacpfn, pacpfv, sync.RWMutex{}, pacufn, pacufv, sync.RWMutex{}, afq, sync.RWMutex{}} //设置监听地址
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
	sn.FileVersionMap[cid_fn] = int(preq.Version)
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
	sn.PendingACPutFV[cid_fn] = int(preq.Version)
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
	sn.PendingACPutFV[cid_fn] = int(preq.Version)
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
	delete(sn.PendingACPutFV, cid_fn)
	sn.PACNMutex.Unlock()
	//获取文件根节点哈希
	sn.FSLRMMMutex.RLock()
	root := sn.FileRootMap[cid_fn]
	sn.FSLRMMMutex.RUnlock()
	log.Println(sn.SNId, "已接收通知", cid_fn)
	return &pb.StorjClientStorageResponse{ClientId: clientId, Filename: filename, Repno: repno, Root: root, Snid: sn.SNId, Message: sn.SNId + " completes the storage of " + cid_fn + "."}, nil
}

// 【供客户端使用的RPC】
func (sn *StorjSN) StorjGetFile(ctx context.Context, req *pb.StorjGetFRequest) (*pb.StorjGetFResponse, error) {
	cid_fn := req.ClientId + "-" + req.Filename + "-" + req.Rep
	datashards := make([][]int32, 0)
	parityleaves := make([][]byte, 0)
	sn.FSLRMMMutex.RLock()
	if sn.FileShardsMap[cid_fn] == nil {
		sn.FSLRMMMutex.RUnlock()
		e := errors.New("datashards not exist")
		return &pb.StorjGetFResponse{Filename: req.Filename, Version: int32(0), DataShards: nil, MerkleLeaves: nil}, e
	} else if sn.FileLeavesMap[cid_fn] == nil {
		sn.FSLRMMMutex.RUnlock()
		e := errors.New("leaves not exist")
		return &pb.StorjGetFResponse{Filename: req.Filename, Version: int32(0), DataShards: nil, MerkleLeaves: nil}, e
	} else {
		//返回dsnum个数据分片和校验块的叶节点
		for i := 0; i < int(req.Dsnum); i++ {
			datashards = append(datashards, sn.FileShardsMap[cid_fn][i])
		}
		for i := int(req.Dsnum); i < len(sn.FileLeavesMap[cid_fn]); i++ {
			parityleaves = append(parityleaves, sn.FileLeavesMap[cid_fn][i])
		}
		version := sn.FileVersionMap[cid_fn]
		sn.FSLRMMMutex.RUnlock()
		return &pb.StorjGetFResponse{Filename: req.Filename, Version: int32(version), DataShards: util.Int32SliceToInt32ArraySNSlice(datashards), MerkleLeaves: parityleaves}, nil
	}
}

// 【供客户端使用的RPC】
func (sn *StorjSN) StorjUpdateFile(ctx context.Context, req *pb.StorjUpdFRequest) (*pb.StorjUpdFResponse, error) {
	clientId := req.ClientId
	filename := req.Filename
	repno := req.Rep
	message := ""
	cid_fn := clientId + "-" + filename + "-" + strconv.Itoa(int(repno))
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
	dss := make([][]int32, 0)
	for i := 0; i < len(req.DataShards); i++ {
		dss = append(dss, req.DataShards[i].Values)
	}
	//2-2-更新文件到各个列表中
	sn.FSLRMMMutex.Lock()
	sn.FileShardsMap[cid_fn] = dss
	sn.FileLeavesMap[cid_fn] = req.MerkleLeaves
	root := util.BuildMerkleTree(req.MerkleLeaves)
	sn.FileRootMap[cid_fn] = root
	sn.FileVersionMap[cid_fn] = 2
	sn.FSLRMMMutex.Unlock()
	message = "Update File Success!"
	//3-修改PendingACUpdFNotice
	sn.PACUFNMutex.Lock()
	sn.PendingACUpdFNotice[cid_fn] = 2
	sn.PACUFNMutex.Unlock()
	// 4-告知审计方文件更新结果
	return &pb.StorjUpdFResponse{Filename: req.Filename, Root: root, Message: message}, nil
}

// 【供审计方使用的RPC】存储节点接收审计方文件更新通知，阻塞等待客户端存放文件，完成后回复审计方
func (sn *StorjSN) StorjUpdateFileNotice(ctx context.Context, preq *pb.StorjClientUFRequest) (*pb.StorjClientUFResponse, error) {
	clientId := preq.ClientId
	filename := preq.Filename
	repno := preq.Rep
	cid_fn := clientId + "-" + filename + "-" + strconv.Itoa(int(repno))
	//写来自审计方的分片更新通知
	sn.PACUFNMutex.Lock()
	sn.PendingACUpdFNotice[cid_fn] = 1
	sn.PendingACUpdFV[cid_fn] = int(preq.Version)
	sn.PACUFNMutex.Unlock()
	//阻塞监测分片是否已完成更新
	iscomplete := 1
	for {
		sn.PACUFNMutex.RLock()
		iscomplete = sn.PendingACUpdFNotice[cid_fn]
		sn.PACUFNMutex.RUnlock()
		if iscomplete == 2 {
			break
		} else if iscomplete == 0 {
			log.Fatalln("nnnn")
		}
	}
	//文件完成存储，则删除pending元素，给审计方返回消息
	sn.PACUFNMutex.Lock()
	delete(sn.PendingACUpdFNotice, cid_fn)
	delete(sn.PendingACUpdFV, cid_fn)
	sn.PACUFNMutex.Unlock()
	//获取文件根节点哈希
	sn.FSLRMMMutex.RLock()
	root := sn.FileRootMap[cid_fn]
	sn.FSLRMMMutex.RUnlock()
	return &pb.StorjClientUFResponse{ClientId: clientId, Filename: filename, Repno: repno, Root: root, Snid: sn.SNId, Message: sn.SNId + " completes the update of " + cid_fn + "."}, nil
}

// 【供审计方使用的RPC】预审计请求处理
func (sn *StorjSN) StorjPreAuditSN(ctx context.Context, req *pb.StorjPASNRequest) (*pb.StorjPASNResponse, error) {
	if req.Snid != sn.SNId {
		e := errors.New("snid in preaudit request not consist with " + sn.SNId)
		return nil, e
	}
	readyFileMap := make(map[string][][]int32) //已经准备好的文件分片，key:clientId-filename-i;value:分片列表
	var rFMMutex sync.Mutex
	unreadyFileVMap := make(map[string]int32) //未准备好的文件，即审计方请求已过时，key:clientId-filename-i,value:版本号
	var urFVMMutex sync.Mutex
	//遍历审计表，判断是否满足审计方的快照要求
	done := make(chan struct{})
	for cid_fni, version := range req.Cidfniv {
		go func(cidfni string, version int) {
			//如果被挑战的版本已完成更新，则加入到readyFileMap中
			//如果当前完成更新的版本小于被挑战的版本，则等待更新完成后加入到readyFileMap中
			//如果当前完成更新的版本大于被挑战的版本，则加入到unreadyFileMap中
			sn.FSLRMMMutex.RLock()
			currentV := sn.FileVersionMap[cidfni]
			sn.FSLRMMMutex.RUnlock()
			if currentV == version {
				sn.FSLRMMMutex.RLock()
				rFMMutex.Lock()
				readyFileMap[cidfni] = sn.FileShardsMap[cidfni]
				rFMMutex.Unlock()
				sn.FSLRMMMutex.RUnlock()
			} else if currentV < version {
				for {
					sn.FSLRMMMutex.RLock()
					currentV = sn.FileVersionMap[cidfni]
					sn.FSLRMMMutex.RUnlock()
					if currentV == version {
						sn.FSLRMMMutex.RLock()
						rFMMutex.Lock()
						readyFileMap[cidfni] = sn.FileShardsMap[cidfni]
						rFMMutex.Unlock()
						sn.FSLRMMMutex.RUnlock()
						break
					}
				}
			} else {
				urFVMMutex.Lock()
				unreadyFileVMap[cidfni] = int32(currentV)
				urFVMMutex.Unlock()
			}
			// 通知主线程任务完成
			done <- struct{}{}
		}(cid_fni, int(version))
	}
	// 等待所有协程完成
	for i := 0; i < len(req.Cidfniv); i++ {
		<-done
	}
	sn.AFQMutex.Lock()
	if sn.AuditorFileQueue[req.Auditno] == nil {
		sn.AuditorFileQueue[req.Auditno] = make(map[string]map[string][][]int32)
	}
	sn.AuditorFileQueue[req.Auditno][strconv.Itoa(int(req.Currpcno))] = readyFileMap
	sn.AFQMutex.Unlock()
	return &pb.StorjPASNResponse{Isready: len(unreadyFileVMap) == 0, Fversion: unreadyFileVMap, Totalrpcs: req.Totalrpcs, Currpcno: req.Currpcno}, nil
}

// 【供审计方使用的RPC】获取存储节点上所有存储文件副本的聚合存储证明
func (sn *StorjSN) StorjGetPosSN(ctx context.Context, req *pb.StorjGAPSNRequest) (*pb.StorjGAPSNResponse, error) {
	sn.AFQMutex.RLock()
	fileshards := sn.AuditorFileQueue[req.Auditno][strconv.Itoa(int(req.Currpcno))]
	sn.AFQMutex.RUnlock()
	if fileshards == nil {
		e := errors.New("auditorno not exsit")
		return &pb.StorjGAPSNResponse{Preleafs: nil}, e
	}
	preleaf := make(map[string]*pb.BytesArray)
	for cidfni, dss := range fileshards {
		pfs := GetPreleafs(dss, req.Cidfnirands[cidfni].Values)
		preleaf[cidfni] = &pb.BytesArray{Values: pfs}
	}
	sn.AFQMutex.Lock()
	delete(sn.AuditorFileQueue[req.Auditno], strconv.Itoa(int(req.Currpcno)))
	if len(sn.AuditorFileQueue[req.Auditno]) == 0 {
		delete(sn.AuditorFileQueue, req.Auditno)
	}
	sn.AFQMutex.Unlock()
	return &pb.StorjGAPSNResponse{Preleafs: preleaf, Totalrpcs: req.Totalrpcs, Currpcno: req.Currpcno}, nil
}

// 获取文件分片对应的preleaf
func GetPreleafs(dss [][]int32, rands []int32) [][]byte {
	preleafs := make([][]byte, 0)
	for i := 0; i < len(dss); i++ {
		// 将s_i转换为字节数组
		s_iBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(s_iBytes, uint32(rands[i]))
		// 将data转换为字节数组
		dataBytes := make([]byte, 4*len(dss[i]))
		for i, v := range dss[i] {
			binary.LittleEndian.PutUint32(dataBytes[i*4:], uint32(v))
		}
		// 计算s_i + dataslice[i]
		combined := append(s_iBytes, dataBytes...)
		// 计算H(H(s_i + dataslice[i]))
		pf := util.Hash(combined)
		preleafs = append(preleafs, pf)
	}
	return preleafs
}
