package nodes

import (
	pb "ECDS/proto" // 根据实际路径修改"
	"ECDS/util"
	"bytes"
	"context"
	"log"
	"strconv"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type FilecoinClient struct {
	ClientID string //客户端ID

	ACRPC pb.FilecoinACServiceClient //审计方RPC对象，用于调用审计方的方法
	// SNRPCs map[string]pb.FilecoinSNServiceClient //存储节点RPC对象列表，key:存储节点地址
}

// 新建客户端
func NewFilecoinClient(id string, ac_addr string, snaddrmap map[string]string) *FilecoinClient {
	// 设置连接审计方服务器的地址
	conn, err := grpc.Dial(ac_addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect to auditor: %v", err)
	}
	// defer conn.Close()
	acrpc := pb.NewFilecoinACServiceClient(conn)
	// //设置连接存储节点服务器的地址
	// snrpcs := make(map[string]pb.StorjSNServiceClient)
	// for key, value := range snaddrmap {
	// 	snconn, err := grpc.Dial(value, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	// 	if err != nil {
	// 		log.Fatalf("did not connect to storage node: %v", err)
	// 	}
	// 	sc := pb.NewStorjSNServiceClient(snconn)
	// 	snrpcs[key] = sc
	// }
	return &FilecoinClient{id, acrpc}
}

// 客户端存放文件
func (client *FilecoinClient) FilecoinPutFile(filepath string, filename string) {
	//1-构建存储请求消息
	stor_req := &pb.FilecoinStorageRequest{
		ClientId: client.ClientID,
		Filename: filename,
		Version:  int32(1),
	}

	// 2-发送存储请求给审计方
	stor_res, err := client.ACRPC.FilecoinSelectSNs(context.Background(), stor_req)
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
