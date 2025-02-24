package baselines

import (
	pb "ECDS/proto" // 根据实际路径修改"
	"ECDS/util"
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	prooftypes "github.com/filecoin-project/go-state-types/proof"
	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"
	"github.com/syndtr/goleveldb/leveldb"
	"google.golang.org/grpc"
)

type SectorSealedInfor struct {
	SectorNum   abi.SectorNumber
	SealedCID   cid.Cid
	UnsealedCID cid.Cid
	Proof       []byte
}

func (ssi *SectorSealedInfor) Sizeof() int {
	return 8 + ssi.SealedCID.ByteLen() + ssi.UnsealedCID.ByteLen() + len(ssi.Proof)
}

type FilecoinSN struct {
	SNId   string //存储节点id
	SNAddr string //存储节点ip地址
	// ClientFileRepMap                          map[string]string                                           //客户端文件副本内容，key:clientID-fliename-i,value:content
	// ClientFileRepVMap                         map[string]int                                              //客户端文件副本版本，key:clientID-filename-i,value:version number
	// ClientFileRepSectorInforMap               map[string]map[int]*SectorSealedInfor                       //为客户端存储的文件列表，key:clientID-filename-i,subkey:sectorNum,value:SectorSealedInfor
	CacheDataShards                           *lru.Cache                                                  //缓存大小在NewStorageNode中固定
	DBDataShards                              *leveldb.DB                                                 //存储路径在NewStorageNode和GetSNStorageCost中固定
	CFRMMutex                                 sync.RWMutex                                                //ClientFileMap的读写锁
	pb.UnimplementedFilecoinSNServiceServer                                                               // 面向客户端的服务器嵌入匿名字段
	pb.UnimplementedFilecoinSNACServiceServer                                                             // 面向审计方的服务器嵌入匿名字段
	PendingACPutFNotice                       map[string]int                                              //用于暂存来自AC的文件存储通知，key:clientid-filename-i,value:1表示该文件在等待存储，2表示该文件完成存储
	PACNMutex                                 sync.RWMutex                                                //用于限制PendingACPutDSNotice访问的锁
	PendingACUpdateFNotice                    map[string]int                                              //用于暂存来自AC的文件更新通知，key:clientid-filename-i,value:1表示该文件在等待存储，2表示该文件完成存储
	PendingACUpdateFVMap                      map[string]int                                              //用于暂存来自AC的文件新版本号，key:clientID-filename-i,value:newversion
	PACUFNMutex                               sync.RWMutex                                                //用于限制PendingACUpdateDSNotice访问的锁
	MinerID                                   abi.ActorID                                                 //Filecoin存储证明矿工ID
	SealProofType                             abi.RegisteredSealProof                                     //Filecoin sealproof类型
	CidFnRepSectorCacheDirPathMap             map[string]string                                           //Filecoin存储证明缓存路径
	CidFnRepStagedSectorFileMap               map[string]string                                           //Filecoin存储证明阶段性扇区文件
	CidFnRepSealedSectorFileMap               map[string]string                                           //Filecoin存储证明密封扇区文件
	CFRSCMutex                                sync.RWMutex                                                //上述三个Map的读写锁
	Ticket                                    abi.SealRandomness                                          //Filecoin存储证明票根
	Seed                                      abi.InteractiveSealRandomness                               //Filecoin存储证明种子
	SectorNumber                              int                                                         //用于标记当前sector的编号
	SNMutex                                   sync.RWMutex                                                //SectorNumber的读写锁
	AuditorFileQueue                          map[string]map[string]map[string]map[int]*SectorSealedInfor //待审计的文件分片，key:审计号，subkey:currpcno,subsubkey:cid-fn-i,subsubvalue:文件扇区信息表int:扇区号
	AuditorSectorCacheDirPathMap              map[string]map[string]map[string]string                     //待审计的文件扇区缓存目录
	AuditorSealedSectorFileMap                map[string]map[string]map[string]string                     //待审计的文件扇区封装目录
	AFQMutex                                  sync.RWMutex                                                //AuditorFileQueue的读写锁
}

// 新建存储分片
func NewFilecoinSN(snid string, snaddr string) *FilecoinSN {
	// clientFileRepMap := make(map[string]string)
	// clientFileRepVMap := make(map[string]int)
	// clientFileRepSectorInforMap := make(map[string]map[int]*SectorSealedInfor)
	// 创建lru缓存
	cache, err := lru.New(50)
	if err != nil {
		log.Fatal(err)
	}
	// 打开或创建数据库
	// path := "/home/ubuntu/ECDS/data/DB/Filecoin/datashards-" + snid
	path := "/root/ECDS/data/DB/Filecoin/datashards-" + snid
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	pacpfn := make(map[string]int)
	pacufn := make(map[string]int)
	pacufvmap := make(map[string]int)
	snidsplit := strings.Split(snid, "n")
	snidint, _ := strconv.Atoi(snidsplit[1])
	sealProofType := abi.RegisteredSealProof_StackedDrg2KiBV1
	CidFnRepsectorCacheDirPath := make(map[string]string)
	CidFnRepstagedSectorFile := make(map[string]string)
	CidFnRepsealedSectorFile := make(map[string]string)
	ticket := abi.SealRandomness{5, 4, 2}
	seed := abi.InteractiveSealRandomness{7, 4, 2}
	afq := make(map[string]map[string]map[string]map[int]*SectorSealedInfor)
	ascdmap := make(map[string]map[string]map[string]string)
	assfmap := make(map[string]map[string]map[string]string)
	sn := &FilecoinSN{snid, snaddr, cache, db, sync.RWMutex{}, pb.UnimplementedFilecoinSNServiceServer{}, pb.UnimplementedFilecoinSNACServiceServer{}, pacpfn, sync.RWMutex{}, pacufn, pacufvmap, sync.RWMutex{}, abi.ActorID(snidint), sealProofType, CidFnRepsectorCacheDirPath, CidFnRepstagedSectorFile, CidFnRepsealedSectorFile, sync.RWMutex{}, ticket, seed, 0, sync.RWMutex{}, afq, ascdmap, assfmap, sync.RWMutex{}} //设置监听地址
	// sn := &FilecoinSN{snid, snaddr, clientFileRepMap, clientFileRepVMap, clientFileRepSectorInforMap, sync.RWMutex{}, pb.UnimplementedFilecoinSNServiceServer{}, pb.UnimplementedFilecoinSNACServiceServer{}, pacpfn, sync.RWMutex{}, pacufn, pacufvmap, sync.RWMutex{}, abi.ActorID(snidint), sealProofType, CidFnRepsectorCacheDirPath, CidFnRepstagedSectorFile, CidFnRepsealedSectorFile, sync.RWMutex{}, ticket, seed, 0, sync.RWMutex{}, afq, ascdmap, assfmap, sync.RWMutex{}} //设置监听地址
	lis, err := net.Listen("tcp", snaddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterFilecoinSNServiceServer(s, sn)
	pb.RegisterFilecoinSNACServiceServer(s, sn)
	log.Println("Server listening on " + snaddr)
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	return sn
}

// 【供客户端使用的RPC】存储节点验证分片序号是否与审计方通知的一致，验签，存放数据分片，告知审计方已存储或存储失败，给客户端回复消息
func (sn *FilecoinSN) FilecoinPutFile(ctx context.Context, preq *pb.FilecoinPutFRequest) (*pb.FilecoinPutFResponse, error) {
	clientId := preq.ClientId
	filename := preq.Filename
	repno := preq.Repno
	message := ""
	cid_fn := clientId + "-" + filename + "-" + strconv.Itoa(int(repno))
	//1-阻塞等待收到审计方通知
	for {
		sn.PACNMutex.RLock()
		if sn.PendingACPutFNotice[cid_fn] == 1 {
			sn.PACNMutex.RUnlock()
			break
		}
		sn.PACNMutex.RUnlock()
	}
	//2-存放数据文件
	//2-3-放置客户端文件名列表
	cfrsis := sn.FilecoinConstructSectors(cid_fn, int(preq.Version), preq.Content)
	sn.CFRMMutex.Lock()
	//构建扇区并生成sealedID
	// sn.ClientFileRepSectorInforMap[cid_fn] = sn.FilecoinConstructSectors(cid_fn, int(preq.Version), preq.Content)
	// sn.ClientFileRepSectorInforMap[cid_fn] = cfrsis
	// sn.ClientFileRepMap[cid_fn] = preq.Content
	// sn.ClientFileRepVMap[cid_fn] = int(preq.Version)
	sn.SaveFilecoinDataShardToDB(cid_fn, cfrsis, preq.Content, int(preq.Version))
	sn.CFRMMutex.Unlock()
	message = "OK"
	log.Println("【PutFile】", cid_fn, message)
	//3-修改PendingACPutDSNotice
	sn.PACNMutex.Lock()
	sn.PendingACPutFNotice[cid_fn] = 2
	sn.PACNMutex.Unlock()
	// 4-告知审计方分片放置结果
	return &pb.FilecoinPutFResponse{Filename: preq.Filename, Repno: preq.Repno, Message: message}, nil
}

func (sn *FilecoinSN) SaveFilecoinDataShardToDB(cid_fn string, clientFileRepSectorInfor map[int]*SectorSealedInfor, clientFileRep string, clientFileRepV int) error {
	// 序列化 clientFileRepSectorInfor
	ssibytes := serializedSSI(clientFileRepSectorInfor)
	// 将clientFileRepSectorInfor写入LevelDB
	ssikey := cid_fn + "SSI"
	err := sn.DBDataShards.Put([]byte(ssikey), ssibytes, nil)
	if err != nil {
		return err
	}
	// 更新缓存
	sn.CacheDataShards.Add(string(ssikey), clientFileRepSectorInfor)

	// 序列化 clientFileRep
	cfrbytes := []byte(clientFileRep)
	// 将clientFileRep写入DB
	cfrkey := cid_fn + "CFR"
	err = sn.DBDataShards.Put([]byte(cfrkey), cfrbytes, nil)
	if err != nil {
		return err
	}
	// 更新缓存
	sn.CacheDataShards.Add(string(cfrkey), clientFileRep)

	// 序列化 clientFileRepV
	cfrvbytes := serializedCFRV(clientFileRepV)
	// 将clientMerkleRootTime写入DB
	cfrvkey := cid_fn + "CFRV"
	err = sn.DBDataShards.Put([]byte(cfrvkey), cfrvbytes, nil)
	if err != nil {
		return err
	}
	// 更新缓存
	sn.CacheDataShards.Add(string(cfrvkey), clientFileRepV)

	return nil
}

// 将clientFileRepV序列化为[]byte
func serializedCFRV(clientFileRepV int) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(clientFileRepV)
	if err != nil {
		fmt.Println("序列化失败:", err)
		return nil
	}
	serializedData := buf.Bytes()
	return serializedData
}

// 将序列化后的clientFileRepV反序列化
func deserializeCFRV(cfrvbytes []byte) int {
	var deserializedData int
	dec := gob.NewDecoder(bytes.NewReader(cfrvbytes))
	err := dec.Decode(&deserializedData)
	if err != nil {
		fmt.Println("反序列化失败:", err)
		return -1
	}
	return deserializedData
}

type SerializableSectorSealedInfor struct {
	SectorNum   abi.SectorNumber
	SealedCID   string
	UnsealedCID string
	Proof       []byte
}

// 将clientFileRepSectorInfor序列化为[]byte
func serializedSSI(clientFileRepSectorInfor map[int]*SectorSealedInfor) []byte {
	// 转换为可序列化的结构体
	serializableMap := make(map[int]*SerializableSectorSealedInfor)
	for key, value := range clientFileRepSectorInfor {
		serializableMap[key] = &SerializableSectorSealedInfor{
			SectorNum:   value.SectorNum,
			SealedCID:   value.SealedCID.String(),
			UnsealedCID: value.UnsealedCID.String(),
			Proof:       value.Proof,
		}
	}

	// 序列化
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(serializableMap)
	if err != nil {
		fmt.Println("序列化失败:", err)
		return nil
	}
	return buf.Bytes()
}

// 反序列化clientFileRepSectorInfor
func deserializedSSI(data []byte) (map[int]*SectorSealedInfor, error) {
	// 注册类型
	gob.Register(&SerializableSectorSealedInfor{})

	// 创建一个缓冲区
	buf := bytes.NewBuffer(data)

	// 创建解码器
	dec := gob.NewDecoder(buf)

	// 解码为 map[int]*SerializableSectorSealedInfor
	serializableMap := make(map[int]*SerializableSectorSealedInfor)
	err := dec.Decode(&serializableMap)
	if err != nil {
		return nil, fmt.Errorf("反序列化失败: %v", err)
	}

	// 转换为 map[int]*SectorSealedInfor
	resultMap := make(map[int]*SectorSealedInfor)
	for key, value := range serializableMap {
		// 将字符串转换为 cid.Cid
		sealedCID, err := cid.Decode(value.SealedCID)
		if err != nil {
			return nil, fmt.Errorf("解析 SealedCID 失败: %v", err)
		}
		unsealedCID, err := cid.Decode(value.UnsealedCID)
		if err != nil {
			return nil, fmt.Errorf("解析 UnsealedCID 失败: %v", err)
		}

		// 构造 SectorSealedInfor
		resultMap[key] = &SectorSealedInfor{
			SectorNum:   value.SectorNum,
			SealedCID:   sealedCID,
			UnsealedCID: unsealedCID,
			Proof:       value.Proof,
		}
	}

	return resultMap, nil
}

func (sn *FilecoinSN) GetFilecoinSSIFromCacheOrDB(ssikey string) (map[int]*SectorSealedInfor, error) {
	// 尝试从缓存中获取
	if val, ok := sn.CacheDataShards.Get(ssikey); ok {
		return val.(map[int]*SectorSealedInfor), nil
	}

	// 缓存未命中，从 LevelDB 获取
	ssibytes, err := sn.DBDataShards.Get([]byte(ssikey), nil)
	if err != nil {
		return nil, err
	}

	// 反序列化为 map[int]*SectorSealedInfor
	clientFileRepSectorInfor, _ := deserializedSSI(ssibytes)
	if err != nil {
		return nil, err
	}

	// 更新缓存
	sn.CacheDataShards.Add(ssikey, clientFileRepSectorInfor)

	return clientFileRepSectorInfor, nil
}

func (sn *FilecoinSN) GetFilecoinCFRFromCacheOrDB(cfrkey string) (string, error) {
	// 尝试从缓存中获取
	if val, ok := sn.CacheDataShards.Get(cfrkey); ok {
		return val.(string), nil
	}

	// 缓存未命中，从 LevelDB 获取
	cfrbytes, err := sn.DBDataShards.Get([]byte(cfrkey), nil)
	if err != nil {
		return "", err
	}

	// 反序列化 clientFileRep
	clientFileRep := string(cfrbytes)
	if err != nil {
		return "", err
	}

	// 更新缓存
	sn.CacheDataShards.Add(cfrkey, clientFileRep)

	return clientFileRep, nil
}

func (sn *FilecoinSN) GetFilecoinCFRVFromCacheOrDB(cfrvkey string) (int, error) {
	// 尝试从缓存中获取
	if val, ok := sn.CacheDataShards.Get(cfrvkey); ok {
		return val.(int), nil
	}

	// 缓存未命中，从 LevelDB 获取
	cfrvbytes, err := sn.DBDataShards.Get([]byte(cfrvkey), nil)
	if err != nil {
		return -1, err
	}

	// 反序列化 clientFileRep
	clientFileRepV := deserializeCFRV(cfrvbytes)
	if err != nil {
		return -1, err
	}

	// 更新缓存
	sn.CacheDataShards.Add(cfrvkey, clientFileRepV)

	return clientFileRepV, nil
}

// 为文件内容构建扇区
func (sn *FilecoinSN) FilecoinConstructSectors(cid_fn string, version int, filecontent string) map[int]*SectorSealedInfor {
	sectors := make(map[int]*SectorSealedInfor)
	sn.CFRSCMutex.Lock()
	sectorCacheDirPath := requireTempDirPath("sector-cache-dir" + cid_fn + "-" + strconv.Itoa(version) + "-")
	stagedSectorFile := requireTempFile(bytes.NewReader([]byte{}), 0)
	fmt.Println("stagedSectorFileName:", stagedSectorFile.Name())
	defer stagedSectorFile.Close()
	sealedSectorFile := requireTempFile(bytes.NewReader([]byte{}), 0)
	fmt.Println("sealedSectorFileName:", sealedSectorFile.Name())
	defer sealedSectorFile.Close()
	sn.CFRSCMutex.Unlock()
	databyte, roudnum := PadTo1524Multiple([]byte(filecontent))
	for i := 0; i < roudnum; i++ {
		sn.SNMutex.RLock()
		sectorNum := abi.SectorNumber(sn.SectorNumber)
		sn.SNMutex.RUnlock()
		startnum := i * 1524
		pieceFileA := requireTempFile(bytes.NewReader(databyte[startnum:startnum+508]), 508)
		//将data写入pieceFileA的方法一
		pieceCIDA, err := ffi.GeneratePieceCIDFromFile(sn.SealProofType, pieceFileA, 508)
		if err != nil {
			log.Fatalf("GeneratePieceCIDFromFile Error: %v", err)
		}
		pieceFileA.Seek(0, 0)
		// 将data写入pieceFileA的方法二
		_, _, err = ffi.WriteWithoutAlignment(sn.SealProofType, pieceFileA, 508, stagedSectorFile)
		if err != nil {
			log.Fatalf("WriteWithoutAlignment Error: %v", err.Error())
		}
		pieceFileB := requireTempFile(bytes.NewReader(databyte[startnum+508:startnum+1524]), 1016)
		pieceCIDB, err := ffi.GeneratePieceCIDFromFile(sn.SealProofType, pieceFileB, 1016)
		if err != nil {
			log.Fatalf("GeneratePieceCIDFromFile Error: %v", err.Error())
		}
		_, err = pieceFileB.Seek(0, 0)
		if err != nil {
			log.Fatalf("Seek Error: %v", err.Error())
		}
		_, _, _, err = ffi.WriteWithAlignment(sn.SealProofType, pieceFileB, 1016, stagedSectorFile, []abi.UnpaddedPieceSize{508})
		if err != nil {
			log.Fatalf("WriteWithAlignment Error: %v", err.Error())
		}
		// 构建分片的公共信息
		publicPieces := []abi.PieceInfo{{
			Size:     abi.UnpaddedPieceSize(508).Padded(),
			PieceCID: pieceCIDA,
		}, {
			Size:     abi.UnpaddedPieceSize(1016).Padded(),
			PieceCID: pieceCIDB,
		}}
		// 预提交封装
		sealPreCommitPhase1Output, err := ffi.SealPreCommitPhase1(sn.SealProofType, sectorCacheDirPath, stagedSectorFile.Name(), sealedSectorFile.Name(), sectorNum, sn.MinerID, sn.Ticket, publicPieces)
		if err != nil {
			log.Fatalf("SealPreCommitPhase1 Error: %v", err.Error())
		}
		log.Println("sealPreCommitPhase1Output:", sealPreCommitPhase1Output)
		sealedCID, unsealedCID, err := ffi.SealPreCommitPhase2(sealPreCommitPhase1Output, sectorCacheDirPath, sealedSectorFile.Name())
		if err != nil {
			log.Fatalf("SealPreCommitPhase2 Error: %v", err.Error())
		}
		// 提交封装
		sealCommitPhase1Output, err := ffi.SealCommitPhase1(sn.SealProofType, sealedCID, unsealedCID, sectorCacheDirPath, sealedSectorFile.Name(), sectorNum, sn.MinerID, sn.Ticket, sn.Seed, publicPieces)
		if err != nil {
			log.Fatalf("SealCommitPhase1 Error: %v", err.Error())
		}
		proof, err := ffi.SealCommitPhase2(sealCommitPhase1Output, sectorNum, sn.MinerID)
		if err != nil {
			log.Fatalf("SealCommitPhase2 Error: %v", err.Error())
		}
		sn.SNMutex.Lock()
		sectors[sn.SectorNumber] = &SectorSealedInfor{sectorNum, sealedCID, unsealedCID, proof}
		sn.SectorNumber++
		sn.SNMutex.Unlock()
	}
	sn.CFRSCMutex.Lock()
	sn.CidFnRepSectorCacheDirPathMap[cid_fn] = sectorCacheDirPath
	sn.CidFnRepStagedSectorFileMap[cid_fn] = stagedSectorFile.Name()
	sn.CidFnRepSealedSectorFileMap[cid_fn] = sealedSectorFile.Name()
	sn.CFRSCMutex.Unlock()
	return sectors
}

func PadTo1524Multiple(databyte []byte) ([]byte, int) {
	// 计算当前长度与 1524 模运算的结果
	paddingNeeded := 1524 - (len(databyte) % 1524)
	if paddingNeeded < 1524 && paddingNeeded > 0 {
		// 创建一个填充切片，用 0 填充
		padding := make([]byte, paddingNeeded)
		// 返回原始数据和填充数据的组合以及填充后的长度
		return append(databyte, padding...), (len(databyte) + paddingNeeded) / 1524
	}
	// 如果不需要填充，返回原始数据和原始长度
	fmt.Println("len(databyte)=", len(databyte), "len(databyte)/1524=", len(databyte)/1524)
	return databyte, len(databyte) / 1524
}

// 【供审计方使用的RPC】存储节点接收审计方文件存放通知，阻塞等待客户端存放文件，完成后回复审计方
func (sn *FilecoinSN) FilecoinPutFileNotice(ctx context.Context, preq *pb.FilecoinClientStorageRequest) (*pb.FilecoinClientStorageResponse, error) {
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
		if iscomplete == 2 {
			break
		}
	}
	//文件完成存储，则删除pending元素，给审计方返回消息
	sn.PACNMutex.Lock()
	delete(sn.PendingACPutFNotice, cid_fn)
	sn.PACNMutex.Unlock()
	sn.CFRMMutex.RLock()
	// snl, scidl, uscidl, pl := SectorSealedInforToMessage(sn.ClientFileRepSectorInforMap[cid_fn])
	ssikey := cid_fn + "SSI"
	clientFileRepSectorInfor, _ := sn.GetFilecoinSSIFromCacheOrDB(ssikey)
	snl, scidl, uscidl, pl := SectorSealedInforToMessage(clientFileRepSectorInfor)
	sn.CFRMMutex.RUnlock()
	return &pb.FilecoinClientStorageResponse{ClientId: clientId, Filename: filename, Repno: repno, Snid: sn.SNId, MinerID: int32(sn.MinerID), SectorNum: snl, SealedCID: scidl, UnsealedCID: uscidl, Proof: pl, Message: sn.SNId + " completes the storage of " + cid_fn + "."}, nil
}

// 将扇区信息表转换为消息传输列表
func SectorSealedInforToMessage(fileRepSectorInforMap map[int]*SectorSealedInfor) (sectorNumList []int32, sealedCIDList []string, unsealedCIDList []string, proofList [][]byte) {
	snl := make([]int32, 0)
	scidl := make([]string, 0)
	uscidl := make([]string, 0)
	pl := make([][]byte, 0)
	for key, value := range fileRepSectorInforMap {
		snl = append(snl, int32(key))
		scidl = append(scidl, value.SealedCID.String())
		uscidl = append(uscidl, value.UnsealedCID.String())
		pl = append(pl, value.Proof)
	}
	return snl, scidl, uscidl, pl
}

func requireTempDirPath(prefix string) string {
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		log.Fatalf("temp dirpath err:%v", err)
	}
	fmt.Println(dir)
	return dir
}

func requireTempFile(fileContentsReader io.Reader, size uint64) *os.File {
	file, err := os.CreateTemp("", "")
	if err != nil {
		log.Fatalf("temp file err:%v", err)
	}

	// 预留文件大小
	err = file.Truncate(int64(size))
	if err != nil {
		log.Fatalf("temp file err:%v", err)
	}

	_, err = io.Copy(file, fileContentsReader)
	if err != nil {
		log.Fatalf("temp file err:%v", err)
	}
	// seek to the beginning
	_, err = file.Seek(0, 0)
	if err != nil {
		log.Fatalf("temp file err:%v", err)
	}
	return file
}

// 【供客户端使用的RPC】
func (sn *FilecoinSN) FilecoinGetFile(ctx context.Context, req *pb.FilecoinGetFRequest) (*pb.FilecoinGetFResponse, error) {
	cid_fn := req.ClientId + "-" + req.Filename + "-" + req.Rep
	sn.CFRMMutex.RLock()
	cfrkey := cid_fn + "CFR"
	clientFileRep, err := sn.GetFilecoinCFRFromCacheOrDB(cfrkey)
	cfrvkey := cid_fn + "CFRV"
	clientFileRepV, err2 := sn.GetFilecoinCFRVFromCacheOrDB(cfrvkey)
	// if _, ok := sn.ClientFileRepMap[cid_fn]; !ok {
	if err != nil {
		sn.CFRMMutex.RUnlock()
		e := errors.New("file rep not exist")
		return &pb.FilecoinGetFResponse{Filename: req.Filename, Version: int32(0), Content: ""}, e
		// } else if _, ok := sn.ClientFileRepVMap[cid_fn]; !ok {
	} else if err2 != nil {
		// content := sn.ClientFileRepMap[cid_fn]
		content := clientFileRep
		sn.CFRMMutex.RUnlock()
		e := errors.New("file version not exist")
		return &pb.FilecoinGetFResponse{Filename: req.Filename, Version: int32(0), Content: content}, e
	} else {
		// content := sn.ClientFileRepMap[cid_fn]
		content := clientFileRep
		// v := sn.ClientFileRepVMap[cid_fn]
		v := clientFileRepV
		sn.CFRMMutex.RUnlock()
		return &pb.FilecoinGetFResponse{Filename: req.Filename, Version: int32(v), Content: content}, nil
	}
}

// 【供客户端使用的RPC】
func (sn *FilecoinSN) FilecoinUpdateFile(ctx context.Context, req *pb.FilecoinUpdFRequest) (*pb.FilecoinUpdFResponse, error) {
	clientId := req.ClientId
	filename := req.Filename
	repno := req.Rep
	cid_fn := clientId + "-" + filename + "-" + strconv.Itoa(int(repno))
	//1-阻塞等待收到审计方通知
	newversion := 0
	for {
		sn.PACUFNMutex.RLock()
		_, ok1 := sn.PendingACUpdateFNotice[cid_fn]
		if ok1 {
			newversion = sn.PendingACUpdateFVMap[cid_fn]
			sn.PACUFNMutex.RUnlock()
			break
		}
		sn.PACUFNMutex.RUnlock()
	}
	//2-更新数据分片
	//2-1-比较客户端发来的原始版本是否与当前版本一致
	sn.CFRMMutex.RLock()
	// originVersion := sn.ClientFileRepVMap[cid_fn]
	cfrvkey := cid_fn + "CFRV"
	originVersion, _ := sn.GetFilecoinCFRVFromCacheOrDB(cfrvkey)
	sn.CFRMMutex.RUnlock()
	if originVersion != int(req.Originversion) {
		log.Println(cid_fn, "update error: original version not consist.")
		e := errors.New("original version not consist")
		return nil, e
	}
	//2-2-更新文件到各个列表中
	cfrsis := sn.FilecoinConstructSectors(cid_fn, newversion, req.Newcontent)
	sn.CFRMMutex.Lock()
	// sn.ClientFileRepSectorInforMap[cid_fn] = sn.FilecoinConstructSectors(cid_fn, newversion, req.Newcontent)
	// sn.ClientFileRepSectorInforMap[cid_fn] = cfrsis
	// sn.ClientFileRepMap[cid_fn] = req.Newcontent
	// sn.ClientFileRepVMap[cid_fn] = newversion
	sn.SaveFilecoinDataShardToDB(cid_fn, cfrsis, req.Newcontent, newversion)
	sn.CFRMMutex.Unlock()
	//3-修改PendingACUpdFNotice
	sn.PACUFNMutex.Lock()
	sn.PendingACUpdateFNotice[cid_fn] = 2
	sn.PACUFNMutex.Unlock()
	// 4-告知审计方文件更新结果
	message := "OK"
	return &pb.FilecoinUpdFResponse{Filename: req.Filename, Message: message}, nil
}

// 【供审计方使用的RPC】存储节点接收审计方文件更新通知，阻塞等待客户端存放文件，完成后回复审计方
func (sn *FilecoinSN) FilecoinUpdateFileNotice(ctx context.Context, preq *pb.FilecoinClientUFRequest) (*pb.FilecoinClientUFResponse, error) {
	clientId := preq.ClientId
	filename := preq.Filename
	repno := preq.Rep
	cid_fn := clientId + "-" + filename + "-" + strconv.Itoa(int(repno))
	//写来自审计方的分片更新通知
	sn.PACUFNMutex.Lock()
	sn.PendingACUpdateFNotice[cid_fn] = 1
	sn.PendingACUpdateFVMap[cid_fn] = int(preq.Version)
	sn.PACUFNMutex.Unlock()
	//阻塞监测分片是否已完成更新
	iscomplete := 1
	for {
		sn.PACUFNMutex.RLock()
		iscomplete = sn.PendingACUpdateFNotice[cid_fn]
		sn.PACUFNMutex.RUnlock()
		if iscomplete == 2 {
			break
		} else if iscomplete == 0 {
			log.Fatalln("nnnn")
		}
	}
	//文件完成存储，则删除pending元素，给审计方返回消息
	sn.PACUFNMutex.Lock()
	delete(sn.PendingACUpdateFNotice, cid_fn)
	delete(sn.PendingACUpdateFVMap, cid_fn)
	sn.PACUFNMutex.Unlock()
	//获取文件扇区封装信息
	sn.CFRMMutex.RLock()
	// snl, scidl, uscidl, pl := SectorSealedInforToMessage(sn.ClientFileRepSectorInforMap[cid_fn])
	ssikey := cid_fn + "SSI"
	clientFileRepSectorInfor, _ := sn.GetFilecoinSSIFromCacheOrDB(ssikey)
	snl, scidl, uscidl, pl := SectorSealedInforToMessage(clientFileRepSectorInfor)
	sn.CFRMMutex.RUnlock()
	return &pb.FilecoinClientUFResponse{ClientId: clientId, Filename: filename, Repno: repno, Snid: sn.SNId, MinerID: int32(sn.MinerID), SectorNum: snl, SealedCID: scidl, UnsealedCID: uscidl, Proof: pl, Message: sn.SNId + " completes the update of " + cid_fn + "."}, nil
}

// 【供审计方使用的RPC】预审计请求处理
func (sn *FilecoinSN) FilecoinPreAuditSN(ctx context.Context, req *pb.FilecoinPASNRequest) (*pb.FilecoinPASNResponse, error) {
	if req.Snid != sn.SNId {
		e := errors.New("snid in preaudit request not consist with " + sn.SNId)
		return nil, e
	}
	readyFileMap := make(map[string]map[int]*SectorSealedInfor) //已经准备好的文件扇区信息，key:clientId-filename-i;value:分片列表
	var rFMMutex sync.Mutex
	unreadyFileVMap := make(map[string]int32) //未准备好的文件，即审计方请求已过时，key:clientId-filename-i,value:版本号
	var urFVMMutex sync.Mutex
	cacheDirPathMap := make(map[string]string)   //已经准备好的文件扇区缓存路径表，key:clientId-filename-i,value:目录
	sealedFileNameMap := make(map[string]string) //已经准备好的文件扇区封装路径表，key:clientId-filename-i,value:目录名
	var cdrsfnMutex sync.Mutex
	//遍历审计表，判断是否满足审计方的快照要求
	done := make(chan struct{})
	sn.CFRMMutex.RLock()
	for cid_fni, version := range req.Cidfniv {
		go func(cidfni string, version int) {
			//如果被挑战的版本已完成更新，则加入到readyFileMap中
			//如果当前完成更新的版本小于被挑战的版本，则等待更新完成后加入到readyFileMap中
			//如果当前完成更新的版本大于被挑战的版本，则加入到unreadyFileMap中
			// sn.CFRMMutex.RLock()
			// currentV := sn.ClientFileRepVMap[cidfni]
			cfrvkey := cidfni + "CFRV"
			currentV, _ := sn.GetFilecoinCFRVFromCacheOrDB(cfrvkey)
			// sn.CFRMMutex.RUnlock()
			if currentV <= version {
				// sn.CFRMMutex.RLock()
				rFMMutex.Lock()
				// readyFileMap[cidfni] = sn.ClientFileRepSectorInforMap[cidfni]
				ssikey := cidfni + "SSI"
				readyFileMap[cidfni], _ = sn.GetFilecoinSSIFromCacheOrDB(ssikey)
				rFMMutex.Unlock()
				cdrsfnMutex.Lock()
				cacheDirPathMap[cidfni] = sn.CidFnRepSectorCacheDirPathMap[cidfni]
				sealedFileNameMap[cidfni] = sn.CidFnRepSealedSectorFileMap[cidfni]
				cdrsfnMutex.Unlock()
				// sn.CFRMMutex.RUnlock()
				// } else if currentV < version {
				// 	for {
				// 		sn.CFRMMutex.RLock()
				// 		currentV = sn.ClientFileRepVMap[cidfni]
				// 		sn.CFRMMutex.RUnlock()
				// 		if currentV == version {
				// 			sn.CFRMMutex.RLock()
				// 			rFMMutex.Lock()
				// 			readyFileMap[cidfni] = sn.ClientFileRepSectorInforMap[cidfni]
				// 			rFMMutex.Unlock()
				// 			sn.CFRMMutex.RUnlock()
				// 			break
				// 		}
				// 	}
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
		sn.AuditorFileQueue[req.Auditno] = make(map[string]map[string]map[int]*SectorSealedInfor)
	}
	sn.AuditorFileQueue[req.Auditno][strconv.Itoa(int(req.Currpcno))] = readyFileMap
	if sn.AuditorSectorCacheDirPathMap[req.Auditno] == nil {
		sn.AuditorSectorCacheDirPathMap[req.Auditno] = make(map[string]map[string]string)
	}
	sn.AuditorSectorCacheDirPathMap[req.Auditno][strconv.Itoa(int(req.Currpcno))] = cacheDirPathMap
	if sn.AuditorSealedSectorFileMap[req.Auditno] == nil {
		sn.AuditorSealedSectorFileMap[req.Auditno] = make(map[string]map[string]string)
	}
	sn.AuditorSealedSectorFileMap[req.Auditno][strconv.Itoa(int(req.Currpcno))] = sealedFileNameMap
	sn.AFQMutex.Unlock()
	sn.CFRMMutex.RUnlock()
	return &pb.FilecoinPASNResponse{Isready: len(unreadyFileVMap) == 0, Fversion: unreadyFileVMap, Totalrpcs: req.Totalrpcs, Currpcno: req.Currpcno}, nil
}

// 【供审计方使用的RPC】获取存储节点上所有存储文件副本的聚合存储证明
func (sn *FilecoinSN) FilecoinGetPosSN(ctx context.Context, req *pb.FilecoinGAPSNRequest) (*pb.FilecoinGAPSNResponse, error) {
	sn.AFQMutex.RLock()
	filesi := sn.AuditorFileQueue[req.Auditno][strconv.Itoa(int(req.Currpcno))]
	sn.AFQMutex.RUnlock()
	if filesi == nil {
		e := errors.New("auditorno not exsit")
		return nil, e
	}
	cidfni_sector_proofs := make(map[string]map[int][]prooftypes.PoStProof)
	for cidfni, secotrs := range filesi {
		for sectornum, sectorinfo := range secotrs {
			var randomness32 [32]byte
			copy(randomness32[:], req.Randomness)
			sn.AFQMutex.RLock()
			sectorCacheDirPath := sn.AuditorSectorCacheDirPathMap[req.Auditno][strconv.Itoa(int(req.Currpcno))][cidfni]
			sealedSectorFileName := sn.AuditorSealedSectorFileMap[req.Auditno][strconv.Itoa(int(req.Currpcno))][cidfni]
			sn.AFQMutex.RUnlock()
			pfs := sn.GetStorProofs(cidfni, sectorinfo, randomness32, sectorCacheDirPath, sealedSectorFileName)
			if cidfni_sector_proofs[cidfni] == nil {
				cidfni_sector_proofs[cidfni] = make(map[int][]prooftypes.PoStProof)
			}
			cidfni_sector_proofs[cidfni][sectornum] = pfs
		}
	}
	sn.AFQMutex.Lock()
	delete(sn.AuditorFileQueue[req.Auditno], strconv.Itoa(int(req.Currpcno)))
	if len(sn.AuditorFileQueue[req.Auditno]) == 0 {
		delete(sn.AuditorFileQueue, req.Auditno)
	}
	sn.AFQMutex.Unlock()
	//将cidfni_sector_proofs转换为可发送的消息
	proofsMessage := ProofsToMessage(cidfni_sector_proofs)
	return &pb.FilecoinGAPSNResponse{Auditno: req.Auditno, Proofs: proofsMessage, Totalrpcs: req.Totalrpcs, Currpcno: req.Currpcno}, nil
}

// 为一个扇区生成存储证明
func (sn *FilecoinSN) GetStorProofs(cid_fni string, si *SectorSealedInfor, randomness [32]byte, sectorCacheDirPath string, sealedSectorFileName string) []prooftypes.PoStProof {
	// fmt.Println("1-sectorCacheDirPath:", sectorCacheDirPath, "sealedSectorFileName:", sealedSectorFileName)
	// sn.CFRSCMutex.RLock()
	// sectorCacheDirPath = sn.CidFnRepSectorCacheDirPathMap[cid_fni]
	// sealedSectorFileName = sn.CidFnRepSealedSectorFileMap[cid_fni]
	// sn.CFRSCMutex.RUnlock()
	// fmt.Println("2-sectorCacheDirPath:", sectorCacheDirPath, "sealedSectorFileName:", sealedSectorFileName)
	// 构建Sector的私有信息
	winningPostProofType := abi.RegisteredPoStProof_StackedDrgWinning2KiBV1
	privateInfo := ffi.NewSortedPrivateSectorInfo(ffi.PrivateSectorInfo{
		SectorInfo: prooftypes.SectorInfo{
			SectorNumber: si.SectorNum,
			SealedCID:    si.SealedCID,
		},
		CacheDirPath:     sectorCacheDirPath,
		PoStProofType:    winningPostProofType,
		SealedSectorPath: sealedSectorFileName,
	})
	// 生成存储证明
	proofs, err := ffi.GenerateWinningPoSt(sn.MinerID, privateInfo, randomness[:])
	if err != nil {
		fmt.Println("GenerateWinningPoSt", err.Error())
	}
	return proofs
}

// 将proofs转换为可发送的消息
func ProofsToMessage(cidfni_sector_proofs map[string]map[int][]prooftypes.PoStProof) map[string]*pb.FilecoinBytesArray {
	messages := make(map[string]*pb.FilecoinBytesArray)
	for cidfni, sectors := range cidfni_sector_proofs {
		for sectornum, proofList := range sectors {
			key := cidfni + "-" + strconv.Itoa(sectornum)
			proofbytesList := make([][]byte, 0)
			//对每个proof进行序列化
			for i := 0; i < len(proofList); i++ {
				// 创建一个 bytes.Buffer 作为输出流
				var buf bytes.Buffer
				// 序列化 PoStProof 实例
				err := proofList[i].MarshalCBOR(&buf)
				if err != nil {
					log.Fatal(err)
				}
				// 打印序列化后的字节
				proofbytesList = append(proofbytesList, buf.Bytes())
			}
			messages[key] = &pb.FilecoinBytesArray{Values: proofbytesList}
		}
	}
	return messages
}

// 【供客户端使用的RPC】获取当前节点上对clientID相关文件的存储空间代价
func (sn *FilecoinSN) FilecoinGetSNStorageCost(ctx context.Context, req *pb.FilecoinGSNSCRequest) (*pb.FilecoinGSNSCResponse, error) {
	cid := req.ClientId
	// totalSize := 0
	// //统计所占存储空间大小
	// sn.CFRMMutex.RLock()
	// clientFSMap := sn.ClientFileRepMap
	// sn.CFRMMutex.RUnlock()
	// for key, file := range clientFSMap {
	// 	if strings.HasPrefix(key, cid) {
	// 		totalSize = totalSize + len([]byte(file))
	// 	}
	// }
	// path := "/home/ubuntu/ECDS/data/DB/Filecoin/datashards-" + sn.SNId
	path := "/root/ECDS/data/DB/Filecoin/datashards-" + sn.SNId
	totalSize, _ := util.GetDatabaseSize(path)
	return &pb.FilecoinGSNSCResponse{ClientId: cid, SnId: sn.SNId, Storagecost: int32(totalSize)}, nil
}
