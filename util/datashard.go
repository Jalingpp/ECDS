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

func (ds *DataShard) Sizeof() int {
	return len([]byte(Int32SliceToStr(ds.Data)))
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

type Meta4File struct {
	//用于记录一个文件所有分片的信息
	LatestVersionSlice   map[string]int32  //key:dsno
	LatestTimestampSlice map[string]string //key:dsno
}

func (m4f *Meta4File) Sizeof() int {
	dsnosize := 0
	for key, _ := range m4f.LatestVersionSlice {
		dsnosize = len([]byte(key))
		if dsnosize > 0 {
			break
		}
	}
	return len(m4f.LatestTimestampSlice) * (2*dsnosize + 8 + 19)
}

// 【客户端执行】创建文件元数据记录器
func NewMeta4File() *Meta4File {
	lvs := make(map[string]int32)
	lts := make(map[string]string)
	m4f := &Meta4File{lvs, lts}
	return m4f
}

// 复制一个元数据记录器
func CopyMeta4File(m4f *Meta4File) *Meta4File {
	nm4f := NewMeta4File()
	for dsno, _ := range m4f.LatestVersionSlice {
		nm4f.LatestVersionSlice[dsno] = m4f.LatestVersionSlice[dsno]
		nm4f.LatestTimestampSlice[dsno] = m4f.LatestTimestampSlice[dsno]
	}
	return nm4f
}

// 【客户端执行】更新一个文件的一个分片的元数据
func (m4f *Meta4File) Update(dsno string, newT string, newV int32) {
	m4f.LatestTimestampSlice[dsno] = newT
	m4f.LatestVersionSlice[dsno] = newV
}
