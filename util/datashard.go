package util

import (
	"fmt"

	"github.com/Nik-U/pbc"
)

type DataShard struct {
	Data      []int32
	Sig       []byte
	Version   int32
	Timestamp string
}

func NewDataShard(data []int32, sig []byte, v int32, t string) *DataShard {
	ds := DataShard{data, sig, v, t}
	return &ds
}

func (ds *DataShard) Print() {
	fmt.Println("data:", ds.Data)
	fmt.Println("sig:", ds.Sig)
	fmt.Println("version:", ds.Version)
	fmt.Println("timestamp:", ds.Timestamp)
}

type PublicInfo struct {
	Pairing *pbc.Pairing
	G       []byte
	PK      []byte
}

func NewPublicInfo(pairing *pbc.Pairing, gb []byte, pubkey []byte) *PublicInfo {
	pi := PublicInfo{pairing, gb, pubkey}
	return &pi
}
