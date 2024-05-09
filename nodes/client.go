package nodes

import (
	"ECDS/encode"
	"ECDS/pdp"
	"ECDS/util"
	"context"
	"log"
	"strconv"

	pb "ECDS/proto" // 根据实际路径修改

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	ClientID  string                        //客户端ID
	Filecoder *encode.FileCoder             //文件编码器，内含每个文件的<文件名,元信息>表
	Sigger    *pdp.Signature                //客户端的签名器
	ACRPC     pb.ACServiceClient            //审计方RPC对象，用于调用审计方的方法
	SNRPCs    map[string]pb.SNServiceClient //存储节点RPC对象列表，key:存储节点地址
}

// 新建客户端，dn和pn分别是数据块和校验块的数量
func NewClient(id string, dn int, pn int, ac_addr string, snaddrmap map[string]string) *Client {
	filecode := encode.NewFileCoder(dn, pn)
	sigger := pdp.NewSig()
	// 设置连接审计方服务器的地址
	conn, err := grpc.Dial(ac_addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect to auditor: %v", err)
	}
	// defer conn.Close()
	c := pb.NewACServiceClient(conn)
	//设置连接存储节点服务器的地址
	snrpcs := make(map[string]pb.SNServiceClient)
	for key, value := range snaddrmap {
		snconn, err := grpc.Dial(value, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
		if err != nil {
			log.Fatalf("did not connect to storage node: %v", err)
		}
		sc := pb.NewSNServiceClient(snconn)
		snrpcs[key] = sc
	}
	return &Client{id, filecode, sigger, c, snrpcs}
}

// 客户端存放文件
func (client *Client) PutFile(filepath string, filename string) {
	//1-构建存储请求消息
	stor_req := &pb.StorageRequest{
		ClientId: client.ClientID,
		Filename: filename,
	}

	// 2-发送存储请求给审计方
	stor_res, err := client.ACRPC.SelectSNs(context.Background(), stor_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}

	//3-从审计方的回复消息中提取存储节点，构建并分发文件分片
	log.Printf("Received response from Auditor: %s", StorageResponseToString(stor_res))
	//3.1-读取文件为字符串
	filestr := util.ReadStringFromFile(filepath)
	//3.2-纠删码编码出所有分片
	datashards, _ := client.Filecoder.Setup(filename, filestr)
	//3.3-将分片存入相应存储节点
	sn4ds := stor_res.SnsForDs
	sn4ps := stor_res.SnsForPs
	done := make(chan struct{})
	//3.3.1-并发分发数据分片
	for i := 0; i < len(sn4ds); i++ {
		go func(sn string, i int) {
			// 3.3.1 - 构建分片存入请求消息
			pds_req := &pb.PutDSRequest{
				ClientId:            client.ClientID,
				Filename:            filename,
				Dsno:                "d" + strconv.Itoa(i),
				DatashardSerialized: datashards[i].SerializeDS(),
			}

			// 3.3.2 - 发送分片存入请求给存储节点
			pds_res, err := client.SNRPCs[sn].PutDataShard(context.Background(), pds_req)
			if err != nil {
				log.Fatalf("storagenode could not process request: %v", err)
			}

			// 3.3.3 - 确认存储节点已存储
			log.Println("StorageNode", sn, "recieves datashard", pds_res.Dsno, "of file", pds_res.Filename, ". Message:", pds_res.Message)

			// 通知主线程任务完成
			done <- struct{}{}
		}(sn4ds[i], i)
	}
	//3.3.2-并发分发校验分片
	for i := 0; i < len(sn4ps); i++ {
		go func(sn string, i int) {
			// 3.3.1 - 构建分片存入请求消息
			pds_req := &pb.PutDSRequest{
				ClientId:            client.ClientID,
				Filename:            filename,
				Dsno:                "p" + strconv.Itoa(i),
				DatashardSerialized: datashards[i+len(sn4ds)].SerializeDS(),
			}

			// 3.3.2 - 发送分片存入请求给存储节点
			pds_res, err := client.SNRPCs[sn].PutDataShard(context.Background(), pds_req)
			if err != nil {
				log.Fatalf("storagenode could not process request: %v", err)
			}

			// 3.3.3 - 确认存储节点已存储
			log.Println("StorageNode", sn, "recieves datashard", pds_res.Dsno, "of file", pds_res.Filename, ". Message:", pds_res.Message)

			// 通知主线程任务完成
			done <- struct{}{}
		}(sn4ps[i], i)
	}
	// 等待所有协程完成
	for i := 0; i < len(sn4ds)+len(sn4ps); i++ {
		<-done
	}
}

// 存储回复消息转为字符串
func StorageResponseToString(sres *pb.StorageResponse) string {
	str := "StorageResponse:{Filename:" + sres.Filename + ",SN4DS:{"
	for i := 0; i < len(sres.SnsForDs); i++ {
		str = str + sres.SnsForDs[i] + ","
	}
	str = str + "},SN4PS:{"
	for i := 0; i < len(sres.SnsForPs); i++ {
		str = str + sres.SnsForPs[i] + ","
	}
	str = str + "}}"
	return str
}
