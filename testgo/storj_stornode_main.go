package main

import (
	storjnodes "ECDS/baselines/storj"
	"ECDS/util"
)

func main() {
	//1-读文件创建所有存储节点
	snaddrfilename := "/root/DSN/ECDS/data/snaddrs"
	storagenodes := make(map[string]*storjnodes.StorjSN) //key:snid
	//读取存储节点地址
	snaddrmap := util.ReadSNAddrFile(snaddrfilename)
	for key, value := range *snaddrmap {
		sn := storjnodes.NewStorjSN(key, value)
		storagenodes[key] = sn
	}
	select {}
}
