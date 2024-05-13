package nodes

import (
	"ECDS/encode"
	"ECDS/pdp"
	"ECDS/util"
	"context"
	"log"
	"sort"
	"sync"

	pb "ECDS/proto" // 根据实际路径修改

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	ClientID  string                        //客户端ID
	Filecoder *encode.FileCoder             //文件编码器，内含每个文件的<文件名,元信息>表
	ACRPC     pb.ACServiceClient            //审计方RPC对象，用于调用审计方的方法
	SNRPCs    map[string]pb.SNServiceClient //存储节点RPC对象列表，key:存储节点地址
}

// 新建客户端，dn和pn分别是数据块和校验块的数量
func NewClient(id string, dn int, pn int, ac_addr string, snaddrmap map[string]string) *Client {
	filecode := encode.NewFileCoder(dn, pn)
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
	return &Client{id, filecode, c, snrpcs}
}

// 客户端注册，向审计方和存储方广播公开信息
func (client *Client) Register() {
	//1-向审计方注册
	//1.1-构建注册请求消息
	rac_req := &pb.RegistACRequest{
		ClientId: client.ClientID,
		G:        client.Filecoder.Sigger.G,
		PK:       client.Filecoder.Sigger.PubKey,
	}
	// 1.2-发送存储请求给审计方
	rac_res, err := client.ACRPC.RegisterAC(context.Background(), rac_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}
	// 1.3-输出注册结果
	log.Println("recieved register response from auditor:", rac_res.Message)

	// 2-向所有存储节点注册
	done := make(chan struct{})
	for key, value := range client.SNRPCs {
		go func(snid string, snrpc pb.SNServiceClient) {
			// 2.1-构造注册请求消息
			rsn_req := &pb.RegistSNRequest{
				ClientId: client.ClientID,
				G:        client.Filecoder.Sigger.G,
				PK:       client.Filecoder.Sigger.PubKey,
			}
			// 2.2-发送请求消息给存储节点
			rsn_res, err := client.SNRPCs[snid].RegisterSN(context.Background(), rsn_req)
			if err != nil {
				log.Fatalf("storagenode could not process request: %v", err)
			}
			// 2.3-输出存储节点回复消息
			log.Println("recieved register response from ", snid, ":", rsn_res.Message)
			// 2.4-通知主线程任务完成
			done <- struct{}{}
		}(key, value)
	}
	// 等待所有协程完成
	for i := 0; i < len(client.SNRPCs); i++ {
		<-done
	}
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
				Dsno:                datashards[i].DSno,
				DatashardSerialized: datashards[i].SerializeDS(),
			}

			// 3.3.2 - 发送分片存入请求给存储节点
			pds_res, err := client.SNRPCs[sn].PutDataShard(context.Background(), pds_req)
			if err != nil {
				log.Fatalf("storagenode could not process request: %v", err)
			}

			// 3.3.3 - 确认存储节点已存储
			log.Println("Received response from StorageNode", sn, "for datashard", pds_res.Dsno, ". Message:", pds_res.Message)

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
				Dsno:                datashards[i+len(sn4ds)].DSno,
				DatashardSerialized: datashards[i+len(sn4ds)].SerializeDS(),
			}

			// 3.3.2 - 发送分片存入请求给存储节点
			pds_res, err := client.SNRPCs[sn].PutDataShard(context.Background(), pds_req)
			if err != nil {
				log.Fatalf("storagenode could not process request: %v", err)
			}

			// 3.3.3 - 确认存储节点已存储
			log.Println("Received response from StorageNode", sn, "for datashard", pds_res.Dsno, ". Message:", pds_res.Message)

			// 通知主线程任务完成
			done <- struct{}{}
		}(sn4ps[i], i)
	}
	// 等待所有协程完成
	for i := 0; i < len(sn4ds)+len(sn4ps); i++ {
		<-done
	}
	//4-确认文件存放完成且元信息一致
	//4.1-构造确认请求
	pfc_req := &pb.PFCRequest{
		ClientId:   client.ClientID,
		Filename:   filename,
		Versions:   client.Filecoder.MetaFileMap[filename].LatestVersionSlice,
		Timestamps: client.Filecoder.MetaFileMap[filename].LatestTimestampSlice,
	}
	// 4.2-发送确认请求给审计方
	pfc_res, err := client.ACRPC.PutFileCommit(context.Background(), pfc_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}
	//4.3-输出确认回复
	log.Println("received auditor put file ", pfc_res.Filename, " commit respond:", pfc_res.Message)
}

// 客户端获取文件
func (client *Client) GetFile(filename string) {
	fileDSs := make(map[string][]int32)
	fdssMutex := sync.Mutex{}
	//1-构建获取文件所在的SNs请求消息
	gfsns_req := &pb.GFACRequest{
		ClientId: client.ClientID,
		Filename: filename,
	}

	// 2-发送获取文件所在的SNs请求消息给审计方
	gfsns_res, err := client.ACRPC.GetFileSNs(context.Background(), gfsns_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}

	// 3-向SNs请求数据分片
	dssnmap := gfsns_res.Snsds
	done := make(chan struct{})
	for key, value := range dssnmap {
		go func(dsno string, snid string) {
			// 3.1-构造获取分片请求消息
			gds_req := &pb.GetDSRequest{ClientId: client.ClientID, Filename: filename, Dsno: dsno}
			// 3.2-向存储节点发送请求
			gds_res, err := client.SNRPCs[snid].GetDataShard(context.Background(), gds_req)
			if err != nil {
				log.Fatalf("storagenode could not process request: %v", err)
			}
			// 3.3-反序列化数据分片并验签
			datashard, _ := util.DeserializeDS(gds_res.DatashardSerialized)
			lv := client.Filecoder.MetaFileMap[filename].LatestVersionSlice[dsno]
			lt := client.Filecoder.MetaFileMap[filename].LatestTimestampSlice[dsno]
			sigger := client.Filecoder.Sigger
			if !pdp.VerifySig(sigger.Pairing, sigger.G, sigger.PubKey, datashard.Data, datashard.Sig, lv, lt) {
				log.Println("datashard ", dsno, " signature verification not pass.")
			}
			// 3.3-将获取到的分片加入到列表中
			fdssMutex.Lock()
			fileDSs[dsno] = datashard.Data
			fdssMutex.Unlock()
			// 通知主线程任务完成
			done <- struct{}{}
		}(key, value)
	}
	// 等待所有协程完成
	for i := 0; i < len(dssnmap); i++ {
		<-done
	}
	// 4-恢复完整文件
	// 4.1-将fileDSs的键按字典序排序
	var keys []string
	for key := range fileDSs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	// 4.2-按照排序后的键重组file
	filestr := ""
	for _, key := range keys {
		filestr = filestr + util.Int32SliceToStr(fileDSs[key])
	}
	log.Println("Get file", filename, ": ", filestr)
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
