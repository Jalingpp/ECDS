package main

import (
	"ECDS/encode"
	"ECDS/pdp"
	"ECDS/util"
	"fmt"
	"math/rand"
	"time"
)

// func main() {
// 	TestIncECPDP()
// }

type StorageShard struct {
	data    []byte
	sig     []byte
	version int32
	time    string
}

func (ss *StorageShard) Print() {
	fmt.Println("data:", string(ss.data))
	fmt.Println("sig:", string(ss.sig))
	fmt.Println("version:", ss.version)
	fmt.Println("time:", ss.time)
}

func TestIncECPDP() {
	//读文件
	filepath := "data/testData"
	fileStr := util.ReadStringFromFile(filepath)
	fileName := "testData"
	//客户端创建文件编码器
	dataNum := 10
	parityNum := 4
	fileCoder := encode.NewFileCoder(dataNum, parityNum)
	//客户端Setup
	dataShards, publicInfo := fileCoder.Setup(fileName, fileStr)
	fmt.Println("【客户端Setup】")
	for i := 0; i < len(dataShards); i++ {
		dataShards[i].Print()
	}
	//存储节点验签
	fmt.Println("【存储节点验签】")
	for i := 0; i < len(dataShards); i++ {
		ds := dataShards[i]
		fmt.Println("Verify datashard-", i)
		pdp.VerifySig(publicInfo.Pairing, publicInfo.G, publicInfo.PK, ds.Data, ds.Sig, ds.Version, ds.Timestamp)
	}

	//客户端发起挑战
	randSource := rand.NewSource(time.Now().UnixNano())
	r := rand.New(randSource) //设置随机种子

	//生成并验证存储证明
	for i := 0; i < len(dataShards); i++ {
		randS := r.Int31()
		//存储节点生成存储证明
		fmt.Println("【存储节点生成存储证明】")
		pos := pdp.ProvePos(publicInfo, &dataShards[i], randS)
		pos.Print()
		//客户端验证存储证明
		fmt.Println("【存储节点验证存储证明】")
		pdp.VerifyPos(pos, fileCoder.Sigger.Pairing, fileCoder.Sigger.G, fileCoder.Sigger.PubKey, fileCoder.MetaFileMap[fileName].LatestVersionSlice[i], fileCoder.MetaFileMap[fileName].LatestTimestampSlice[i], randS)
	}

	//客户端修改数据块并生成校验块增量分片
	filepath2 := "data/testChangeData"
	newDataStr := util.ReadStringFromFile(filepath2)
	udpDataRow := 2
	fmt.Println("【客户端修改数据块】")
	fmt.Println("newData[", udpDataRow, "]:", util.ByteSliceToInt32Slice([]byte(newDataStr)))
	incParityShards := fileCoder.UpdateDataShard(fileName, udpDataRow, &dataShards[udpDataRow], newDataStr)

	//存储节点对校验块增量分片验签并更新校验块
	fmt.Println("【存储节点对增量分块验签】")
	for i := 0; i < len(incParityShards); i++ {
		ips := incParityShards[i]
		pdp.VerifySig(publicInfo.Pairing, publicInfo.G, publicInfo.PK, ips.Data, ips.Sig, ips.Version, ips.Timestamp)
	}

	//存储节点更新校验块分片
	fmt.Println("【存储节点更新校验块分片】")
	for i := 0; i < len(incParityShards); i++ {
		ips := incParityShards[i]
		encode.UpdateParityShard(&dataShards[dataNum+i], &ips, publicInfo)
		dataShards[dataNum+i].Print()
	}

	//客户端再次挑战，存储节点生成存储证明，客户端验证存储证明
	for i := 0; i < len(dataShards); i++ {
		randS := r.Int31()
		//存储节点生成存储证明
		fmt.Println("【存储节点生成存储证明2】")
		pos := pdp.ProvePos(publicInfo, &dataShards[i], randS)
		pos.Print()
		//客户端验证存储证明
		fmt.Println("【存储节点验证存储证明2】")
		pdp.VerifyPos(pos, fileCoder.Sigger.Pairing, fileCoder.Sigger.G, fileCoder.Sigger.PubKey, fileCoder.MetaFileMap[fileName].LatestVersionSlice[i], fileCoder.MetaFileMap[fileName].LatestTimestampSlice[i], randS)
	}

	//客户端请求数据
	fmt.Println("【客户端请求数据】")
	shardRows := []int{0, 1, 3, 4, 5, 6, 7, 8, 9, 10}

	//服务器返回数据分片,客户端组成新的数据
	fmt.Println("【客户端收到数据分片】")
	restDataSlice := make([][]int32, len(shardRows))
	for i := 0; i < len(shardRows); i++ {
		restDataSlice[i] = make([]int32, len(dataShards[shardRows[0]].Data))
		copy(restDataSlice[i], dataShards[shardRows[i]].Data)
		fmt.Println("data[", shardRows[i], "]:", restDataSlice[i])
	}

	//客户端解码得到全量数据文件
	fmt.Println("【客户端解码得到全量数据文件】")
	fileInt32Slices := fileCoder.Rsec.Decode(restDataSlice, shardRows)
	fileDecodedStr := ""
	for i := 0; i < len(fileInt32Slices); i++ {
		fileDecodedStr = fileDecodedStr + util.Int32SliceToStr(fileInt32Slices[i])
	}
	fmt.Println("解码后的数据为：", fileDecodedStr)
	// util.PrintInt32Slices(fileInt32Slices)

	//输出sig的大小
	fmt.Println("Sig Size:", len(dataShards[0].Sig), "bytes.")

}
