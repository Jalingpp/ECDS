package main

import (
	baselines "ECDS/baselines/filecoin"
	"ECDS/util"
)

func main() {
	//1-读文件创建所有存储节点
	snaddrfilename := "data/snaddr"
	storagenodes := make(map[string]*baselines.FilecoinSN) //key:snid
	//读取存储节点地址
	snaddrmap := util.ReadSNAddrFile(snaddrfilename)
	for key, value := range *snaddrmap {
		sn := baselines.NewFilecoinSN(key, value)
		storagenodes[key] = sn
	}
	select {}
}
