package storjnodes

import (
	"ECDS/encode"
	pb "ECDS/proto" // 根据实际路径修改
	"ECDS/util"
	"bytes"
	"context"
	"log"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type StorjClient struct {
	ClientID string                             //客户端ID
	Rsec     *encode.RSEC                       //纠删码编码器
	ACRPC    pb.StorjACServiceClient            //审计方RPC对象，用于调用审计方的方法
	SNRPCs   map[string]pb.StorjSNServiceClient //存储节点RPC对象列表，key:存储节点地址
}

// 新建客户端，dn和pn分别是数据块和校验块的数量
func NewStorjClient(id string, dn int, pn int, ac_addr string, snaddrmap map[string]string) *StorjClient {
	rsec := encode.NewRSEC(dn, pn)
	// 设置连接审计方服务器的地址
	conn, err := grpc.Dial(ac_addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect to auditor: %v", err)
	}
	// defer conn.Close()
	acrpc := pb.NewStorjACServiceClient(conn)
	//设置连接存储节点服务器的地址
	snrpcs := make(map[string]pb.StorjSNServiceClient)
	for key, value := range snaddrmap {
		snconn, err := grpc.Dial(value, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
		if err != nil {
			log.Fatalf("did not connect to storage node: %v", err)
		}
		sc := pb.NewStorjSNServiceClient(snconn)
		snrpcs[key] = sc
	}
	return &StorjClient{id, rsec, acrpc, snrpcs}
}

// 客户端存放文件
func (client *StorjClient) StorjPutFile(filepath string, filename string) {
	//1-构建存储请求消息
	stor_req := &pb.StorjStorageRequest{
		ClientId: client.ClientID,
		Filename: filename,
		Version:  int32(1),
	}

	// 2-发送存储请求给审计方
	stor_res, err := client.ACRPC.StorjSelectSNs(context.Background(), stor_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}

	//3-从审计方的回复消息中提取存储节点，构建并分发文件分片
	//3.1-读取文件为字符串
	filestr := util.ReadStringFromFile(filepath)
	//3.2-纠删码编码出所有分片
	datashards := client.StorjRSECFilestr(filestr)
	//3.3-将文件复制2f+1次，分别存入2f+1个存储节点
	sn4fd := stor_res.SnsForFd
	done := make(chan struct{})
	randmap := make(map[string][]int32) //记录每个副本对应的随机数集，key:filename-i,value:随机数数组
	rootmap := make(map[string][]byte)  //记录每个副本对应的默克尔树根节点哈希值，key:filename-i,value:根节点哈希值
	var randrootmapMutex sync.Mutex
	//3.3.1-并发分发文件副本到各存储节点
	for i := 0; i < len(sn4fd); i++ {
		go func(sn string, i int) {
			indexKey := filename + "-" + strconv.Itoa(i)
			// 客户端编码
			rands, leafs, root := client.StorjConsMerkleTree(datashards)
			randrootmapMutex.Lock()
			randmap[indexKey] = rands
			rootmap[indexKey] = root
			randrootmapMutex.Unlock()
			// 将二维数组转换为 DataShardSlice 消息
			dataShardSlice := util.Int32SliceToInt32ArraySNSlice(datashards)
			// 发送分片和叶子给存储节点
			pf_req := &pb.StorjPutFRequest{
				ClientId:     client.ClientID,
				Filename:     filename,
				Repno:        int32(i),
				Version:      int32(1),
				DataShards:   dataShardSlice,
				MerkleLeaves: leafs,
			}
			pf_res, err := client.SNRPCs[sn].StorjPutFile(context.Background(), pf_req)
			if err != nil {
				log.Println("storagenode could not process request error:", err)
			}
			// 3.3.3 - 确认存储节点已存储且计算出的哈希值一致
			if !bytes.Equal(pf_res.Root, root) {
				log.Println("the root between storagenode and client not consist")
			}
			// 通知主线程任务完成
			done <- struct{}{}
		}(sn4fd[i], i)
	}

	//3.3.2-等待所有文件分发协程完成
	for i := 0; i < len(sn4fd); i++ {
		<-done
	}
	//4-确认文件存放完成且发送随机数集和默克尔树根给审计方
	//4.1-构造确认请求
	randmessmap := make(map[string]*pb.Int32ArrayAC)
	for key, value := range randmap {
		randmessmap[key] = &pb.Int32ArrayAC{Values: value}
	}
	pfc_req := &pb.StorjPFCRequest{
		ClientId: client.ClientID,
		Filename: filename,
		Randmap:  randmessmap,
		Rootmap:  rootmap,
	}
	// 4.2-发送确认请求给审计方
	pfc_res, err := client.ACRPC.StorjPutFileCommit(context.Background(), pfc_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}
	// 4.3-输出确认回复
	log.Println("received auditor put file ", pfc_res.Filename, " commit respond:", pfc_res.Message)
}

// 使用纠删码编码生成数据块和校验块
func (client *StorjClient) StorjRSECFilestr(filestr string) [][]int32 {
	dsnum := client.Rsec.DataNum + client.Rsec.ParityNum
	//创建一个包含数据块和校验块的[]int32切片
	dataSlice := make([][]int32, dsnum)
	//对文件进行分块后放入切片中
	dsStrings := util.SplitString(filestr, client.Rsec.DataNum)
	for i := 0; i < client.Rsec.DataNum; i++ {
		dataSlice[i] = util.ByteSliceToInt32Slice([]byte(dsStrings[i]))
	}
	//初始化切片中的校验块
	for i := client.Rsec.DataNum; i < dsnum; i++ {
		dataSlice[i] = make([]int32, len(dataSlice[0]))
	}
	//RS纠删码编码
	err := client.Rsec.Encode(dataSlice)
	if err != nil {
		panic(err)
	}
	return dataSlice
}

// 为dataslice构建默克尔树，输出随机数集，每个叶子节点，根节点哈希值
func (client *StorjClient) StorjConsMerkleTree(dataslice [][]int32) ([]int32, [][]byte, []byte) {
	var randomNumbers []int32
	var leafNodes [][]byte

	// 生成叶子节点
	for _, data := range dataslice {
		leaf, randomNumber := util.GenerateLeaf(data)
		leafNodes = append(leafNodes, leaf)
		randomNumbers = append(randomNumbers, randomNumber)
	}

	// 构建默克尔树
	rootHash := util.BuildMerkleTree(leafNodes)
	return randomNumbers, leafNodes, rootHash
}

// 客户端获取文件已提交的最新版本，返回文件数据和版本号
func (client *StorjClient) StorjGetFile(filename string) (string, int) {
	//获取存储该文件的存储节点，依次请求数据分片，若数据分片验证无效，则请求校验分片，若该节点请求响应超时，则请求下一个存储节点
	fileDSs := make([][]int32, 0) //记录存储节点返回的文件分片，用于最后拼接完整文件,key:dsno
	//1-构建获取文件所在的SNs请求消息
	gfsns_req := &pb.StorjGFACRequest{
		ClientId: client.ClientID,
		Filename: filename,
	}
	// 2-发送获取文件所在的SNs请求消息给审计方
	gfsns_res, err := client.ACRPC.StorjGetFileSNs(context.Background(), gfsns_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}
	// 3-依次向SN请求文件数据分片，请求到验证通过后则退出循环
	fsnmap := gfsns_res.Snsds
	version := 0
	for key, value := range fsnmap {
		// 解析副本号
		keysplit := strings.Split(key, "-")
		fn := keysplit[0]
		if fn != filename {
			continue
		}
		rep := keysplit[1]
		// 3.1-构造获取文件的请求消息
		gds_req := &pb.StorjGetFRequest{ClientId: client.ClientID, Filename: filename, Rep: rep, Dsnum: int32(client.Rsec.DataNum)}
		// 3.2-向存储节点发送请求
		gds_res, err := client.SNRPCs[value].StorjGetFile(context.Background(), gds_req)
		if err != nil {
			log.Println(client.ClientID, "get", filename, "rep", rep, "storagenode getfile error:", err)
		} else {
			log.Println(client.ClientID, "get", filename, "rep", rep, "version", gds_res.Version, "success")
			version = int(gds_res.Version)
			// 将获取到的数据分片进行格式转换
			for i := 0; i < len(gds_res.DataShards); i++ {
				ds := gds_res.DataShards[i].Values
				fileDSs = append(fileDSs, ds)
			}
			// 验证数据有效性
			// 3.3-向审计方请求默克尔树根和随机数集
			grr_req := &pb.StorjGRRRequest{ClientId: client.ClientID, Filename: filename, Rep: rep}
			grr_res, err := client.ACRPC.StorjGetRandRoot(context.Background(), grr_req)
			if err != nil {
				log.Println("storagenode getfile error:", err)
			}
			//验证收到的文件是否有效，有效则获取文件结束，否则换下一个存储节点获取文件
			root_sn := util.GenerateMTRoot(fileDSs, grr_res.Rands.Values, gds_res.MerkleLeaves)
			root_ac := grr_res.Root
			if bytes.Equal(root_ac, root_sn) {
				//退出循环
				break
			} else {
				//输出错误
				log.Println(value, "return file error")
			}
		}
	}
	//组装datashards并输出
	str := ""
	for i := 0; i < len(fileDSs); i++ {
		str = str + util.Int32SliceToStr(fileDSs[i])
	}
	// log.Println("receive file:", str)
	return str, version
}

// 客户端更新某个数据分片
func (client *StorjClient) StorjUpdateDataShard(filename string, dsno string, newDSStr string) {
	splitdsno := strings.Split(dsno, "-")
	udpDataRow, _ := strconv.Atoi(splitdsno[1]) //由待更新的dsno导出的待更新分片序号
	// 1-获取旧的文件数据
	oldfilestr, oldV := client.StorjGetFile(filename)
	dsStrings := util.SplitString(oldfilestr, client.Rsec.DataNum)
	// 2-用newDSStr替换文件中相应位置的数据，得到新的文件数据
	dsStrings[udpDataRow] = newDSStr
	newfilestr := ""
	for i := 0; i < len(dsStrings); i++ {
		newfilestr = newfilestr + dsStrings[i]
	}
	newV := oldV + 1
	// 3-用新的文件数据替换掉审计方和存储节点处的文件数据
	// 3.1-向审计方提出更新申请，审计方返回与该文件相关的存储节点，并告知存储节点待更新的文件副本
	uf_req := &pb.StorjUFRequest{ClientId: client.ClientID, Filename: filename, Version: int32(newV)}
	uf_res, err := client.ACRPC.StorjUpdateFileReq(context.Background(), uf_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}
	// 3.2-纠删码编码出所有分片
	datashards := client.StorjRSECFilestr(newfilestr)
	//3.3-将文件复制2f+1次，分别存入2f+1个存储节点
	sn4fd := uf_res.Snsds
	done := make(chan struct{})
	randmap := make(map[string][]int32) //记录每个副本对应的随机数集，key:filename-i,value:随机数数组
	rootmap := make(map[string][]byte)  //记录每个副本对应的默克尔树根节点哈希值，key:filename-i,value:根节点哈希值
	var randrootmapMutex sync.Mutex
	//3.3.1-并发分发新的文件副本到各存储节点进行更新
	for fni, snid := range sn4fd {
		go func(sn string, fni string) {
			splitfni := strings.Split(fni, "-")
			i, _ := strconv.Atoi(splitfni[1])
			// 客户端编码
			rands, leafs, root := client.StorjConsMerkleTree(datashards)
			randrootmapMutex.Lock()
			randmap[fni] = rands
			rootmap[fni] = root
			randrootmapMutex.Unlock()
			// 将二维数组转换为 DataShardSlice 消息
			dataShardSlice := util.Int32SliceToInt32ArraySNSlice(datashards)
			// 发送新的分片和叶子给存储节点
			pf_req := &pb.StorjUpdFRequest{
				ClientId:     client.ClientID,
				Filename:     filename,
				Rep:          int32(i),
				DataShards:   dataShardSlice,
				MerkleLeaves: leafs,
			}
			pf_res, err := client.SNRPCs[sn].StorjUpdateFile(context.Background(), pf_req)
			if err != nil {
				log.Println("storagenode could not process request error:", err)
			}
			// 3.3.3 - 确认存储节点已存储且计算出的哈希值一致
			if !bytes.Equal(pf_res.Root, root) {
				log.Println("the root between storagenode and client not consist")
			}
			// 通知主线程任务完成
			done <- struct{}{}
		}(snid, fni)
	}

	//3.3.2-等待所有文件分发协程完成
	for i := 0; i < len(sn4fd); i++ {
		<-done
	}
	//4-确认文件更新完成且发送随机数集和默克尔树根给审计方
	//4.1-构造确认请求
	randmessmap := make(map[string]*pb.Int32ArrayAC)
	for key, value := range randmap {
		randmessmap[key] = &pb.Int32ArrayAC{Values: value}
	}
	ufc_req := &pb.StorjUFCRequest{
		ClientId: client.ClientID,
		Filename: filename,
		Randmap:  randmessmap,
		Rootmap:  rootmap,
	}
	// 4.2-发送确认请求给审计方
	ufc_res, err := client.ACRPC.StorjUpdateFileCommit(context.Background(), ufc_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}
	// 4.3-输出确认回复
	log.Println("received auditor update file ", ufc_res.Filename, " commit respond:", ufc_res.Message)
}
