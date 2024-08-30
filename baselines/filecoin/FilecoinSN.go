package nodes

import "sync"

type FilecoinSN struct {
	SNId          string               //存储节点id
	SNAddr        string               //存储节点ip地址
	ClientFileMap map[string][]string  //为客户端存储的文件列表，key:clientID,value:filename
	CFMMutex      sync.RWMutex         //ClientFileMap的读写锁
	FileShardsMap map[string][][]int32 //文件的数据分片列表，key:clientID-filename-i,value:分片
}
