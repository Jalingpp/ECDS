package main

import (
	"ECDS/nodes"
	"ECDS/util"
	"log"
	"strconv"
	"time"
)

func main() {
	dn := 11
	pn := 20
	acAddr := "localhost:50051"
	snAddrFilepath := "data/snaddr"
	clientNum := 5
	fileNum := 100
	// logoutpath := "data/outlog"
	//读存储节点地址表
	snaddrmap := util.ReadSNAddrFile(snAddrFilepath)
	start := time.Now()
	done := make(chan struct{})
	for i := 0; i < clientNum; i++ {
		clientId := "client" + strconv.Itoa(i)
		go func(clientId string) {
			//创建一个客户端
			client1 := nodes.NewClient(clientId, dn, pn, acAddr, *snaddrmap)
			//客户端向存储系统注册
			client1.Register()
			log.Println(clientId, "regist complete")
			filepath := "data/testData2"
			for j := 0; j < fileNum; j++ {
				//客户端PutFile
				filename := "testData" + strconv.Itoa(j)
				client1.PutFile(filepath, filename)
				log.Println(clientId, "put", filename, "complete")
				// message := clientId + "put" + filename + "completed\n"
				// util.LogToFile(logoutpath, message)
				//客户端GetFile
				client1.GetFile(filename, false)
				log.Println(clientId, "get", filename, "complete")
				// message = clientId + "get" + filename + "completed\n"
				// util.LogToFile(logoutpath, message)
				//客户端UpdateDS
				client1.UpdateDS(filename, "d-1", "12345")
				log.Println(clientId, "update", filename, "complete")
				// message = clientId + "update" + filename + "completed\n"
				// util.LogToFile(logoutpath, message)
			}
			done <- struct{}{}
		}(clientId)
	}
	// 等待所有协程完成
	for i := 0; i < clientNum; i++ {
		<-done
	}
	log.Println("结束")
	duration := time.Since(start)
	log.Println("Program executed in:", duration.Milliseconds())
}
