package baselines

import (
	"ECDS/encode"
	pb "ECDS/proto" // 根据实际路径修改
	"ECDS/util"
	"bytes"
	"context"
	"log"
	"strconv"
	"sync"

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
			_, err := client.SNRPCs[sn].SiaPutFileDS(context.Background(), pf_req)
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
			_, err := client.SNRPCs[sn].SiaPutFileDS(context.Background(), pds_req)
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

// 客户端获取文件:isorigin=true,则只返回原始dsnum个分片的文件，否则返回所有
func (client *SiaClient) SiaGetFile(filename string, isorigin bool) string {
	fileDSs := make(map[string][]int32)  //记录存储节点返回的文件分片，用于最后拼接完整文件,key:dsno
	errdsnosn := make(map[string]string) //记录未正常返回分片的snid，key:dsno,value:snid
	fdssMutex := sync.Mutex{}
	//1-构建获取文件所在的SNs请求消息
	gfsns_req := &pb.SiaGFACRequest{
		ClientId: client.ClientID,
		Filename: filename,
	}

	// 2-发送获取文件所在的SNs请求消息给审计方
	gfsns_res, err := client.ACRPC.SiaGetFileSNs(context.Background(), gfsns_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}

	// 3-向SNs请求数据分片
	dssnmap := gfsns_res.Snsds
	snrootmap := gfsns_res.Roots
	done := make(chan struct{})
	for key, value := range dssnmap {
		go func(dsno string, snid string) {
			// 3.1-构造获取分片请求消息
			gds_req := &pb.SiaGetFRequest{ClientId: client.ClientID, Filename: filename, Dsno: dsno}
			// 3.2-向存储节点发送请求
			gds_res, err := client.SNRPCs[snid].SiaGetFileDS(context.Background(), gds_req)
			if err != nil {
				log.Println("storagenode could not process request:", err)
				errdsnosn[dsno] = snid
			} else if gds_res.DataShard == nil {
				log.Println("sn", snid, "return nil datashard", dsno)
				errdsnosn[dsno] = snid
			} else {
				// 3.3-验证数据分片有效性
				// 3.3.1-版本一致性检查
				if gfsns_res.Version != gds_res.Version {
					log.Println("sn", snid, "return outdated datashard", dsno, "versionAC:", gfsns_res.Version, "versionSN:", gds_res.Version)
					errdsnosn[dsno] = snid
				}
				// 3.3.2-数据有效性检查
				datashard := gds_res.DataShard
				dsHash := util.Hash([]byte(util.Int32SliceToStr(datashard)))
				rootAC := snrootmap[snid]
				rootSN := util.GenerateRootByPaths(dsHash, int(gds_res.Index), gds_res.Merklepath)
				if !bytes.Equal(rootAC, rootSN) {
					log.Println("datashard ", dsno, "verification not pass.")
					errdsnosn[dsno] = snid
				}
				// 3.3-将获取到的分片加入到列表中
				fdssMutex.Lock()
				fileDSs[dsno] = datashard
				fdssMutex.Unlock()
			}
			// 通知主线程任务完成
			done <- struct{}{}
		}(key, value)
	}
	// 等待所有协程完成
	for i := 0; i < len(dssnmap); i++ {
		<-done
	}
	// 4-故障仲裁与校验块申请,如果故障一直存在则一直申请，直到系统中备用校验块不足而报错
	// 4.0-设置sn黑名单
	blacksns := make(map[string]string) //key:snid,vlaue:h-黑,白是分片序号
	for {
		// fmt.Println("errdsnosn:", errdsnosn)
		if len(errdsnosn) == 0 {
			break
		} else {
			// 4.0-将未返回分片的存储节点id加入黑名单
			for _, value := range errdsnosn {
				blacksns[value] = "h"
			}
			// 4.1-构造请求消息
			er_req := &pb.SiaGDSERequest{ClientId: client.ClientID, Filename: filename, Errdssn: errdsnosn, Blacksns: blacksns}
			// 4.2-向AC发出请求
			er_res, err := client.ACRPC.SiaGetDSErrReport(context.Background(), er_req)
			if err != nil {
				log.Fatalf("auditor could not process request: %v", err)
			}
			// 4.3-判断AC是否返回足够数量的存储节点，不够时报错
			if len(er_res.Snsds) < len(errdsnosn) {
				log.Fatalf("honest sns for parity shard not enough")
			} else {
				errdsnosn = make(map[string]string) //置空错误名单，重利用
			}
			paritysnrootmap := er_res.Roots
			// 4.4-向存储节点请求校验块
			for key, value := range er_res.Snsds {
				go func(dsno string, snid string) {
					// 4.4.1-构造获取分片请求消息
					gds_req := &pb.SiaGetFRequest{ClientId: client.ClientID, Filename: filename, Dsno: dsno}
					// 4.4.2-向存储节点发送请求
					// fmt.Println("ClientId:", client.ClientID, "filename:", filename, "Dsno:", dsno)
					gds_res, err := client.SNRPCs[snid].SiaGetFileDS(context.Background(), gds_req)
					if err != nil {
						log.Println("storagenode could not process request:", err)
						errdsnosn[dsno] = snid
					} else if gds_res.DataShard == nil {
						log.Println("sn", snid, "return nil parityshard", dsno)
						errdsnosn[dsno] = snid
					} else {
						// 3.3-验证数据分片有效性
						// 3.3.1-版本一致性检查
						if gfsns_res.Version != gds_res.Version {
							log.Println("sn", snid, "return outdated datashard", dsno, "version", gds_res.Version)
							errdsnosn[dsno] = snid
						}
						// 3.3.2-数据有效性检查
						datashard := gds_res.DataShard
						dsHash := util.Hash([]byte(util.Int32SliceToStr(datashard)))
						rootAC := paritysnrootmap[snid]
						rootSN := util.GenerateRootByPaths(dsHash, int(gds_res.Index), gds_res.Merklepath)
						if !bytes.Equal(rootAC, rootSN) {
							log.Println("datashard ", dsno, "verification not pass.")
							errdsnosn[dsno] = snid
						}
						// 3.3-将获取到的分片加入到列表中
						fdssMutex.Lock()
						fileDSs[dsno] = datashard
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
	filestr := client.SiaRecoverFileDS(fileDSs, "d-", "p-")
	if isorigin {
		// log.Println("Get file", filename, ":", filestr)
		return filestr
	}
	// 5.2-若添加过额外的数据分片，则也恢复这部分分片
	turnnum := len(fileDSs)/client.Rsec.DataNum - 1
	for i := 0; i < turnnum; i++ {
		dPrefix := "d-" + strconv.Itoa(client.Rsec.DataNum+turnnum) + "-"
		pPrefix := "p-" + strconv.Itoa(client.Rsec.DataNum+turnnum) + "-"
		filestr = filestr + client.SiaRecoverFileDS(fileDSs, dPrefix, pPrefix)
	}
	// log.Println("Get file", filename, ":", filestr)
	return filestr
}

// 恢复文件或数据块，在GetFile中被调用
func (client *SiaClient) SiaRecoverFileDS(fileDSs map[string][]int32, dprefix string, pprefix string) string {
	//遍历收到的文件分片，按需排放，判断是否需要纠删码解码
	isneedencode := false
	var orderedFileDSs [][]int32
	var rows []int
	for i := 0; i < client.Rsec.DataNum; i++ {
		key := dprefix + strconv.Itoa(i)
		if fileDSs[key] != nil {
			orderedFileDSs = append(orderedFileDSs, fileDSs[key])
			rows = append(rows, i)
		} else {
			isneedencode = true
		}
	}
	for i := 0; i < client.Rsec.ParityNum; i++ {
		key := pprefix + strconv.Itoa(i)
		if fileDSs[key] != nil {
			orderedFileDSs = append(orderedFileDSs, fileDSs[key])
			rows = append(rows, client.Rsec.DataNum+i)
		}
	}
	// 如果不需要解码，按序转换为字符串输出；如果需要解码，则纠删码解码后转换为字符串输出
	filestr := ""
	var orderedFile [][]int32
	if isneedencode {
		orderedFile = client.Rsec.Decode(orderedFileDSs, rows)
	} else {
		orderedFile = orderedFileDSs
	}
	for i := 0; i < len(orderedFile); i++ {
		filestr = filestr + util.Int32SliceToStr(orderedFile[i])
	}
	return filestr
}
