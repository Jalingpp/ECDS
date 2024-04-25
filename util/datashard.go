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
	Pairing   *pbc.Pairing
	G         []byte
	PK        []byte
}

func NewDataShard(data []int32, sig []byte, v int32, t string, pairing *pbc.Pairing, gb []byte, pubkey []byte) *DataShard {
	ds := DataShard{data, sig, v, t, pairing, gb, pubkey}
	return &ds
}

func (ds *DataShard) Print() {
	fmt.Println("data:", ds.Data)
	fmt.Println("sig:", ds.Sig)
	fmt.Println("version:", ds.Version)
	fmt.Println("timestamp:", ds.Timestamp)
}
