package main

import (
	"ECDS/nodes"
	"ECDS/util"
)

func main() {
	dn := 11
	pn := 20
	acAddr := "localhost:50051"
	snAddrFilepath := "data/snaddr"
	//读存储节点地址表
	snaddrmap := util.ReadSNAddrFile(snAddrFilepath)
	//创建一个客户端
	client1 := nodes.NewClient("client1", dn, pn, acAddr, *snaddrmap)
	//客户端PutFile
	filepath := "data/testData2"
	filename := "testData2"
	client1.PutFile(filepath, filename)
}
