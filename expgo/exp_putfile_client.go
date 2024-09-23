package main

import (
	baselines "ECDS/baselines/filecoin"
	sianodes "ECDS/baselines/sia"
	storjnodes "ECDS/baselines/storj"
	"ECDS/nodes"
	"ECDS/util"
	"log"
	"os"
	"strconv"
	"sync"
	"time"
)

func main() {
	//固定参数
	dn := 11
	pn := 20
	acAddr, _ := util.ReadOneAddr("data/acaddr")
	snAddrFilepath := "data/snaddrs"
	snaddrmap := util.ReadSNAddrFile(snAddrFilepath)

	//传入参数
	args := os.Args
	dsnMode := args[1]                    //dsn模式
	filedir := args[2]                    //数据集目录
	clientNum, _ := strconv.Atoi(args[3]) //客户端数量
	fileNum, _ := strconv.Atoi(args[4])   //文件数量

	//创建客户端并完成注册
	ecco, filecoinco, storjco, siaco := CreateClient(dsnMode, clientNum, dn, pn, acAddr, *snaddrmap)

	//创建延迟统计通道
	var latencyDuration time.Duration = 0
	latencyDurationChList := make([]chan time.Duration, clientNum)
	for i := 0; i < clientNum; i++ {
		latencyDurationChList[i] = make(chan time.Duration)
	}
	latencyDurationList := make([]time.Duration, clientNum)
	doneCh := make(chan bool)
	go CountLatency(&latencyDurationList, &latencyDurationChList, doneCh)

	//PutFile
	avgFileNum := fileNum / clientNum
	start := time.Now()
	done := make(chan struct{})
	for i := 0; i < clientNum; i++ {
		clientId := "client" + strconv.Itoa(i)
		go func(clientId string, index int) {
			//创建一个客户端
			PutfileByMode(dsnMode, clientId, index, avgFileNum, filedir, latencyDurationChList[i], ecco, filecoinco, storjco, siaco)
			done <- struct{}{}
		}(clientId, i)
	}
	// 等待所有协程完成
	for i := 0; i < clientNum; i++ {
		<-done
	}
	duration := time.Since(start)
	throughput := strconv.FormatFloat(float64(avgFileNum*clientNum)/duration.Seconds(), 'f', -1, 64)
	//计算
	<-doneCh
	for _, du := range latencyDurationList {
		latencyDuration += du
	}
	avglatency := strconv.FormatFloat(float64(latencyDuration.Milliseconds())/float64(avgFileNum*clientNum), 'f', -1, 64)

	util.LogToFile("data/outlog_client", "[putfile-w1-"+dsnMode+"-clientNum"+strconv.Itoa(clientNum)+"] throughput="+throughput+", avglatency="+avglatency+" ms")
	log.Println("[putfile-w1-" + dsnMode + "-clientNum" + strconv.Itoa(clientNum) + "] throughput=" + throughput + ", avglatency=" + avglatency + " ms")

}

func CountLatency(rets *[]time.Duration, durationChList *[]chan time.Duration, done chan bool) {
	wG := sync.WaitGroup{}
	wG.Add(len(*rets))
	for i := 0; i < len(*rets); i++ {
		idx := i

		go func() {
			ch := (*durationChList)[idx]
			for du := range ch {
				(*rets)[idx] += du
			}
			wG.Done()
		}()
	}
	wG.Wait()
	done <- true
}

func CreateClient(dsnMode string, clientNum int, dn int, pn int, acAddr string, snaddrmap map[string]string) (map[string]*nodes.Client, map[string]*baselines.FilecoinClient, map[string]*storjnodes.StorjClient, map[string]*sianodes.SiaClient) {
	if dsnMode == "ec" {
		clientObject := make(map[string]*nodes.Client)
		for i := 0; i < clientNum; i++ {
			clientId := "client" + strconv.Itoa(i)
			//创建一个客户端
			client1 := nodes.NewClient(clientId, dn, pn, acAddr, snaddrmap)
			//客户端向存储系统注册
			client1.Register()
			clientObject[clientId] = client1
			log.Println(clientId, "regist complete")
		}
		return clientObject, nil, nil, nil
	} else if dsnMode == "filecoin" {
		clientObject := make(map[string]*baselines.FilecoinClient)
		for i := 0; i < clientNum; i++ {
			clientId := "client" + strconv.Itoa(i)
			//创建一个客户端
			client1 := baselines.NewFilecoinClient(clientId, acAddr, snaddrmap)
			clientObject[clientId] = client1
			log.Println(clientId, "创建完成。")
		}
		return nil, clientObject, nil, nil
	} else if dsnMode == "storj" {
		clientObject := make(map[string]*storjnodes.StorjClient)
		for i := 0; i < clientNum; i++ {
			clientId := "client" + strconv.Itoa(i)
			//创建一个客户端
			client1 := storjnodes.NewStorjClient(clientId, dn, pn, acAddr, snaddrmap)
			clientObject[clientId] = client1
			log.Println(clientId, "创建完成。")
		}
		return nil, nil, clientObject, nil
	} else if dsnMode == "sia" {
		clientObject := make(map[string]*sianodes.SiaClient)
		for i := 0; i < clientNum; i++ {
			clientId := "client" + strconv.Itoa(i)
			//创建一个客户端
			client1 := sianodes.NewSiaClient(clientId, dn, pn, acAddr, snaddrmap)
			clientObject[clientId] = client1
			log.Println(clientId, "创建完成。")
		}
		return nil, nil, nil, clientObject
	} else {
		log.Fatalln("dsnMode error")
		return nil, nil, nil, nil
	}
}

func PutfileByMode(dsnMode string, clientId string, i int, avgFileNum int, filedir string, latencyDurationCh chan time.Duration, ecco map[string]*nodes.Client, filecoinco map[string]*baselines.FilecoinClient, storjco map[string]*storjnodes.StorjClient, siaco map[string]*sianodes.SiaClient) {
	if dsnMode == "ec" {
		client1 := ecco[clientId]
		for j := i*avgFileNum + 1; j <= (i+1)*avgFileNum; j++ {
			//客户端PutFile
			filepath := filedir + strconv.Itoa(j)
			filename := strconv.Itoa(j)
			st := time.Now()
			client1.PutFile(filepath, filename)
			du := time.Since(st)
			latencyDurationCh <- du //计latency入ch
			log.Println(clientId, "put", filename, "complete")
		}
	}

}
