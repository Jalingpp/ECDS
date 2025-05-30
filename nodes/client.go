package nodes

import (
	"ECDS/encode"
	"ECDS/pdp"
	"ECDS/util"
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

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
	// 设置连接审计方服务器的地址
	conn, err := grpc.Dial(ac_addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		log.Println("grpc.Dial err")
		log.Fatalf("did not connect to auditor: %v", err)
	}
	// defer conn.Close()
	acrpc := pb.NewACServiceClient(conn)
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
	//向审计方请求Params和g
	gpg_req := &pb.GetPGRequest{ClientId: id}
	gpg_res, err := acrpc.GetParamsG(context.Background(), gpg_req)
	if err != nil {
		log.Fatalf("storagenode could not process request: %v", err)
	}
	filecode := encode.NewFileCoder(dn, pn, gpg_res.Params, gpg_res.G)
	return &Client{id, filecode, acrpc, snrpcs}
}

// 客户端注册，向审计方和存储方广播公开信息
func (client *Client) Register() {
	//1-向审计方注册
	//1.1-构建注册请求消息
	rac_req := &pb.RegistACRequest{
		ClientId: client.ClientID,
		PK:       client.Filecoder.Sigger.PubKey,
	}
	// 1.2-发送存储请求给审计方
	_, err := client.ACRPC.RegisterAC(context.Background(), rac_req)
	if err != nil {
		log.Println("client regist in ac error:", err)
	}
	// else {
	// 	// 1.3-输出注册结果
	// 	// log.Println("recieved register response from auditor:", rac_res.Message)
	// }

	// 2-向所有存储节点注册
	done := make(chan struct{})
	for key, value := range client.SNRPCs {
		go func(snid string, snrpc pb.SNServiceClient) {
			// 2.1-构造注册请求消息
			rsn_req := &pb.CRegistSNRequest{
				ClientId: client.ClientID,
				PK:       client.Filecoder.Sigger.PubKey,
			}
			// 2.2-发送请求消息给存储节点
			_, err := client.SNRPCs[snid].ClientRegisterSN(context.Background(), rsn_req)
			if err != nil {
				log.Println("client regist in sn error:", err)
			}
			// else {
			// 	// 2.3-输出存储节点回复消息
			// 	log.Println("recieved register response from ", snid, ":", rsn_res.Message)
			// }
			// 2.4-通知主线程任务完成
			done <- struct{}{}
		}(key, value)
	}
	// 等待所有协程完成
	for i := 0; i < len(client.SNRPCs); i++ {
		<-done
	}
}

// 客户端存放文件,返回原始文件大小（单位：字节）
func (client *Client) PutFile(filepath string, filename string) int {
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
	// log.Printf("Received response from Auditor: %s", StorageResponseToString(stor_res))
	//3.1-读取文件为字符串
	filestr := util.ReadStringFromFile(filepath)
	filesize := len([]byte(filestr))
	//3.2-纠删码编码出所有分片
	datashards := client.Filecoder.Setup(filename, filestr)
	// log.Printf("Client ec datashards completed.")
	//3.3-将分片存入相应存储节点
	sn4ds := stor_res.SnsForDs
	sn4ps := stor_res.SnsForPs
	done := make(chan struct{})
	//3.3.1-并发分发数据分片
	for i := 0; i < len(sn4ds); i++ {
		go func(sn string, i int) {
			//验签
			if !pdp.VerifySig(client.Filecoder.Sigger.Params, client.Filecoder.Sigger.G, client.Filecoder.Sigger.PubKey, datashards[i].Data, datashards[i].Sig, datashards[i].Version, datashards[i].Timestamp) {
				log.Println(datashards[i], "signature verify error!")
			}
			// 3.3.1 - 构建分片存入请求消息
			pds_req := &pb.PutDSRequest{
				ClientId:            client.ClientID,
				Filename:            filename,
				Dsno:                datashards[i].DSno,
				DatashardSerialized: datashards[i].SerializeDS(),
			}
			// log.Println("Before sending datashards to ", sn)
			// 3.3.2 - 发送分片存入请求给存储节点
			_, err := client.SNRPCs[sn].PutDataShard(context.Background(), pds_req)
			if err != nil {
				log.Println("storagenode could not process request error:", err)
			}

			// 3.3.3 - 确认存储节点已存储
			// log.Println(client.ClientID, "received response from", sn, "for", pds_res.Dsno, "of", filename, ". Message:", pds_res.Message)

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
			_, err := client.SNRPCs[sn].PutDataShard(context.Background(), pds_req)
			if err != nil {
				log.Fatalf("storagenode could not process request: %v", err)
			}

			// 3.3.3 - 确认存储节点已存储
			// log.Println(client.ClientID, "received response from", sn, "for", pds_res.Dsno, "of", filename, ". Message:", pds_res.Message)

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
		Versions:   client.Filecoder.GetVersions(filename),
		Timestamps: client.Filecoder.GetTimestamps(filename),
	}
	// 4.2-发送确认请求给审计方
	_, err = client.ACRPC.PutFileCommit(context.Background(), pfc_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}
	return filesize
	//4.3-输出确认回复
	// log.Println("received auditor put file ", pfc_res.Filename, " commit respond:", pfc_res.Message)
}

// 客户端获取文件:isorigin=true,则只返回原始dsnum个分片的文件，否则返回所有
func (client *Client) GetFile(filename string, isorigin bool) string {
	fileDSs := make(map[string][]int32)  //记录存储节点返回的文件分片，用于最后拼接完整文件,key:dsno
	errdsnosn := make(map[string]string) //记录未正常返回分片的snid，key:dsno,value:snid
	// pairing, _ := pbc.NewPairingFromString(client.Filecoder.Sigger.Params)
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
				log.Println("storagenode could not process request:", err)
				errdsnosn[dsno] = snid
			} else if gds_res.DatashardSerialized == nil {
				log.Println("sn", snid, "return nil datashard", dsno)
				errdsnosn[dsno] = snid
			} else {
				// 3.3-反序列化数据分片并验签
				datashard, _ := util.DeserializeDS(gds_res.DatashardSerialized)
				lv := client.Filecoder.GetVersion(filename, dsno)
				lt := client.Filecoder.GetTimestamp(filename, dsno)
				sigger := client.Filecoder.Sigger
				if !pdp.VerifySig(client.Filecoder.Sigger.Params, sigger.G, sigger.PubKey, datashard.Data, datashard.Sig, lv, lt) {
					log.Println("datashard ", dsno, " signature verification not pass.")
					errdsnosn[dsno] = snid
				}
				// 3.3-将获取到的分片加入到列表中
				fdssMutex.Lock()
				fileDSs[dsno] = datashard.Data
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

// 恢复文件或数据块，在GetFile中被调用
func (client *Client) RecoverFileDS(fileDSs map[string][]int32, dprefix string, pprefix string) string {
	//遍历收到的文件分片，按需排放，判断是否需要纠删码解码
	isneedencode := false
	var orderedFileDSs [][]int32
	var rows []int
	for i := 0; i < client.Filecoder.Rsec.DataNum; i++ {
		key := dprefix + strconv.Itoa(i)
		if fileDSs[key] != nil {
			orderedFileDSs = append(orderedFileDSs, fileDSs[key])
			rows = append(rows, i)
		} else {
			isneedencode = true
		}
	}
	for i := 0; i < client.Filecoder.Rsec.ParityNum; i++ {
		key := pprefix + strconv.Itoa(i)
		if fileDSs[key] != nil {
			orderedFileDSs = append(orderedFileDSs, fileDSs[key])
			rows = append(rows, client.Filecoder.Rsec.DataNum+i)
		}
	}
	// 如果不需要解码，按序转换为字符串输出；如果需要解码，则纠删码解码后转换为字符串输出
	filestr := ""
	var orderedFile [][]int32
	if isneedencode {
		orderedFile = client.Filecoder.Rsec.Decode(orderedFileDSs, rows)
	} else {
		orderedFile = orderedFileDSs
	}
	for i := 0; i < len(orderedFile); i++ {
		filestr = filestr + util.Int32SliceToStr(orderedFile[i])
	}
	return filestr
}

// 客户端更新某个数据分片
func (client *Client) UpdateDS(filename string, dsno string, newDSStr string) {
	splitdsno := strings.Split(dsno, "-")
	udpDataRow, _ := strconv.Atoi(splitdsno[1]) //由待更新的dsno导出的待更新分片序号
	randSource := rand.NewSource(time.Now().UnixNano())
	r := rand.New(randSource) //设置随机种子
	// pairing, _ := pbc.NewPairingFromString(client.Filecoder.Sigger.Params)
	// 1-获取旧的数据分片(和校验块所在的存储节点id)
	oldDSs, dsSNs, psSNs, err := client.GetDataShard(filename, dsno, true)
	if err != nil {
		log.Fatalf("get old datashard error: %v", err)
	}
	// 2-存储节点更新数据分片和所有校验块：分别考虑数据分片是单个和多个的情况
	if len(oldDSs) == 1 {
		// 2.1-旧数据分片为单个：获取旧分片
		var oldDS *util.DataShard
		for _, value := range oldDSs {
			oldDS = value
		}
		// log.Println("get old datashard", oldDS.DSno, ":", oldDS.Data)
		// 2.2-旧数据分片为单个：更新分片，得到校验块增量
		incParityShards := client.Filecoder.UpdateDataShard(filename, udpDataRow, oldDS, newDSStr)
		// 2.3-将更新后的数据分片发送给存储节点覆盖旧分片
		var newDSnos []string
		var newDSs []*util.DataShard
		newDSnos = append(newDSnos, oldDS.DSno)
		newDSs = append(newDSs, oldDS)
		done := make(chan struct{})
		go client.UpdDSsOnSN(filename, newDSnos, newDSs, dsSNs[oldDS.DSno])
		// 2.4-将校验块增量发送给相应存储节点进行更新
		for i := 0; i < len(incParityShards); i++ {
			go func(ipsno int) {
				psno := "p-" + strconv.Itoa(ipsno)
				// 2.4.1-构造请求
				randomNum := r.Int31()
				ipslist := make([][]byte, 0)
				ipslist = append(ipslist, incParityShards[ipsno].SerializeDS())
				pips_req := &pb.PutIPSRequest{ClientId: client.ClientID, Filename: filename, Psno: psno, DatashardsSerialized: ipslist, Randomnum: randomNum}
				// 2.4.2-向存储节点发送请求
				pips_res, err := client.SNRPCs[psSNs[psno]].PutIncParityShards(context.Background(), pips_req)
				if err != nil {
					log.Fatalf("storage could not process request: %v", err)
				}
				// 2.4.3-验证存储证明
				pos, errpos := pdp.DeserializePOS(pips_res.PosSerialized)
				if errpos != nil {
					log.Fatalf("updated parityshard pos deserialize error: %v", errpos)
				}
				sigger := client.Filecoder.Sigger
				v := client.Filecoder.GetVersion(filename, psno)
				t := client.Filecoder.GetTimestamp(filename, psno)
				isCorrect := pdp.VerifyPos(pos, client.Filecoder.Sigger.Params, sigger.G, sigger.PubKey, v, t, randomNum)
				if !isCorrect {
					log.Println("storage", psSNs[psno], "update parityshard", psno, "of", client.ClientID, filename, "pos verify not pass")
				}
				// 通知主线程任务完成
				done <- struct{}{}
			}(i)
		}
		//等待所有线程执行完成
		for i := 0; i < len(incParityShards); i++ {
			<-done
		}
		// 2.5-向审计方确认更新完成
		newversions := make(map[string]int32)
		newtimestamps := make(map[string]string)
		newversions[oldDS.DSno] = oldDS.Version
		newtimestamps[oldDS.DSno] = oldDS.Timestamp
		for i := 0; i < len(incParityShards); i++ {
			newDSnos = append(newDSnos, incParityShards[i].DSno)
			newversions[incParityShards[i].DSno] = incParityShards[i].Version
			newtimestamps[incParityShards[i].DSno] = incParityShards[i].Timestamp
		}
		client.UpdateDSACCommit(filename, newDSnos, newversions, newtimestamps)
		// if client.UpdateDSACCommit(filename, newDSnos, newversions, newtimestamps) {
		// 	log.Println(client.ClientID, filename, newDSnos, "update completed.")
		// }
	} else if len(oldDSs) > 1 {
		// 旧数据分片为多个
		// 2.1-切分新数据字符串
		newDSstrs := util.SplitString(newDSStr, len(oldDSs))
		// 2.2-遍历旧分片，分别生成校验块增量
		odsIPS := make(map[string][]util.DataShard) //每个旧数据分片更新引发的校验块增量
		var odsIPSMutex sync.Mutex                  //odsIPS的访问锁
		var oldDSData []int32                       //合并后的分片数据
		for key, value := range oldDSs {
			oldDSData = append(oldDSData, value.Data...)
			keysplit := strings.Split(key, "-")
			udpDR, _ := strconv.Atoi(keysplit[2])
			go func(odsno string, fn string, udr int, oDSD *util.DataShard, ndstr string) {
				ips := client.Filecoder.UpdateDataShard(fn, udr, oDSD, ndstr)
				odsIPSMutex.Lock()
				odsIPS[odsno] = ips
				odsIPSMutex.Unlock()
			}(key, filename, udpDR, value, newDSstrs[udpDR])
		}
		// log.Println("get oldDS:", util.Int32SliceToStr(oldDSData))
		// 2.3-将属于同一个存储节点的分片合并
		sndsnomap := make(map[string][]string) //key:snid,value:dsnos
		for key, value := range dsSNs {
			if sndsnomap[value] == nil {
				sndsnomap[value] = make([]string, 0)
			}
			sndsnomap[value] = append(sndsnomap[value], key)
		}
		// 2.4-将属于同一个存储节点的分片发送至存储节点
		uds_done := make(chan struct{})
		for key, value := range sndsnomap {
			dss := make([]*util.DataShard, 0)
			for i := 0; i < len(value); i++ {
				dss = append(dss, oldDSs[value[i]])
			}
			go func(filename string, value []string, dss []*util.DataShard, key string) {
				client.UpdDSsOnSN(filename, value, dss, key)
				uds_done <- struct{}{}
			}(filename, value, dss, key)
		}
		//等待所有线程执行完成
		for i := 0; i < len(sndsnomap); i++ {
			<-uds_done
		}
		// 2.5-将属于同一个存储节点的校验分片合并
		psipsmap := make(map[string][]util.DataShard) //key:psno,value:序列化后的增量校验块列表
		for _, value := range odsIPS {
			for i := 0; i < len(value); i++ {
				if psipsmap[value[i].DSno] == nil {
					psipsmap[value[i].DSno] = make([]util.DataShard, 0)
				}
				psipsmap[value[i].DSno] = append(psipsmap[value[i].DSno], value[i])
			}
		}
		// 2.6-将属于同一个存储节点的校验分片发送至存储节点进行增量更新
		uips_done := make(chan struct{})
		for key, value := range psipsmap {
			snid := psSNs[key]
			seipslist := make([][]byte, 0)
			for i := 0; i < len(value); i++ {
				seipslist = append(seipslist, value[i].SerializeDS())
			}
			go func(snid string, psno string, ipss [][]byte) {
				//构造发送请求消息
				randomNum := r.Int31()
				uips_req := &pb.PutIPSRequest{ClientId: client.ClientID, Filename: filename, Psno: psno, DatashardsSerialized: ipss, Randomnum: randomNum}
				//发送请求给存储节点
				uips_res, err := client.SNRPCs[snid].PutIncParityShards(context.Background(), uips_req)
				if err != nil {
					log.Fatalf("storage could not process request: %v", err)
				}
				// 验证存储证明
				pos, errpos := pdp.DeserializePOS(uips_res.PosSerialized)
				if errpos != nil {
					log.Fatalf("updated parityshard pos deserialize error: %v", errpos)
				}
				sigger := client.Filecoder.Sigger
				v := client.Filecoder.GetVersion(filename, psno)
				t := client.Filecoder.GetTimestamp(filename, psno)
				isCorrect := pdp.VerifyPos(pos, client.Filecoder.Sigger.Params, sigger.G, sigger.PubKey, v, t, randomNum)
				if !isCorrect {
					log.Println("storage", psSNs[psno], "update parityshard", psno, "pos verify not pass")
				}
				// 通知主线程任务完成
				uips_done <- struct{}{}
			}(snid, key, seipslist)
		}
		//等待所有线程执行完成
		for i := 0; i < len(psipsmap); i++ {
			<-uips_done
		}
		// 2.7-向审计方确认更新完成
		newDSnos := make([]string, 0)
		newVersions := make(map[string]int32)
		newTimestamps := make(map[string]string)
		for key, value := range oldDSs {
			newDSnos = append(newDSnos, key)
			newVersions[key] = value.Version
			newTimestamps[key] = value.Timestamp
		}
		for key, value := range psipsmap {
			newDSnos = append(newDSnos, key)
			newVersions[key] = value[len(value)-1].Version
			newTimestamps[key] = value[len(value)-1].Timestamp
		}
		client.UpdateDSACCommit(filename, newDSnos, newVersions, newTimestamps)
		// if client.UpdateDSACCommit(filename, newDSnos, newVersions, newTimestamps) {
		// 	log.Println("datashard update completed.")
		// }
	} else {
		log.Println("old datashard not exist")
	}
}

// 客户端向审计方确认完成数据分片更新
func (client *Client) UpdateDSACCommit(filename string, dsnos []string, versions map[string]int32, times map[string]string) bool {
	//构造确认请求消息
	uds_req := &pb.UDSCRequest{ClientId: client.ClientID, Filename: filename, Dsnos: dsnos, Versions: versions, Timestamps: times}
	//向审计方发送请求
	uds_res, err := client.ACRPC.UpdateDSCommit(context.Background(), uds_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}
	//处理审计方回复
	// log.Println("received update DS commit message from auditor:", uds_res.Message)
	return len(uds_res.Dssnmap) == len(dsnos)
}

// 客户端更新SN上的一个或多个数据分片
func (client *Client) UpdDSsOnSN(filename string, dsnos []string, dss []*util.DataShard, snid string) bool {
	//对所有数据分片序列化
	var seDss [][]byte
	for i := 0; i < len(dss); i++ {
		seDss = append(seDss, dss[i].SerializeDS())
	}
	//构造更新请求消息
	udss_req := &pb.UpdDSsRequest{ClientId: client.ClientID, Filename: filename, Dsnos: dsnos, DatashardsSerialized: seDss}
	//向存储节点发送更新请求
	udss_res, err := client.SNRPCs[snid].UpdateDataShards(context.Background(), udss_req)
	if err != nil {
		log.Fatalf("storage could not process request: %v", err)
	}
	// log.Println("received update datashards message from storagenode:", udss_res.Message)
	return len(dsnos) == len(udss_res.Dsnos)
}

// 客户端获取某个数据分片：返回分片或所有子分片，同时返回所有数据块和校验块所在的存储节点id列表，key是dsno/psno，value是snid
func (client *Client) GetDataShard(filename string, dsno string, isupdate bool) (map[string]*util.DataShard, map[string]string, map[string]string, error) {
	// 1.1-构造获取数据分片所在存储节点id的请求
	gdssn_req := &pb.GDSSNRequest{ClientId: client.ClientID, Filename: filename, Dsno: dsno, Isupdate: isupdate}
	// 1.2-发送获取分片所在存储节点id的请求消息给审计方
	gdssn_res, err := client.ACRPC.GetDSSn(context.Background(), gdssn_req)
	if err != nil {
		log.Fatalf("auditor could not process request: %v", err)
	}
	// 1.3-向存储节点请求数据分片（可能是多个子分片）
	dssnmap := gdssn_res.Snsds
	DSs := make(map[string]*util.DataShard) //记录存储节点返回的文件分片，用于最后拼接完整分片,key:dsno
	errdsnosn := make(map[string]string)    //记录未正常返回分片的snid，key:dsno,value:snid
	dssMutex := sync.Mutex{}
	// pairing, _ := pbc.NewPairingFromString(client.Filecoder.Sigger.Params)
	done := make(chan struct{})
	for key, value := range dssnmap {
		go func(dsno string, snid string) {
			// 1.3.1-构造获取分片请求消息
			gds_req := &pb.GetDSRequest{ClientId: client.ClientID, Filename: filename, Dsno: dsno}
			// 1.3.2-向存储节点发送请求
			gds_res, err := client.SNRPCs[snid].GetDataShard(context.Background(), gds_req)
			if err != nil {
				// log.Println("storagenode could not process request:", err)
				errdsnosn[dsno] = snid
			} else if gds_res.DatashardSerialized == nil {
				// log.Println("sn", snid, "return nil datashard", dsno)
				errdsnosn[dsno] = snid
			} else {
				// 1.3.3-反序列化数据分片并验签
				datashard, _ := util.DeserializeDS(gds_res.DatashardSerialized)
				lv := client.Filecoder.GetVersion(filename, dsno)
				lt := client.Filecoder.GetTimestamp(filename, dsno)
				sigger := client.Filecoder.Sigger
				if !pdp.VerifySig(client.Filecoder.Sigger.Params, sigger.G, sigger.PubKey, datashard.Data, datashard.Sig, lv, lt) {
					// log.Println("datashard ", dsno, " signature verification not pass.")
					errdsnosn[dsno] = snid
				}
				// 1.3.4-将获取到的分片加入到列表中
				dssMutex.Lock()
				DSs[dsno] = datashard
				dssMutex.Unlock()
			}
			// 通知主线程任务完成
			done <- struct{}{}
		}(key, value)
	}
	// 等待所有协程完成
	for i := 0; i < len(dssnmap); i++ {
		<-done
	}
	// 1.4-故障仲裁与校验块申请,如果故障一直存在则一直申请，直到系统中备用校验块不足而报错
	// 1.4.1-如果分片只有一个且发生故障，则需要通过恢复文件恢复分片
	if len(dssnmap) == 1 {
		for key, _ := range errdsnosn {
			filestr := client.GetFile(filename, true)
			//对文件进行分块，选择相应序号的分片放入切片中
			dsStrings := util.SplitString(filestr, client.Filecoder.Rsec.DataNum)
			keysplit := strings.Split(key, "-")
			no, err := strconv.Atoi(keysplit[1])
			if err != nil {
				fmt.Println("dsno format error:", err)
				e := errors.New("dsno format error")
				return nil, nil, nil, e
			}
			//构建一个数据分片
			dataSlice := util.ByteSliceToInt32Slice([]byte(dsStrings[no]))
			client.Filecoder.MFMMutex.RLock()
			oldV := client.Filecoder.GetVersion(filename, dsno)
			oldT := client.Filecoder.GetTimestamp(filename, dsno)
			client.Filecoder.MFMMutex.RUnlock()
			sig := client.Filecoder.Sigger.GetSig(dataSlice, oldT, oldV)
			datashard := util.NewDataShard(dsno, dataSlice, sig, oldV, oldT)
			DSs[key] = datashard
		}
		return DSs, gdssn_res.Snsds, gdssn_res.Snsps, nil
	} else {
		// 1.4.2-如果分片不只有一个且发生故障，则通过纠删码解码恢复分片
		// 1.4.2.1-设置sn黑名单
		blacksns := make(map[string]string) //key:snid,vlaue:h-黑,白是分片序号
		for {
			if len(errdsnosn) == 0 {
				break
			} else {
				// log.Println("get subdatashards from sn errors:", errdsnosn)
				// 1.4.2.2-将未返回分片的存储节点id加入黑名单
				for _, value := range errdsnosn {
					blacksns[value] = "h"
				}
				// 1.4.2.3-构造请求消息
				er_req := &pb.GDSERequest{ClientId: client.ClientID, Filename: filename, Errdssn: errdsnosn, Blacksns: blacksns}
				// 1.4.2.4-向AC发出请求
				er_res, err := client.ACRPC.GetDSErrReport(context.Background(), er_req)
				if err != nil {
					log.Fatalf("auditor could not process request: %v", err)
				}
				// 1.4.2.5-判断AC是否返回足够数量的存储节点，不够时报错
				if len(er_res.Snsds) < len(errdsnosn) {
					log.Fatalf("honest sns for parity subshard not enough")
				} else {
					errdsnosn = make(map[string]string) //置空错误名单，重利用
				}
				// 1.4.2.6-向存储节点请求校验块
				// log.Println("request sns of parity subshards:", er_res.Snsds)
				for key, value := range er_res.Snsds {
					go func(dsno string, snid string) {
						// 1.4.2.6.1-构造获取分片请求消息
						gds_req := &pb.GetDSRequest{ClientId: client.ClientID, Filename: filename, Dsno: dsno}
						// 1.4.2.6.2-向存储节点发送请求
						gds_res, err := client.SNRPCs[snid].GetDataShard(context.Background(), gds_req)
						if err != nil {
							// log.Println("storagenode could not process request:", err)
							errdsnosn[dsno] = snid
						} else if gds_res.DatashardSerialized == nil {
							// log.Println("sn", snid, "return nil paritysubshard", dsno)
							errdsnosn[dsno] = snid
						} else {
							// 1.4.2.6.3-反序列化数据分片并验签
							datashard, _ := util.DeserializeDS(gds_res.DatashardSerialized)
							lv := client.Filecoder.GetVersion(filename, dsno)
							lt := client.Filecoder.GetTimestamp(filename, dsno)
							sigger := client.Filecoder.Sigger
							if !pdp.VerifySig(client.Filecoder.Sigger.Params, sigger.G, sigger.PubKey, datashard.Data, datashard.Sig, lv, lt) {
								// log.Println("paritysubshard ", dsno, " signature verification not pass.")
								errdsnosn[dsno] = snid
							}
							// 1.4.2.6.4-将获取到的分片加入到列表中
							dssMutex.Lock()
							DSs[dsno] = datashard
							dssMutex.Unlock()
							// 1.4.2.6.5-将snid加入白名单
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
		// 1.4.2.7-如果未发生故障，只需要将所有子分片拼接；如果发生过故障，则需要纠删码恢复
		if len(blacksns) == 0 {
			return DSs, gdssn_res.Snsds, gdssn_res.Snsps, nil
		} else {
			var orderedDSs [][]int32
			var rows []int
			dprefix := dsno + "-"
			for i := 0; i < client.Filecoder.Rsec.DataNum; i++ {
				key := dprefix + strconv.Itoa(i)
				if DSs[key] != nil {
					orderedDSs = append(orderedDSs, DSs[key].Data)
					rows = append(rows, i)
				}
			}
			dsnosplit := strings.Split(dsno, "-")
			pprefix := "p-" + dsnosplit[1] + "-"
			for i := 0; i < client.Filecoder.Rsec.ParityNum; i++ {
				key := pprefix + strconv.Itoa(i)
				if DSs[key] != nil {
					orderedDSs = append(orderedDSs, DSs[key].Data)
					rows = append(rows, client.Filecoder.Rsec.DataNum+i)
				}
			}
			DSsSlice := client.Filecoder.Rsec.Decode(orderedDSs, rows)
			totalDSs := make(map[string]*util.DataShard)
			for i := 0; i < client.Filecoder.Rsec.DataNum; i++ {
				subkey := dsno + "-" + strconv.Itoa(i)
				if DSs[subkey] != nil {
					totalDSs[subkey] = DSs[subkey]
				} else {
					client.Filecoder.MFMMutex.RLock()
					oldV := client.Filecoder.GetVersion(filename, dsno)
					oldT := client.Filecoder.GetTimestamp(filename, dsno)
					client.Filecoder.MFMMutex.RUnlock()
					sig := client.Filecoder.Sigger.GetSig(DSsSlice[i], oldT, oldV)
					totalDSs[subkey] = util.NewDataShard(subkey, DSsSlice[i], sig, oldV, oldT)
				}
			}
			return totalDSs, gdssn_res.Snsds, gdssn_res.Snsps, nil
		}
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

// 客户端向所有存储节点请求获取存储空间代价（单位：字节）
func (client *Client) GetSNsStorageCosts() int {
	totalSize := 0
	var tsMutex sync.RWMutex
	done := make(chan struct{})
	for key, _ := range client.SNRPCs {
		go func(snid string) {
			// 3.1-构造获取存储空间代价请求消息
			gsnsc_req := &pb.GSNSCRequest{ClientId: client.ClientID}
			// 3.2-向存储节点发送请求
			gsnsc_res, err := client.SNRPCs[snid].GetSNStorageCost(context.Background(), gsnsc_req)
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
