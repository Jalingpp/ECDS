package util

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"

	"github.com/Nik-U/pbc"
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
	Params string //用于生成pairing的参数
	G      []byte
	PK     []byte
}

func NewPublicInfo(params string, gb []byte, pubkey []byte) *PublicInfo {
	pi := PublicInfo{params, gb, pubkey}
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

// 将 *pbc.Pairing 对象序列化为字节流
func SerializePairing(pairing *pbc.Pairing) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(pairing)
	if err != nil {
		return nil
	}
	return buf.Bytes()
}

// 将字节流反序列化为 *pbc.Pairing 对象
func DeserializePairing(data []byte) (*pbc.Pairing, error) {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	var pairing pbc.Pairing
	err := dec.Decode(&pairing)
	if err != nil {
		return nil, err
	}
	return &pairing, nil
}
