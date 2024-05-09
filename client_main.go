package main

import (
	"ECDS/nodes"
)

func main() {
	dn := 11
	pn := 20
	acAddr := "localhost:50051"
	//创建一个客户端
	client1 := nodes.NewClient("client1", dn, pn, acAddr)
	//客户端PutFile
	filepath := ""
	filename := "file1"
	client1.PutFile(filepath, filename)
}
