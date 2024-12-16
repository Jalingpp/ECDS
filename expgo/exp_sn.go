package main

import (
	baselines "ECDS/baselines/filecoin"
	sianodes "ECDS/baselines/sia"
	storjnodes "ECDS/baselines/storj"
	"ECDS/nodes"
	"ECDS/util"
	"fmt"
	"log"
	"os"
)

func main() {
	//固定参数

	//传入参数
	args := os.Args
	dsnMode := args[1] //dsn模式
	datadir := args[2]
	snAddrFilepath := datadir

	ecsns, filecoinsns, storjsns, siasns := CreateSNByMode(dsnMode, snAddrFilepath)
	if dsnMode == "ec" {
		fmt.Println("已创建", len(ecsns), "个ECSN")
	} else if dsnMode == "filecoin" {
		fmt.Println("已创建", len(filecoinsns), "个FilecoinSN")
	} else if dsnMode == "storj" {
		fmt.Println("已创建", len(storjsns), "个StorjSN")
	} else if dsnMode == "sia" {
		fmt.Println("已创建", len(siasns), "个SiaSN")
	} else {
		log.Fatalln("dsnMode error")
	}
	select {}
}

func CreateSNByMode(dsnMode string, snaddrfn string) (map[string]*nodes.StorageNode, map[string]*baselines.FilecoinSN, map[string]*storjnodes.StorjSN, map[string]*sianodes.SiaSN) {
	if dsnMode == "ec" {
		storagenodes := make(map[string]*nodes.StorageNode) //key:snid
		//读取存储节点地址
		snaddrmap := util.ReadSNAddrFile(snaddrfn)
		for key, value := range *snaddrmap {
			sn := nodes.NewStorageNode(key, value)
			storagenodes[key] = sn
		}
		return storagenodes, nil, nil, nil
	} else if dsnMode == "filecoin" {
		storagenodes := make(map[string]*baselines.FilecoinSN) //key:snid
		//读取存储节点地址
		snaddrmap := util.ReadSNAddrFile(snaddrfn)
		for key, value := range *snaddrmap {
			sn := baselines.NewFilecoinSN(key, value)
			storagenodes[key] = sn
		}
		return nil, storagenodes, nil, nil
	} else if dsnMode == "storj" {
		storagenodes := make(map[string]*storjnodes.StorjSN) //key:snid
		//读取存储节点地址
		snaddrmap := util.ReadSNAddrFile(snaddrfn)
		for key, value := range *snaddrmap {
			sn := storjnodes.NewStorjSN(key, value)
			storagenodes[key] = sn
		}
		return nil, nil, storagenodes, nil
	} else if dsnMode == "sia" {
		storagenodes := make(map[string]*sianodes.SiaSN) //key:snid
		//读取存储节点地址
		snaddrmap := util.ReadSNAddrFile(snaddrfn)
		for key, value := range *snaddrmap {
			sn := sianodes.NewSiaSN(key, value)
			storagenodes[key] = sn
		}
		return nil, nil, nil, storagenodes
	} else {
		log.Fatalln("dsnMode error")
		return nil, nil, nil, nil
	}
}
