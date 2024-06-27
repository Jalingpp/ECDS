package storjnodes

import (
	"ECDS/encode"
	"ECDS/pdp"
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
	}

	// 2-发送存储请求给审计方
	stor_res, err := client.ACRPC.StorjSelectSNs(context.Background(), stor_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}

	//3-从审计方的回复消息中提取存储节点，构建并分发文件分片
	// log.Printf("Received response from Auditor: %s", StorageResponseToString(stor_res))
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

// 客户端获取文件:isorigin=true,则只返回原始dsnum个分片的文件，否则返回所有
func (client *StorjClient) GetFile(filename string, isorigin bool) string {
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
	for key, value := range fsnmap {
		// 解析副本号
		keysplit := strings.Split(key, "-")
		rep := keysplit[1]
		log.Println("rep:", rep)
		// 3.1-构造获取文件的请求消息
		gds_req := &pb.StorjGetFRequest{ClientId: client.ClientID, Filename: filename, Rep: rep, Dsnum: int32(client.Rsec.DataNum)}
		// 3.2-向存储节点发送请求
		gds_res, err := client.SNRPCs[value].StorjGetFile(context.Background(), gds_req)
		if err != nil {
			log.Fatalln("storagenode getfile error:", err)
		} else {
			//验证数据有效性
			// 3.3-向审计方请求默克尔树根和随机数集
			
		}
	}
	// 等待所有协程完成
	for i := 0; i < len(dssnmap); i++ {
		<-done
	}
	// 4-故障仲裁与校验块申请,如果故障一直存在则一直申请，直到系统中备用校验块不足而报错
	// 4.0-设置sn黑名单
	blacksns := make(map[string]string) //key:snid,vlaue:h-黑,白是分片序号
	for {
		if len(errdsnosn) == 0 {
			break
		} else {
			// log.Println("get datashards from sn errors:", errdsnosn)
			// 4.0-将未返回分片的存储节点id加入黑名单
			for _, value := range errdsnosn {
				blacksns[value] = "h"
			}
			// 4.1-构造请求消息
			er_req := &pb.GDSERequest{ClientId: client.ClientID, Filename: filename, Errdssn: errdsnosn, Blacksns: blacksns}
			// 4.2-向AC发出请求
			er_res, err := client.ACRPC.GetDSErrReport(context.Background(), er_req)
			if err != nil {
				log.Fatalf("auditor could not process request: %v", err)
			}
			// 4.3-判断AC是否返回足够数量的存储节点，不够时报错
			if len(er_res.Snsds) < len(errdsnosn) {
				log.Fatalf("honest sns for parity shard not enough")
			} else {
				errdsnosn = make(map[string]string) //置空错误名单，重利用
			}
			// 4.4-向存储节点请求校验块
			// log.Println("request sns of parity shards:", er_res.Snsds)
			for key, value := range er_res.Snsds {
				go func(dsno string, snid string) {
					// 4.4.1-构造获取分片请求消息
					gds_req := &pb.GetDSRequest{ClientId: client.ClientID, Filename: filename, Dsno: dsno}
					// 4.4.2-向存储节点发送请求
					gds_res, err := client.SNRPCs[snid].GetDataShard(context.Background(), gds_req)
					if err != nil {
						log.Println("storagenode could not process request:", err)
						errdsnosn[dsno] = snid
					} else if gds_res.DatashardSerialized == nil {
						log.Println("sn", snid, "return nil parityshard", dsno)
						errdsnosn[dsno] = snid
					} else {
						// 4.4.3-反序列化数据分片并验签
						datashard, _ := util.DeserializeDS(gds_res.DatashardSerialized)
						lv := client.Filecoder.GetVersion(filename, dsno)
						lt := client.Filecoder.GetTimestamp(filename, dsno)
						sigger := client.Filecoder.Sigger
						if !pdp.VerifySig(client.Filecoder.Sigger.Params, sigger.G, sigger.PubKey, datashard.Data, datashard.Sig, lv, lt) {
							log.Println("parityshard ", dsno, " signature verification not pass.")
							errdsnosn[dsno] = snid
						}
						// 4.4.4-将获取到的分片加入到列表中
						fdssMutex.Lock()
						fileDSs[dsno] = datashard.Data
						fdssMutex.Unlock()
						// 4.4.5-将snid加入白名单
						blacksns[snid] = dsno
					}
					// 通知主线程任务完成
					done <- struct{}{}
				}(key, value)
			}
			// 等待所有协程完成
			for i := 0; i < len(er_res.Snsds); i++ {
				<-done
			}
		}
	}
	// 5-恢复完整文件
	// 5.1-恢复以"d-"和"p-"为前缀的文件分片
	filestr := client.RecoverFileDS(fileDSs, "d-", "p-")
	if isorigin {
		// log.Println("Get file", filename, ":", filestr)
		return filestr
	}
	// 5.2-若添加过额外的数据分片，则也恢复这部分分片
	turnnum := len(fileDSs)/client.Filecoder.Rsec.DataNum - 1
	for i := 0; i < turnnum; i++ {
		dPrefix := "d-" + strconv.Itoa(client.Filecoder.Rsec.DataNum+turnnum) + "-"
		pPrefix := "p-" + strconv.Itoa(client.Filecoder.Rsec.DataNum+turnnum) + "-"
		filestr = filestr + client.RecoverFileDS(fileDSs, dPrefix, pPrefix)
	}
	// log.Println("Get file", filename, ":", filestr)
	return filestr
}
