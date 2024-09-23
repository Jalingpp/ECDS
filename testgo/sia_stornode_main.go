package main

import (
	sianodes "ECDS/baselines/sia"
	"ECDS/util"
)

func main() {
	//1-读文件创建所有存储节点
	snaddrfilename := "data/snaddr"
	storagenodes := make(map[string]*sianodes.SiaSN) //key:snid
	//读取存储节点地址
	snaddrmap := util.ReadSNAddrFile(snaddrfilename)
	for key, value := range *snaddrmap {
		sn := sianodes.NewSiaSN(key, value)
		storagenodes[key] = sn
	}
	select {}
}
