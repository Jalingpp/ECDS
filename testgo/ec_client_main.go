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
	// acAddr := "10.0.4.29:50051"
	acAddr := "localhost:50051"
	snAddrFilepath := "/root/DSN/ECDS/data/snaddrs"
	clientNum := 50
	fileNum := 100
	// logoutpath := "data/outlog"
	//读存储节点地址表
	snaddrmap := util.ReadSNAddrFile(snAddrFilepath)
	//创建客户端并完成注册
	clientObject := make(map[string]*nodes.Client)
	for i := 0; i < clientNum; i++ {
		clientId := "client" + strconv.Itoa(i)
		//创建一个客户端
		client1 := nodes.NewClient(clientId, dn, pn, acAddr, *snaddrmap)
		//客户端向存储系统注册
		client1.Register()
		clientObject[clientId] = client1
		log.Println(clientId, "regist complete")
	}

	//PutFile
	start := time.Now()
	done := make(chan struct{})
	for i := 0; i < clientNum; i++ {
		clientId := "client" + strconv.Itoa(i)
		go func(clientId string) {
			//创建一个客户端
			client1 := clientObject[clientId]
			filepath := "/root/DSN/ECDS/data/testData2"
			for j := 0; j < fileNum; j++ {
				//客户端PutFile
				filename := "testData" + strconv.Itoa(j)
				client1.PutFile(filepath, filename)
				log.Println(clientId, "put", filename, "complete")
				// message := clientId + "put" + filename + "completed\n"
				// util.LogToFile(logoutpath, message)
				//客户端GetFile
				// client1.GetFile(filename, false)
				// log.Println(clientId, "get", filename, "complete")
				// message = clientId + "get" + filename + "completed\n"
				// util.LogToFile(logoutpath, message)
				//客户端UpdateDS
				// client1.UpdateDS(filename, "d-1", "12345")
				// log.Println(clientId, "update", filename, "complete")
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
	util.LogToFile("/root/DSN/ECDS/data/outlog", "putfile结束")
	// log.Println("putfile结束")
	duration := time.Since(start)
	util.LogToFile("/root/DSN/ECDS/data/outlog", strconv.Itoa(int(duration.Milliseconds())))
	log.Println("putfile executed in:", duration.Milliseconds())

	//GetFile
	start = time.Now()
	done = make(chan struct{})
	for i := 0; i < clientNum; i++ {
		clientId := "client" + strconv.Itoa(i)
		go func(clientId string) {
			// //创建一个客户端
			client1 := clientObject[clientId]
			for j := 0; j < fileNum; j++ {
				filename := "testData" + strconv.Itoa(j)
				//客户端GetFile
				client1.GetFile(filename, false)
				log.Println(clientId, "get", filename, "complete")
			}
			done <- struct{}{}
		}(clientId)
	}
	// 等待所有协程完成
	for i := 0; i < clientNum; i++ {
		<-done
	}
	util.LogToFile("/root/DSN/ECDS/data/outlog", "getfile结束")
	// log.Println("putfile结束")
	duration = time.Since(start)
	util.LogToFile("/root/DSN/ECDS/data/outlog", strconv.Itoa(int(duration.Milliseconds())))
	log.Println("getfile executed in:", duration.Milliseconds())

	//UpdateFile
	start = time.Now()
	done = make(chan struct{})
	for i := 0; i < clientNum; i++ {
		clientId := "client" + strconv.Itoa(i)
		go func(clientId string) {
			// //创建一个客户端
			client1 := clientObject[clientId]
			for j := 0; j < fileNum; j++ {
				filename := "testData" + strconv.Itoa(j)
				//客户端UpdateDS
				client1.UpdateDS(filename, "d-1", "12345")
				log.Println(clientId, "update", filename, "complete")
			}
			done <- struct{}{}
		}(clientId)
	}
	// 等待所有协程完成
	for i := 0; i < clientNum; i++ {
		<-done
	}
	util.LogToFile("/root/DSN/ECDS/data/outlog", "updatefile结束")
	// log.Println("putfile结束")
	duration = time.Since(start)
	util.LogToFile("/root/DSN/ECDS/data/outlog", strconv.Itoa(int(duration.Milliseconds())))
	log.Println("updatefile executed in:", duration.Milliseconds())
}
