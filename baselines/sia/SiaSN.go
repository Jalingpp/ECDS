package baselines

import (
	pb "ECDS/proto" // 根据实际路径修改
	"ECDS/util"
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"github.com/syndtr/goleveldb/leveldb"
	"google.golang.org/grpc"
)

type SiaSN struct {
	SNId   string //存储节点id
	SNAddr string //存储节点ip地址
	// ClientDSHashMap      map[string][][]byte //为客户端存储的文件列表，key:clientID,value:数据分片的哈希值列表
	// ClientDSHashIndexMap map[string]int      //分片在ClientDSHashMap某个列表中的索引号，key:clientID-filename-i,value:索引号
	// FileShardsMap                        map[string][]int32                   //文件的数据分片列表，key:clientID-filename-i,value:分片
	// FileVersionMap                       map[string]int                       //文件的版本号，key:clientID-filename-i,value:版本号
	// ClientMerkleRootMap                  map[string][]byte                    //客户端所有分片构建成的Merkel树根最新哈希值，key:clientID,value:hash value of root
	// ClientMerkleRootTimeMap              map[string]int                       //最新根节点哈希值的时间戳，key:clientID,value:时间戳
	CacheDataShards                      *lru.Cache                           //缓存大小在NewStorageNode中固定
	DBDataShards                         *leveldb.DB                          //存储路径在NewStorageNode和GetSNStorageCost中固定
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
	// clientdshMap := make(map[string][][]byte)
	// clientdshiMap := make(map[string]int)
	// clientmrMap := make(map[string][]byte)
	// clientmrtMap := make(map[string]int)
	// fileShardsMap := make(map[string][]int32)
	// fileversionMap := make(map[string]int)
	// 创建lru缓存
	cache, err := lru.New(50)
	if err != nil {
		log.Fatal(err)
	}
	// 打开或创建数据库
	path := "/home/ubuntu/ECDS/data/DB/Sia/datashards-" + snid
	// path := "/root/DSN/ECDS/data/DB/Sia/datashards-" + snid
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	pacpfn := make(map[string]int)
	pacpp := make(map[string][][]byte)
	pacpt := make(map[string][]byte)
	pacptr := make(map[string]int)
	pacufn := make(map[string]int)
	pacupp := make(map[string][][]byte)
	pacut := make(map[string][]byte)
	pacutr := make(map[string]int)
	afq := make(map[string]map[string]*SiaAuditInfor)
	sn := &SiaSN{snid, snaddr, cache, db, sync.RWMutex{}, pb.UnimplementedSiaSNServiceServer{}, pb.UnimplementedSiaSNACServiceServer{}, pacpfn, pacpp, pacpt, pacptr, sync.RWMutex{}, pacufn, pacupp, pacut, pacutr, sync.RWMutex{}, afq, sync.RWMutex{}} //设置监听地址
	// sn := &SiaSN{snid, snaddr, clientdshMap, clientdshiMap, fileShardsMap, fileversionMap, clientmrMap, clientmrtMap, sync.RWMutex{}, pb.UnimplementedSiaSNServiceServer{}, pb.UnimplementedSiaSNACServiceServer{}, pacpfn, pacpp, pacpt, pacptr, sync.RWMutex{}, pacufn, pacupp, pacut, pacutr, sync.RWMutex{}, afq, sync.RWMutex{}} //设置监听地址
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
	fskey := cid_fn + "FS"
	fileshard, _ := sn.GetSiaFileShardFromCacheOrDB(fskey)
	// if sn.FileShardsMap[cid_fn] != nil {
	if fileshard != nil {
		sn.CFMMutex.Unlock()
		message = "Filename Already Exist!"
		e := errors.New("filename already exist")
		return &pb.SiaPutFResponse{Filename: preq.Filename, Dsno: preq.Dsno, Message: message}, e
	}
	// sn.FileShardsMap[cid_fn] = preq.DataShard
	// sn.FileVersionMap[cid_fn] = int(preq.Version)
	fileshardToSave := preq.DataShard
	fileversionToSave := int(preq.Version)
	//2-3-放置客户端文件名列表
	cdshkey := clientId + "CDSH"
	clientDSHash, _ := sn.GetSiaClientDSHashFromCacheOrDB(cdshkey)
	// if sn.ClientDSHashMap[clientId] == nil {
	if clientDSHash == nil {
		// sn.ClientDSHashMap[clientId] = make([][]byte, 0)
		clientDSHash = make([][]byte, 0)
	}
	//对分片取哈希
	dshash := util.Hash([]byte(util.Int32SliceToStr(preq.DataShard)))
	// sn.ClientDSHashMap[clientId] = append(sn.ClientDSHashMap[clientId], dshash)
	clientDSHash = append(clientDSHash, dshash)
	// sn.ClientDSHashIndexMap[cid_fn] = len(sn.ClientDSHashMap[clientId]) - 1
	clientDSHashIndex := len(clientDSHash) - 1
	//计算Merkel根节点哈希，并获取dsno分片对应的验证路径
	// leafHashes := sn.ClientDSHashMap[preq.ClientId]
	leafHashes := clientDSHash
	index := clientDSHashIndex
	// sn.CFMMutex.Unlock()
	root, paths := util.BuildMerkleTreeAndGeneratePath(leafHashes, index)
	// sn.CFMMutex.Lock()
	cmrtkey := clientId + "CMRT"
	// oldtime := sn.ClientMerkleRootTimeMap[preq.ClientId]
	oldtime, _ := sn.GetSiaClientMerkleRootTimeFromCacheOrDB(cmrtkey)
	// sn.ClientMerkleRootMap[preq.ClientId] = root
	clientMerkleRootToSave := root
	// sn.ClientMerkleRootTimeMap[preq.ClientId] = oldtime + 1
	clientMerkleRootTimeToSave := oldtime + 1
	sn.SaveSiaDataShardToDB(cid_fn, clientId, clientDSHash, clientDSHashIndex, fileshardToSave, fileversionToSave, clientMerkleRootToSave, clientMerkleRootTimeToSave)
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

func (sn *SiaSN) SaveSiaDataShardToDB(cid_fn string, clientId string, clientDSHash [][]byte, clientDSHashIndex int, fileShard []int32, fileVersion int, clientMerkleRoot []byte, clientMerkleRootTime int) error {
	// 序列化 clientDSHash
	cdshbytes := serializedCDSH(clientDSHash)
	// 将clientDSHash写入LevelDB
	cdshkey := clientId + "CDSH"
	err := sn.DBDataShards.Put([]byte(cdshkey), cdshbytes, nil)
	if err != nil {
		return err
	}
	// 更新缓存
	sn.CacheDataShards.Add(string(cdshkey), clientDSHash)

	// 序列化 clientDSHashIndex
	cdshibytes := serializedCDSHI(clientDSHashIndex)
	// 将leaves写入DB
	cdshikey := cid_fn + "CDSHI"
	err = sn.DBDataShards.Put([]byte(cdshikey), cdshibytes, nil)
	if err != nil {
		return err
	}
	// 更新缓存
	sn.CacheDataShards.Add(string(cdshikey), clientDSHashIndex)

	// 序列化 fileshard
	fsbytes := serializedFS(fileShard)
	// 将fileshard写入DB
	fskey := cid_fn + "FS"
	err = sn.DBDataShards.Put([]byte(fskey), fsbytes, nil)
	if err != nil {
		return err
	}
	// 更新缓存
	sn.CacheDataShards.Add(string(fskey), fileShard)

	// 序列化版本号
	fvbytes := serializedFV(fileVersion)
	// 将version写入DB
	fvkey := cid_fn + "FV"
	err = sn.DBDataShards.Put([]byte(fvkey), fvbytes, nil)
	if err != nil {
		return err
	}
	// 更新缓存
	sn.CacheDataShards.Add(string(fvkey), fileVersion)

	// 将clientMerkleRoot写入LevelDB
	cmrkey := clientId + "CMR"
	err = sn.DBDataShards.Put([]byte(cmrkey), clientMerkleRoot, nil)
	if err != nil {
		return err
	}
	// 更新缓存
	sn.CacheDataShards.Add(string(cmrkey), clientMerkleRoot)

	// 序列化 clientMerkleRootTime
	cmrtbytes := serializedCMRT(clientMerkleRootTime)
	// 将clientMerkleRootTime写入DB
	cmrtkey := clientId + "CMRT"
	err = sn.DBDataShards.Put([]byte(cmrtkey), cmrtbytes, nil)
	if err != nil {
		return err
	}
	// 更新缓存
	sn.CacheDataShards.Add(string(cmrtkey), clientMerkleRootTime)

	return nil
}

func (sn *SiaSN) GetSiaFileVersionFromCacheOrDB(clientIDFV string) (int, error) {
	// 尝试从缓存中获取
	if val, ok := sn.CacheDataShards.Get(clientIDFV); ok {
		return val.(int), nil
	}

	// 缓存未命中，从 LevelDB 获取
	fvbytes, err := sn.DBDataShards.Get([]byte(clientIDFV), nil)
	if err != nil {
		return -1, err
	}

	// 反序列化 clientDSHash
	fileversion := deserializeCMRT(fvbytes)
	if err != nil {
		return -1, err
	}

	// 更新缓存
	sn.CacheDataShards.Add(clientIDFV, fileversion)

	return fileversion, nil
}

// 将fileversion序列化为[]byte
func serializedFV(fileversion int) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(fileversion)
	if err != nil {
		fmt.Println("序列化失败:", err)
		return nil
	}
	serializedData := buf.Bytes()
	return serializedData
}

// 将序列化后的fileversion反序列化
func deserializeFV(fvbytes []byte) int {
	var deserializedData int
	dec := gob.NewDecoder(bytes.NewReader(fvbytes))
	err := dec.Decode(&deserializedData)
	if err != nil {
		fmt.Println("反序列化失败:", err)
		return -1
	}
	return deserializedData
}

func (sn *SiaSN) GetSiaClientDSHashIndexFromCacheOrDB(clientIDCDSHI string) (int, error) {
	// 尝试从缓存中获取
	if val, ok := sn.CacheDataShards.Get(clientIDCDSHI); ok {
		return val.(int), nil
	}

	// 缓存未命中，从 LevelDB 获取
	cdshibytes, err := sn.DBDataShards.Get([]byte(clientIDCDSHI), nil)
	if err != nil {
		return -1, err
	}

	// 反序列化 clientDSHash
	clientDSHashIndex := deserializeCDSHI(cdshibytes)
	if err != nil {
		return -1, err
	}

	// 更新缓存
	sn.CacheDataShards.Add(clientIDCDSHI, clientDSHashIndex)

	return clientDSHashIndex, nil
}

// 将clientDSHashIndex序列化为[]byte
func serializedCDSHI(clientDSHashIndex int) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(clientDSHashIndex)
	if err != nil {
		fmt.Println("序列化失败:", err)
		return nil
	}
	serializedData := buf.Bytes()
	return serializedData
}

// 将序列化后的clientDSHashIndex反序列化
func deserializeCDSHI(cdshibytes []byte) int {
	var deserializedData int
	dec := gob.NewDecoder(bytes.NewReader(cdshibytes))
	err := dec.Decode(&deserializedData)
	if err != nil {
		fmt.Println("反序列化失败:", err)
		return -1
	}
	return deserializedData
}

func (sn *SiaSN) GetSiaClientMerkleRootTimeFromCacheOrDB(clientIDCMRT string) (int, error) {
	// 尝试从缓存中获取
	if val, ok := sn.CacheDataShards.Get(clientIDCMRT); ok {
		return val.(int), nil
	}

	// 缓存未命中，从 LevelDB 获取
	cmrtbytes, err := sn.DBDataShards.Get([]byte(clientIDCMRT), nil)
	if err != nil {
		return -1, err
	}

	// 反序列化 clientDSHash
	clientMerkleRootTime := deserializeCMRT(cmrtbytes)
	if err != nil {
		return -1, err
	}

	// 更新缓存
	sn.CacheDataShards.Add(clientIDCMRT, clientMerkleRootTime)

	return clientMerkleRootTime, nil
}

// 将clientMerkleRootTime序列化为[]byte
func serializedCMRT(clientMerkleRootTime int) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(clientMerkleRootTime)
	if err != nil {
		fmt.Println("序列化失败:", err)
		return nil
	}
	serializedData := buf.Bytes()
	return serializedData
}

// 将序列化后的clientMerkleRootTime反序列化
func deserializeCMRT(cmrtbytes []byte) int {
	var deserializedData int
	dec := gob.NewDecoder(bytes.NewReader(cmrtbytes))
	err := dec.Decode(&deserializedData)
	if err != nil {
		fmt.Println("反序列化失败:", err)
		return -1
	}
	return deserializedData
}

func (sn *SiaSN) GetSiaClientDSHashFromCacheOrDB(clientIDCDSH string) ([][]byte, error) {
	// 尝试从缓存中获取
	if val, ok := sn.CacheDataShards.Get(clientIDCDSH); ok {
		return val.([][]byte), nil
	}

	// 缓存未命中，从 LevelDB 获取
	cdshbytes, err := sn.DBDataShards.Get([]byte(clientIDCDSH), nil)
	if err != nil {
		return nil, err
	}

	// 反序列化 clientDSHash
	clientDSHash := deserializeCDSH(cdshbytes)
	if err != nil {
		return nil, err
	}

	// 更新缓存
	sn.CacheDataShards.Add(clientIDCDSH, clientDSHash)

	return clientDSHash, nil
}

// 将clientDSHash序列化为[]byte
func serializedCDSH(clientDSHash [][]byte) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(clientDSHash)
	if err != nil {
		fmt.Println("序列化失败:", err)
		return nil
	}
	serializedData := buf.Bytes()
	return serializedData
}

// 将序列化后的clientDSHash反序列化
func deserializeCDSH(cdshbytes []byte) [][]byte {
	var deserializedData [][]byte
	dec := gob.NewDecoder(bytes.NewReader(cdshbytes))
	err := dec.Decode(&deserializedData)
	if err != nil {
		fmt.Println("反序列化失败:", err)
		return nil
	}
	return deserializedData
}

func (sn *SiaSN) GetSiaFileShardFromCacheOrDB(clientIDFS string) ([]int32, error) {
	// 尝试从缓存中获取
	if val, ok := sn.CacheDataShards.Get(clientIDFS); ok {
		return val.([]int32), nil
	}

	// 缓存未命中，从 LevelDB 获取
	fsbytes, err := sn.DBDataShards.Get([]byte(clientIDFS), nil)
	if err != nil {
		return nil, err
	}

	// 反序列化 fileshard
	fileshard := deserializeFS(fsbytes)
	if err != nil {
		return nil, err
	}

	// 更新缓存
	sn.CacheDataShards.Add(clientIDFS, fileshard)

	return fileshard, nil
}

// 将FileShard序列化为[]byte
func serializedFS(fileShard []int32) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(fileShard)
	if err != nil {
		fmt.Println("序列化失败:", err)
		return nil
	}
	serializedData := buf.Bytes()
	return serializedData
}

// 将序列化后的FileShard反序列化
func deserializeFS(fsbytes []byte) []int32 {
	var deserializedData []int32
	dec := gob.NewDecoder(bytes.NewReader(fsbytes))
	err := dec.Decode(&deserializedData)
	if err != nil {
		fmt.Println("反序列化失败:", err)
		return nil
	}
	return deserializedData
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
	// index := sn.ClientDSHashIndexMap[cid_fn]
	cdshikey := cid_fn + "CDSHI"
	index, _ := sn.GetSiaClientDSHashIndexFromCacheOrDB(cdshikey)
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
	fskey := cid_fni + "FS"
	fileshard, _ := sn.GetSiaFileShardFromCacheOrDB(fskey)
	// if sn.FileShardsMap[cid_fni] == nil {
	if fileshard == nil {
		sn.CFMMutex.RUnlock()
		e := errors.New("datashard not exist")
		return nil, e
	} else {
		// seds := sn.FileShardsMap[cid_fni]
		seds := fileshard
		// version := sn.FileVersionMap[cid_fni]
		fvkey := cid_fni + "FV"
		version, _ := sn.GetSiaFileVersionFromCacheOrDB(fvkey)
		//构建数据分片的存储证明
		// leafHashes := sn.ClientDSHashMap[req.ClientId]
		cdshkey := req.ClientId + "CDSH"
		leafHashes, _ := sn.GetSiaClientDSHashFromCacheOrDB(cdshkey)
		// index := sn.ClientDSHashIndexMap[cid_fni]
		cdshikey := cid_fni + "CDSHI"
		index, _ := sn.GetSiaClientDSHashIndexFromCacheOrDB(cdshikey)
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
	fskey := cid_fn + "FS"
	fileshard, _ := sn.GetSiaFileShardFromCacheOrDB(fskey)
	// if sn.FileShardsMap[cid_fn] == nil {
	if fileshard == nil {
		sn.CFMMutex.Unlock()
		message = "Filename Not Exist!"
		e := errors.New("filename not exist")
		return nil, e
	}
	// sn.FileShardsMap[cid_fn] = ds
	fileshardToSave := ds
	// sn.FileVersionMap[cid_fn] = int(req.Newversion)
	fileversionToSave := int(req.Newversion)
	//2-3-放置客户端文件名列表
	cdshkey := clientId + "CDSH"
	clientDSHash, _ := sn.GetSiaClientDSHashFromCacheOrDB(cdshkey)
	// if sn.ClientDSHashMap[clientId] == nil {
	if clientDSHash == nil {
		// sn.ClientDSHashMap[clientId] = make([][]byte, 0)
		clientDSHash = make([][]byte, 0)
	}
	//对分片取哈希后更新至列表
	dshash := util.Hash([]byte(util.Int32SliceToStr(ds)))
	// index := sn.ClientDSHashIndexMap[cid_fn]
	cdshikey := cid_fn + "CDSHI"
	clientDSHashIndex, _ := sn.GetSiaClientDSHashIndexFromCacheOrDB(cdshikey)
	// sn.ClientDSHashMap[clientId][index] = dshash
	clientDSHash[clientDSHashIndex] = dshash
	// leafHashes := sn.ClientDSHashMap[clientId]
	leafHashes := clientDSHash
	// sn.CFMMutex.Unlock()
	root, paths := util.BuildMerkleTreeAndGeneratePath(leafHashes, clientDSHashIndex)
	// sn.CFMMutex.Lock()
	// oldtime := sn.ClientMerkleRootTimeMap[clientId]
	cmrtkey := clientId + "CMRT"
	oldtime, _ := sn.GetSiaClientMerkleRootTimeFromCacheOrDB(cmrtkey)
	// sn.ClientMerkleRootMap[clientId] = root
	clientMerkleRootToSave := root
	// sn.ClientMerkleRootTimeMap[clientId] = oldtime + 1
	clientMerkleRootTimeToSave := oldtime + 1
	sn.SaveSiaDataShardToDB(cid_fn, clientId, clientDSHash, clientDSHashIndex, fileshardToSave, fileversionToSave, clientMerkleRootToSave, clientMerkleRootTimeToSave)
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
	cdshikey := cid_fn + "CDSHI"
	// index := sn.ClientDSHashIndexMap[cid_fn]
	index, _ := sn.GetSiaClientDSHashIndexFromCacheOrDB(cdshikey)
	fvkey := cid_fn + "FV"
	// version := sn.FileVersionMap[cid_fn]
	version, _ := sn.GetSiaFileVersionFromCacheOrDB(fvkey)
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
	fskey := cid_fni + "FS"
	fileshard, _ := sn.GetSiaFileShardFromCacheOrDB(fskey)
	// if sn.FileShardsMap[cid_fni] == nil {
	if fileshard == nil {
		sn.CFMMutex.RUnlock()
		return nil, nil
	}
	fvkey := cid_fni + "FV"
	// v := sn.FileVersionMap[cid_fni]
	v, _ := sn.GetSiaFileVersionFromCacheOrDB(fvkey)
	if v < int(version) {
		sn.CFMMutex.RUnlock()
		return nil, nil
	}
	cdshikey := cid_fni + "CDSHI"
	// index := sn.ClientDSHashIndexMap[cid_fni]
	index, _ := sn.GetSiaClientDSHashIndexFromCacheOrDB(cdshikey)
	cmrtkey := cid + "CMRT"
	// rt := sn.ClientMerkleRootTimeMap[cid]
	rt, _ := sn.GetSiaClientMerkleRootTimeFromCacheOrDB(cmrtkey)
	// ds := make([]int32, len(sn.FileShardsMap[cid_fni]))
	ds := make([]int32, len(fileshard))
	// copy(ds, sn.FileShardsMap[cid_fni])
	copy(ds, fileshard)
	cdshkey := cid + "CDSH"
	// hashes := sn.ClientDSHashMap[cid]
	hashes, _ := sn.GetSiaClientDSHashFromCacheOrDB(cdshkey)
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

// 【供客户端使用的RPC】获取当前节点上对clientID相关文件的存储空间代价
func (sn *SiaSN) SiaGetSNStorageCost(ctx context.Context, req *pb.SiaGSNSCRequest) (*pb.SiaGSNSCResponse, error) {
	cid := req.ClientId
	// totalSize := 0
	// //统计所占存储空间大小
	// sn.CFMMutex.RLock()
	// clientFSMap := sn.FileShardsMap
	// sn.CFMMutex.RUnlock()
	// for key, ds := range clientFSMap {
	// 	if strings.HasPrefix(key, cid) {
	// 		totalSize = totalSize + len([]byte(util.Int32SliceToStr(ds)))
	// 	}
	// }
	path := "/home/ubuntu/ECDS/data/DB/Sia/datashards-" + sn.SNId
	// path := "/root/DSN/ECDS/data/DB/Sia/datashards-" + sn.SNId
	totalSize, _ := util.GetDatabaseSize(path)
	return &pb.SiaGSNSCResponse{ClientId: cid, SnId: sn.SNId, Storagecost: int32(totalSize)}, nil
}
