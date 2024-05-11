package util

import (
	"encoding/json"
	"fmt"
)

type DataShard struct {
	DSno      string
	Data      []int32
	Sig       []byte
	Version   int32
	Timestamp string
}

func NewDataShard(dsno string, data []int32, sig []byte, v int32, t string) *DataShard {
	ds := DataShard{dsno, data, sig, v, t}
	return &ds
}

func (ds *DataShard) Print() {
	fmt.Println("dsno:", ds.DSno)
	fmt.Println("data:", ds.Data)
	fmt.Println("sig:", ds.Sig)
	fmt.Println("version:", ds.Version)
	fmt.Println("timestamp:", ds.Timestamp)
}

type PublicInfo struct {
	G  []byte
	PK []byte
}

func NewPublicInfo(gb []byte, pubkey []byte) *PublicInfo {
	pi := PublicInfo{gb, pubkey}
	return &pi
}

// 序列化数据分片
func (ds *DataShard) SerializeDS() []byte {
	jsonDS, err := json.Marshal(ds)
	if err != nil {
		fmt.Printf("SerializeDS error: %v\n", err)
		return nil
	}
	return jsonDS
}

// 反序列化数据分片
func DeserializeDS(data []byte) (*DataShard, error) {
	var ds DataShard
	if err := json.Unmarshal(data, &ds); err != nil {
		fmt.Printf("DeserializeDS error: %v\n", err)
		return nil, err
	}
	return &ds, nil
}
