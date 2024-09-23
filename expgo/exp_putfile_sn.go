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
	"strconv"
	"time"
)

func main() {
	snAddrFilepath := "data/snaddrs"

	//传入参数
	args := os.Args
	dsnMode := args[1]                    //dsn模式
	clientNum, _ := strconv.Atoi(args[2]) //客户端数量

	util.LogToFile("data/outlog_sn", "[putfile-w1-"+dsnMode+"-clientNum"+strconv.Itoa(clientNum)+"]")
	fmt.Println("[putfile-w1-" + dsnMode + "-clientNum" + strconv.Itoa(clientNum) + "]")
	ecsns, filecoinsns, storjsns, siasns := CreateSNByMode(dsnMode, snAddrFilepath)
	for {
		if dsnMode == "ec" {
			// fmt.Println("已创建", len(ecsns), "个ECSN")
			//统计所占存储空间大小
			SizeList := make(map[string]int, len(ecsns))
			for key, value := range ecsns {
				fdsmap := value.FileShardsMap
				for _, dslist := range fdsmap {
					for _, ds := range dslist {
						SizeList[key] = SizeList[key] + ds.Sizeof()
					}
				}
			}
			totalSize := 0
			for _, size := range SizeList {
				totalSize = totalSize + size
			}
			util.LogToFile("data/outlog_sn", "total size="+strconv.Itoa(totalSize))
		} else if dsnMode == "filecoin" {
			// fmt.Println("已创建", len(filecoinsns), "个FilecoinSN")
			//统计所占存储空间大小
			SizeList := make(map[string]int, len(filecoinsns))
			for key, value := range filecoinsns {
				fdsmap := value.ClientFileRepMap
				for _, ds := range fdsmap {
					SizeList[key] = SizeList[key] + len([]byte(ds))
				}
			}
			totalSize := 0
			for _, size := range SizeList {
				totalSize = totalSize + size
			}
			util.LogToFile("data/outlog_sn", "total size="+strconv.Itoa(totalSize))
		} else if dsnMode == "storj" {
			// fmt.Println("已创建", len(storjsns), "个StorjSN")
			SizeList := make(map[string]int, len(storjsns))
			for key, value := range storjsns {
				fdsmap := value.FileShardsMap
				for _, dss := range fdsmap {
					for i := 0; i < len(dss); i++ {
						SizeList[key] = SizeList[key] + len([]byte(util.Int32SliceToStr(dss[i])))
					}
				}
			}
			totalSize := 0
			for _, size := range SizeList {
				totalSize = totalSize + size
			}
			util.LogToFile("data/outlog_sn", "total size="+strconv.Itoa(totalSize))
		} else if dsnMode == "sia" {
			fmt.Println("已创建", len(siasns), "个SiaSN")
		} else {
			// log.Fatalln("dsnMode error")
			//统计所占存储空间大小
			SizeList := make(map[string]int, len(siasns))
			for key, value := range siasns {
				fdsmap := value.FileShardsMap
				for _, ds := range fdsmap {
					SizeList[key] = SizeList[key] + len([]byte(util.Int32SliceToStr(ds)))
				}
			}
			totalSize := 0
			for _, size := range SizeList {
				totalSize = totalSize + size
			}
			util.LogToFile("data/outlog_sn", "total size="+strconv.Itoa(totalSize))
		}
		time.Sleep(time.Duration(20) * time.Second)
	}
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
