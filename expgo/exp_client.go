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

	//传入参数
	args := os.Args
	dsnMode := args[1]                    //dsn模式
	filedir := args[2]                    //数据集目录
	clientNum, _ := strconv.Atoi(args[3]) //客户端数量
	fileNum, _ := strconv.Atoi(args[4])   //文件数量
	datadir := args[5]
	acAddr, _ := util.ReadOneAddr(datadir + "acaddr")
	snAddrFilepath := datadir + "snaddrs"
	snaddrmap := util.ReadSNAddrFile(snAddrFilepath)

	//创建客户端并完成注册
	ecco, filecoinco, storjco, siaco := CreateClient(dsnMode, clientNum, dn, pn, acAddr, *snaddrmap)

	//创建延迟统计通道
	totalLatency := int64(0)
	totalOriginFileSize := 0
	var tlMutex sync.RWMutex

	//PutFile
	avgFileNum := fileNum / clientNum
	start := time.Now()
	done := make(chan struct{})
	for i := 0; i < clientNum; i++ {
		clientId := "client" + strconv.Itoa(i)
		go func(clientId string, index int) {
			latency, filesize := PutfileByMode(dsnMode, clientId, index, avgFileNum, filedir, ecco, filecoinco, storjco, siaco)
			tlMutex.Lock()
			totalLatency = totalLatency + latency
			totalOriginFileSize = totalOriginFileSize + filesize
			tlMutex.Unlock()
			done <- struct{}{}
		}(clientId, i)
	}
	// 等待所有协程完成
	for i := 0; i < clientNum; i++ {
		<-done
	}
	duration := time.Since(start)
	throughput := strconv.FormatFloat(float64(avgFileNum*clientNum)/duration.Seconds(), 'f', -1, 64)
	avglatency := totalLatency / int64(clientNum)
	// 获取文件在DSN中的存储空间代价
	totalSize := GetSNStorageCost(dsnMode, ecco, filecoinco, storjco, siaco)
	storagecostratio := float64(totalSize) / float64(totalOriginFileSize)
	util.LogToFile(datadir+"outlog_client", "[putfile-w1-"+dsnMode+"-clientNum"+strconv.Itoa(clientNum)+"] throughput="+throughput+", avglatency="+strconv.Itoa(int(avglatency))+" ms, snstoragecost="+strconv.Itoa(totalSize/1024)+" KB, storageratio="+strconv.FormatFloat(storagecostratio, 'f', -1, 64)+"\n")
	log.Println("[putfile-w1-" + dsnMode + "-clientNum" + strconv.Itoa(clientNum) + "] throughput=" + throughput + ", avglatency=" + strconv.Itoa(int(avglatency)) + " ms, snstoragecost=" + strconv.Itoa(totalSize/1024) + " KB, storageratio=" + strconv.FormatFloat(storagecostratio, 'f', -1, 64))

	//getFile
	totalLatency = int64(0)
	avgFileNum = fileNum / clientNum
	start = time.Now()
	done = make(chan struct{})
	for i := 0; i < clientNum; i++ {
		clientId := "client" + strconv.Itoa(i)
		go func(clientId string, index int) {
			latency := GetfileByMode(dsnMode, clientId, index, avgFileNum, filedir, ecco, filecoinco, storjco, siaco)
			tlMutex.Lock()
			totalLatency = totalLatency + latency
			tlMutex.Unlock()
			done <- struct{}{}
		}(clientId, i)
	}
	// 等待所有协程完成
	for i := 0; i < clientNum; i++ {
		<-done
	}
	duration = time.Since(start)
	throughput = strconv.FormatFloat(float64(avgFileNum*clientNum)/duration.Seconds(), 'f', -1, 64)
	avglatency = totalLatency / int64(clientNum)
	util.LogToFile(datadir+"outlog_client", "[getfile-w1-"+dsnMode+"-clientNum"+strconv.Itoa(clientNum)+"] throughput="+throughput+", avglatency="+strconv.Itoa(int(avglatency))+" ms\n")
	log.Println("[getfile-w1-" + dsnMode + "-clientNum" + strconv.Itoa(clientNum) + "] throughput=" + throughput + ", avglatency=" + strconv.Itoa(int(avglatency)) + " ms")
}

func CountLatency(rets *[]time.Duration, durationChList *[]chan time.Duration, done *chan bool) {
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
	*done <- true
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

func PutfileByMode(dsnMode string, clientId string, i int, avgFileNum int, filedir string, ecco map[string]*nodes.Client, filecoinco map[string]*baselines.FilecoinClient, storjco map[string]*storjnodes.StorjClient, siaco map[string]*sianodes.SiaClient) (int64, int) {
	count := 0
	totalFileSize := 0
	if dsnMode == "ec" {
		client1 := ecco[clientId]
		totalLatency := int64(0)
		for j := i*avgFileNum + 1; j <= (i+1)*avgFileNum; j++ {
			//客户端PutFile
			filepath := filedir + strconv.Itoa(j)
			filename := strconv.Itoa(j)
			st := time.Now()
			log.Println("before putfile", clientId, filename)
			fs := client1.PutFile(filepath, filename)
			log.Println("after putfile", clientId, filename)
			du := time.Since(st)
			totalFileSize = totalFileSize + fs
			latency := du.Milliseconds()
			totalLatency = totalLatency + latency
			// log.Println(clientId, "put", filename, "complete in", latency, " ms")
			count++
			if count%50 == 0 {
				log.Println(count, clientId, "put", filename, "complete in", latency, " ms")
			}
		}
		return totalLatency / int64(avgFileNum), totalFileSize
	} else if dsnMode == "filecoin" {
		client1 := filecoinco[clientId]
		totalLatency := int64(0)
		for j := i*avgFileNum + 1; j <= (i+1)*avgFileNum; j++ {
			//客户端PutFile
			filepath := filedir + strconv.Itoa(j)
			filename := strconv.Itoa(j)
			st := time.Now()
			fs := client1.FilecoinPutFile(filepath, filename)
			du := time.Since(st)
			totalFileSize = totalFileSize + fs
			latency := du.Milliseconds()
			totalLatency = totalLatency + latency
			// log.Println(clientId, "put", filename, "complete in", latency, " ms")
			count++
			if count%50 == 0 {
				log.Println(count, clientId, "put", filename, "complete in", latency, " ms")
			}
		}
		return totalLatency / int64(avgFileNum), totalFileSize
	} else if dsnMode == "storj" {
		client1 := storjco[clientId]
		totalLatency := int64(0)
		for j := i*avgFileNum + 1; j <= (i+1)*avgFileNum; j++ {
			//客户端PutFile
			filepath := filedir + strconv.Itoa(j)
			filename := strconv.Itoa(j)
			st := time.Now()
			fs := client1.StorjPutFile(filepath, filename)
			totalFileSize = totalFileSize + fs
			du := time.Since(st)
			latency := du.Milliseconds()
			totalLatency = totalLatency + latency
			// log.Println(clientId, "put", filename, "complete in", latency, " ms")
			count++
			if count%50 == 0 {
				log.Println(count, clientId, "put", filename, "complete in", latency, " ms")
			}
		}
		return totalLatency / int64(avgFileNum), totalFileSize
	} else if dsnMode == "sia" {
		client1 := siaco[clientId]
		totalLatency := int64(0)
		for j := i*avgFileNum + 1; j <= (i+1)*avgFileNum; j++ {
			//客户端PutFile
			filepath := filedir + strconv.Itoa(j)
			filename := strconv.Itoa(j)
			st := time.Now()
			fs := client1.SiaPutFile(filepath, filename)
			du := time.Since(st)
			totalFileSize = totalFileSize + fs
			latency := du.Milliseconds()
			totalLatency = totalLatency + latency
			// log.Println(clientId, "put", filename, "complete in", latency, " ms")
			count++
			if count%50 == 0 {
				log.Println(count, clientId, "put", filename, "complete in", latency, " ms")
			}
		}
		return totalLatency / int64(avgFileNum), totalFileSize
	} else {
		log.Fatalln("dsnMode error")
		return 0, 0
	}
}

func GetSNStorageCost(dsnMode string, ecco map[string]*nodes.Client, filecoinco map[string]*baselines.FilecoinClient, storjco map[string]*storjnodes.StorjClient, siaco map[string]*sianodes.SiaClient) int {
	totalSize := 0
	var tsMutex sync.RWMutex
	if dsnMode == "ec" {
		for _, co := range ecco {
			size := co.GetSNsStorageCosts()
			tsMutex.Lock()
			totalSize = totalSize + size
			tsMutex.Unlock()
		}
		return totalSize
	} else if dsnMode == "filecoin" {
		for _, co := range filecoinco {
			size := co.FilecoinGetSNsStorageCosts()
			tsMutex.Lock()
			totalSize = totalSize + size
			tsMutex.Unlock()
		}
		return totalSize
	} else if dsnMode == "storj" {
		for _, co := range storjco {
			size := co.StorjGetSNsStorageCosts()
			tsMutex.Lock()
			totalSize = totalSize + size
			tsMutex.Unlock()
		}
		return totalSize
	} else if dsnMode == "sia" {
		for _, co := range siaco {
			size := co.SiaGetSNsStorageCosts()
			tsMutex.Lock()
			totalSize = totalSize + size
			tsMutex.Unlock()
		}
		return totalSize
	} else {
		log.Fatalln("dsnMode error")
		return totalSize
	}
}

func GetfileByMode(dsnMode string, clientId string, i int, avgFileNum int, filedir string, ecco map[string]*nodes.Client, filecoinco map[string]*baselines.FilecoinClient, storjco map[string]*storjnodes.StorjClient, siaco map[string]*sianodes.SiaClient) int64 {
	count := 0
	if dsnMode == "ec" {
		client1 := ecco[clientId]
		totalLatency := int64(0)
		for j := i*avgFileNum + 1; j <= (i+1)*avgFileNum; j++ {
			filename := strconv.Itoa(j)
			st := time.Now()
			filestr := client1.GetFile(filename, true)
			du := time.Since(st)
			latency := du.Milliseconds()
			totalLatency = totalLatency + latency
			count++
			if count%50 == 0 {
				log.Println(count, clientId, "get", filename, "complete in", latency, " ms:", filestr)
			}
		}
		return totalLatency / int64(avgFileNum)
	} else if dsnMode == "filecoin" {
		client1 := filecoinco[clientId]
		totalLatency := int64(0)
		for j := i*avgFileNum + 1; j <= (i+1)*avgFileNum; j++ {
			//客户端PutFile
			filename := strconv.Itoa(j)
			st := time.Now()
			filestr, fileversion := client1.FilecoinGetFile(filename)
			du := time.Since(st)
			latency := du.Milliseconds()
			totalLatency = totalLatency + latency
			count++
			if count%50 == 0 {
				log.Println(count, clientId, "get", filename, "complete in", latency, " ms:", fileversion, filestr)
			}
		}
		return totalLatency / int64(avgFileNum)
	} else if dsnMode == "storj" {
		client1 := storjco[clientId]
		totalLatency := int64(0)
		for j := i*avgFileNum + 1; j <= (i+1)*avgFileNum; j++ {
			filename := strconv.Itoa(j)
			st := time.Now()
			filestr, fileversion := client1.StorjGetFile(filename)
			du := time.Since(st)
			latency := du.Milliseconds()
			totalLatency = totalLatency + latency
			count++
			if count%50 == 0 {
				log.Println(count, clientId, "get", filename, "complete in", latency, " ms:", fileversion, filestr)
			}
		}
		return totalLatency / int64(avgFileNum)
	} else if dsnMode == "sia" {
		client1 := siaco[clientId]
		totalLatency := int64(0)
		for j := i*avgFileNum + 1; j <= (i+1)*avgFileNum; j++ {
			filename := strconv.Itoa(j)
			st := time.Now()
			filestr := client1.SiaGetFile(filename, true)
			du := time.Since(st)
			latency := du.Milliseconds()
			totalLatency = totalLatency + latency
			count++
			if count%50 == 0 {
				log.Println(count, clientId, "put", filename, "complete in", latency, " ms:", filestr)
			}
		}
		return totalLatency / int64(avgFileNum)
	} else {
		log.Fatalln("dsnMode error")
		return 0
	}
}
