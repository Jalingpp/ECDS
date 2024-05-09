package main

import (
	"ECDS/nodes"
	"ECDS/util"
	"fmt"
	"strings"
)

func main() {
	//1-读文件创建所有存储节点
	snaddrfilename := "data/snaddr"
	storagenodes := make(map[string]*nodes.StorageNode) //key:snid
	//读取存储节点地址
	reader := util.BufIOReader(snaddrfilename)
	// 逐行读取文件内容
	for {
		// 读取一行数据
		line, err := reader.ReadString('\n')
		if err != nil {
			break // 文件读取结束或者发生错误时退出循环
		}
		// 处理一行数据
		fmt.Print("ReadSNAddr:", line)
		lineslice := strings.Split(strings.TrimRight(line, "\n"), ",") //去除换行符
		sn := nodes.NewStorageNode(lineslice[0], lineslice[1])
		storagenodes[lineslice[0]] = sn
	}
	select {}
}
