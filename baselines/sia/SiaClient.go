package baselines

import (
	"ECDS/encode"
	pb "ECDS/proto" // 根据实际路径修改
	"ECDS/util"
	"context"
	"log"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type SiaClient struct {
	ClientID string                           //客户端ID
	Rsec     *encode.RSEC                     //纠删码编码器
	ACRPC    pb.SiaACServiceClient            //审计方RPC对象，用于调用审计方的方法
	SNRPCs   map[string]pb.SiaSNServiceClient //存储节点RPC对象列表，key:存储节点地址
}

// 新建客户端，dn和pn分别是数据块和校验块的数量
func NewSiaClient(id string, dn int, pn int, ac_addr string, snaddrmap map[string]string) *SiaClient {
	rsec := encode.NewRSEC(dn, pn)
	// 设置连接审计方服务器的地址
	conn, err := grpc.Dial(ac_addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect to auditor: %v", err)
	}
	// defer conn.Close()
	acrpc := pb.NewSiaACServiceClient(conn)
	//设置连接存储节点服务器的地址
	snrpcs := make(map[string]pb.SiaSNServiceClient)
	for key, value := range snaddrmap {
		snconn, err := grpc.Dial(value, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
		if err != nil {
			log.Fatalf("did not connect to storage node: %v", err)
		}
		sc := pb.NewSiaSNServiceClient(snconn)
		snrpcs[key] = sc
	}
	return &SiaClient{id, rsec, acrpc, snrpcs}
}

// 客户端存放文件
func (client *SiaClient) SiaPutFile(filepath string, filename string) {
	//1-构建存储请求消息
	stor_req := &pb.SiaStorageRequest{
		ClientId: client.ClientID,
		Filename: filename,
		Version:  int32(1),
	}

	// 2-发送存储请求给审计方
	stor_res, err := client.ACRPC.SiaSelectSNs(context.Background(), stor_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}

	//3-从审计方的回复消息中提取存储节点，构建并分发文件分片
	//3.1-读取文件为字符串
	filestr := util.ReadStringFromFile(filepath)
	//3.2-纠删码编码出所有分片
	datashards := client.SiaRSECFilestr(filestr)
	//3.3-将分片存入相应存储节点
	sn4ds := stor_res.SnsForDs
	sn4ps := stor_res.SnsForPs
	done := make(chan struct{})
	//3.3.1-并发分发文件数据分片到各存储节点
	for i := 0; i < len(sn4ds); i++ {
		go func(sn string, i int) {
			// 发送分片和叶子给存储节点
			pf_req := &pb.SiaPutFRequest{
				ClientId:  client.ClientID,
				Filename:  filename,
				Dsno:      "d-" + strconv.Itoa(i),
				Version:   int32(1),
				DataShard: datashards[i],
			}
			_, err := client.SNRPCs[sn].SiaPutFile(context.Background(), pf_req)
			if err != nil {
				log.Println("storagenode could not process request error:", err)
			}
			// log.Println(sn, pf_res.Filename, pf_res.Dsno, "message:", pf_res.Message)
			// 通知主线程任务完成
			done <- struct{}{}
		}(sn4ds[i], i)
	}
	// 3.3.2-并发分发校验分片
	for i := 0; i < len(sn4ps); i++ {
		go func(sn string, i int) {
			// 3.3.1 - 构建分片存入请求消息
			pds_req := &pb.SiaPutFRequest{
				ClientId:  client.ClientID,
				Filename:  filename,
				Dsno:      "p-" + strconv.Itoa(i),
				Version:   int32(1),
				DataShard: datashards[i+len(sn4ds)],
			}
			// 3.3.2 - 发送分片存入请求给存储节点
			_, err := client.SNRPCs[sn].SiaPutFile(context.Background(), pds_req)
			if err != nil {
				log.Fatalf("storagenode could not process request: %v", err)
			}
			// log.Println(sn, pf_res.Filename, pf_res.Dsno, "message:", pf_res.Message)
			// 通知主线程任务完成
			done <- struct{}{}
		}(sn4ps[i], i)
	}
	// 等待所有协程完成
	for i := 0; i < len(sn4ds)+len(sn4ps); i++ {
		<-done
	}
	//4-确认文件存放完成且发送随机数集和默克尔树根给审计方
	//4.1-构造确认请求
	//获取所有分片的哈希值
	dsHashMap := make(map[string][]byte)
	for i := 0; i < len(datashards); i++ {
		dsbytes := []byte(util.Int32SliceToStr(datashards[i]))
		var key string
		if i < len(sn4ds) {
			key = client.ClientID + "-" + filename + "-d-" + strconv.Itoa(i)
		} else {
			key = client.ClientID + "-" + filename + "-p-" + strconv.Itoa(i-len(sn4ds))
		}
		dsHashMap[key] = util.Hash(dsbytes)
	}
	pfc_req := &pb.SiaPFCRequest{
		ClientId:  client.ClientID,
		Filename:  filename,
		Dshashmap: dsHashMap,
	}
	// 4.2-发送确认请求给审计方
	_, err = client.ACRPC.SiaPutFileCommit(context.Background(), pfc_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}
	// 4.3-输出确认回复
	// log.Println("received auditor put file ", pfc_res.Filename, " commit respond:", pfc_res.Message)
}

// 使用纠删码编码生成数据块和校验块
func (client *SiaClient) SiaRSECFilestr(filestr string) [][]int32 {
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
