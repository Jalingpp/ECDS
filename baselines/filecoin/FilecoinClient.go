package baselines

import (
	pb "ECDS/proto" // 根据实际路径修改"
	"ECDS/util"
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type FilecoinClient struct {
	ClientID string //客户端ID

	ACRPC  pb.FilecoinACServiceClient            //审计方RPC对象，用于调用审计方的方法
	SNRPCs map[string]pb.FilecoinSNServiceClient //存储节点RPC对象列表，key:存储节点地址
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
	snrpcs := make(map[string]pb.FilecoinSNServiceClient)
	for key, value := range snaddrmap {
		snconn, err := grpc.Dial(value, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
		if err != nil {
			log.Fatalf("did not connect to storage node: %v", err)
		}
		sc := pb.NewFilecoinSNServiceClient(snconn)
		snrpcs[key] = sc
	}
	return &FilecoinClient{id, acrpc, snrpcs}
}

// 客户端存放文件,返回原始文件大小（单位：字节）
func (client *FilecoinClient) FilecoinPutFile(filepath string, filename string) int {
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
	// fmt.Println(client.ClientID, filename, "FilecoinSelectSNs:", stor_res.SnsForFd)

	//3-从审计方的回复消息中提取存储节点，构建并分发文件分片
	//3.1-读取文件为字符串
	filestr := util.ReadStringFromFile(filepath)
	filesize := len([]byte(filestr))
	//3.3-将文件复制2f+1次，分别存入2f+1个存储节点(2f+1是AC返回的存储节点数目)
	sn4fd := stor_res.SnsForFd
	done := make(chan struct{})
	//3.3.1-并发分发文件副本到各存储节点
	for i := 0; i < len(sn4fd); i++ {
		go func(sn string, i int) {
			// 发送分片和叶子给存储节点
			pf_req := &pb.FilecoinPutFRequest{
				ClientId: client.ClientID,
				Filename: filename,
				Repno:    int32(i),
				Version:  int32(1),
				Content:  filestr,
			}
			pf_res, err := client.SNRPCs[sn].FilecoinPutFile(context.Background(), pf_req)
			if err != nil {
				log.Println("storagenode could not process request error:", err)
			}
			fmt.Println(client.ClientID, filename, i, sn, "FilecoinPutFile.")
			// 3.3.3 - 确认存储节点已存储且计算出的哈希值一致
			if pf_res.Message != "OK" {
				log.Println("SN storage failed.")
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
	pfc_req := &pb.FilecoinPFCRequest{
		ClientId: client.ClientID,
		Filename: filename,
	}
	// 4.2-发送确认请求给审计方
	_, err = client.ACRPC.FilecoinPutFileCommit(context.Background(), pfc_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}
	return filesize
	// 4.3-输出确认回复
	// log.Println("received auditor put file ", pfc_res.Filename, " commit respond:", pfc_res.Message)
}

// 客户端获取文件已提交的最新版本，返回文件数据和版本号
func (client *FilecoinClient) FilecoinGetFile(filename string) (string, int) {
	//获取存储该文件的存储节点，依次请求数据文件，若数据文件无效，则请求其他数据文件副本，若该节点请求响应超时，则请求下一个存储节点
	//1-构建获取文件所在的SNs请求消息
	gfsns_req := &pb.FilecoinGFACRequest{
		ClientId: client.ClientID,
		Filename: filename,
	}
	// 2-发送获取文件所在的SNs请求消息给审计方
	gfsns_res, err := client.ACRPC.FilecoinGetFileSNs(context.Background(), gfsns_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}
	// 3-依次向SN请求文件数据分片，请求到验证通过后则退出循环
	fsnmap := gfsns_res.Snsds
	content := ""
	for key, value := range fsnmap {
		// 解析副本号
		keysplit := strings.Split(key, "-")
		fn := keysplit[1]
		if fn != filename {
			continue
		}
		rep := keysplit[2]
		// 3.1-构造获取文件的请求消息
		gds_req := &pb.FilecoinGetFRequest{ClientId: client.ClientID, Filename: filename, Rep: rep}
		// 3.2-向存储节点发送请求
		gds_res, err := client.SNRPCs[value].FilecoinGetFile(context.Background(), gds_req)
		if err != nil {
			log.Println(client.ClientID, "get", filename, "rep", rep, "storagenode getfile error:", err)
		} else {
			//比较SN返回的版本号是否与AC给的一致
			if gds_res.Version != gfsns_res.Version {
				log.Println(client.ClientID, "get", filename, "rep", rep, "version", gfsns_res.Version, "failed: version error")
			} else {
				log.Println(client.ClientID, "get", filename, "rep", rep, "version", gds_res.Version, "success:", gds_res.Content)
				content = gds_res.Content
				break
			}
		}
	}
	return content, int(gfsns_res.Version)
}

// 客户端更新某个数据分片
func (client *FilecoinClient) FilecoinUpdateFile(filename string, newfilepath string) {
	// 1-获取新的文件数据
	newfilestr := util.ReadStringFromFile(newfilepath)
	// 3-用新的文件数据替换掉审计方和存储节点处的文件数据
	// 3.1-向审计方提出更新申请，审计方返回与该文件相关的存储节点，并告知存储节点待更新的文件副本
	uf_req := &pb.FilecoinUFRequest{ClientId: client.ClientID, Filename: filename}
	uf_res, err := client.ACRPC.FilecoinUpdateFileReq(context.Background(), uf_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}
	//3.3-将文件复制2f+1次，分别存入2f+1个存储节点
	sn4fd := uf_res.Snsds
	done := make(chan struct{})
	//3.3.1-并发分发新的文件副本到各存储节点进行更新
	for fni, snid := range sn4fd {
		go func(sn string, fni string) {
			splitfni := strings.Split(fni, "-")
			i, _ := strconv.Atoi(splitfni[2])
			// 发送新的分片和叶子给存储节点
			pf_req := &pb.FilecoinUpdFRequest{
				ClientId:      client.ClientID,
				Filename:      filename,
				Rep:           int32(i),
				Originversion: uf_res.Version,
				Newcontent:    newfilestr,
			}
			pf_res, err := client.SNRPCs[sn].FilecoinUpdateFile(context.Background(), pf_req)
			if err != nil {
				log.Println("storagenode could not process request error:", err)
			}
			log.Println(sn, "complete update", pf_res.Filename, pf_res.Repno, ":", pf_res.Message)
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
	ufc_req := &pb.FilecoinUFCRequest{
		ClientId:   client.ClientID,
		Filename:   filename,
		Newversion: uf_res.Version + 1,
	}
	// 4.2-发送确认请求给审计方
	ufc_res, err := client.ACRPC.FilecoinUpdateFileCommit(context.Background(), ufc_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}
	// 4.3-输出确认回复
	log.Println("received auditor update file ", ufc_res.Filename, " commit respond:", ufc_res.Message)
}

// 客户端向所有存储节点请求获取存储空间代价（单位：字节）
func (client *FilecoinClient) FilecoinGetSNsStorageCosts() int {
	totalSize := 0
	var tsMutex sync.RWMutex
	done := make(chan struct{})
	for key, _ := range client.SNRPCs {
		go func(snid string) {
			// 3.1-构造获取存储空间代价请求消息
			gsnsc_req := &pb.FilecoinGSNSCRequest{ClientId: client.ClientID}
			// 3.2-向存储节点发送请求
			gsnsc_res, err := client.SNRPCs[snid].FilecoinGetSNStorageCost(context.Background(), gsnsc_req)
			if err != nil {
				log.Fatalln("storagenode could not process request:", err)
				return
			}
			tsMutex.Lock()
			totalSize = totalSize + int(gsnsc_res.Storagecost)
			tsMutex.Unlock()
			// 通知主线程任务完成
			done <- struct{}{}
		}(key)
	}
	// 等待所有协程完成
	for i := 0; i < len(client.SNRPCs); i++ {
		<-done
	}
	return totalSize
}
