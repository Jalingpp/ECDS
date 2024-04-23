package main

import (
	"ECDS/encode"
	"ECDS/pdp"
	"ECDS/util"
	"fmt"
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
	file_str := util.ReadStringFromFile(filepath)
	// fmt.Println("file_str:",file_str)

	//划分文件
	file_datas := util.SplitString(file_str, 10)
	// for i:=0;i<len(file_datas);i++ {
	// 	fmt.Println(i,":",file_datas[i])
	// }

	//纠删码编码(10,4)
	data_num := 10
	parity_num := 4
	rsec := encode.FileCoder{}
	rsec.Setup(data_num, parity_num)
	ecdata_slice := make([][]byte, data_num+parity_num)
	for i := 0; i < data_num+parity_num; i++ {
		if i < data_num {
			ecdata_slice[i] = []byte(file_datas[i])
		} else {
			ecdata_slice[i] = make([]byte, len(ecdata_slice[0]))
		}
	}
	rsec.Encode(ecdata_slice)
	// for i := 0; i < len(ecdata_slice); i++ {
	// 	fmt.Println("打印编码生成的冗余块：")
	// 	fmt.Println(i, string(ecdata_slice[i]))
	// }

	//签名
	sigger := pdp.Signature{}
	sigger.Setup()
	storageShards := make([]StorageShard, 0)
	for i := 0; i < len(ecdata_slice); i++ {
		storageShard := StorageShard{}
		storageShard.data = make([]byte, len(ecdata_slice[i]))
		copy(storageShard.data, ecdata_slice[i])
		// storageShard.data = ecdata_slice[i]
		t := time.Now().Format("2006-01-02 15:04:05")
		sig := sigger.GetSig2(string(ecdata_slice[i]), t, int32(1))
		storageShard.sig = sig
		storageShard.version = 1
		storageShard.time = t
		storageShards = append(storageShards, storageShard)
	}
	// for i := 0; i < len(storageShards); i++ {
	// 	fmt.Println("第", i, "个数据块信息：")
	// 	storageShards[i].Print()
	// }

	//生成并验证存储证明
	// randSource := rand.NewSource(time.Now().UnixNano())
	// r := rand.New(randSource) //设置随机种子
	// for i := 0; i < len(storageShards); i++ {
	// 	randS := r.Int31()
	// 	pos := pdp.ProvePos(sigger.Pairing, sigger.G, sigger.PubKey, string(storageShards[i].data), storageShards[i].sig, randS)
	// 	pdp.VerifyPos(pos, sigger.Pairing, sigger.G, sigger.PubKey, storageShards[i].version, storageShards[i].time, randS)
	// }

	//修改数据块
	filepath2 := "data/testChangeData"
	newData := util.ReadStringFromFile(filepath2)
	oldData := make([]byte, len(newData)) //保留旧数据块
	copy(oldData, storageShards[0].data)
	// fmt.Println("oldData:", string(oldData))
	copy(storageShards[0].data, []byte(newData)) //修改第0号数据分片的数据
	storageShards[0].version++
	storageShards[0].time = time.Now().Format("2006-01-02 15:04:05")
	// fmt.Println("sig[0] before change:", string(storageShards[0].sig))
	storageShards[0].sig = sigger.GetSig2(string(storageShards[0].data), storageShards[0].time, storageShards[0].version)
	// fmt.Println("sig[0] after change:", string(storageShards[0].sig))

	//重新计算冗余块
	//方法1：重编码
	// fmt.Println("Before change:")
	// for i := 0; i < len(ecdata_slice); i++ {
	// 	fmt.Println(i, ":", string(ecdata_slice[i]))
	// }
	ecdata_slice[0] = []byte(newData)
	rsec.Encode(ecdata_slice)
	// fmt.Println("After change:")
	// for i := rsec.Data_num; i < len(ecdata_slice); i++ {
	// 	fmt.Println(i, ":", ecdata_slice[i])
	// }
	//方法2：增量更新
	newParitys := make([][]byte, rsec.Parity_num)
	for i := 0; i < rsec.Parity_num; i++ {
		newParitys[i] = make([]byte, len(newData))
	}
	// oldDataSlice := make([][]byte, rsec.Parity_num)   //记录旧的校验块
	// oldVersionSlice := make([]int32, rsec.Parity_num) //记录校验块的旧版本号
	// oldTimeSlice := make([]string, rsec.Parity_num)   //记录校验块的旧时间戳
	incData := encode.GetIncData(oldData, []byte(newData))
	rsec.UpdateParity(incData, 0, newParitys)
	// fmt.Println("incParity:")
	// for i := 0; i < len(newParitys); i++ {
	// 	fmt.Println(i, ":", newParitys[i])
	// }
	// fmt.Println("oldParity:")
	// for i := rsec.Data_num; i < len(storageShards); i++ {
	// 	fmt.Println(i, ":", storageShards[i].data)
	// }
	sigger.Test(storageShards[rsec.Data_num].data, ecdata_slice[rsec.Data_num], newParitys[0])
	// for i := 0; i < rsec.Parity_num; i++ {
	// 	copy(newParitys[i], storageShards[rsec.Data_num+i].data) //旧的校验块值
	// 	rsec.UpdateParity(incData, 0, newParitys)
	// 	// fmt.Println("newParity-", i, ":", newParitys[i])
	// 	oldDataSlice[i] = make([]byte, len(storageShards[rsec.Data_num+i].data))
	// 	copy(oldDataSlice[i], storageShards[rsec.Data_num+i].data) //记录旧的校验块
	// 	// fmt.Println("oldDataSlice-", i, oldDataSlice[i])
	// 	copy(storageShards[rsec.Data_num+i].data, newParitys[i])    //更新旧校验块的值
	// 	oldVersionSlice[i] = storageShards[rsec.Data_num+i].version //记录旧版本号
	// 	storageShards[rsec.Data_num+i].version++
	// 	oldTimeSlice[i] = storageShards[rsec.Data_num+i].time //记录旧时间戳
	// 	storageShards[rsec.Data_num+i].time = time.Now().Format("2006-01-02 15:04:05")
	// 	// fmt.Println("storageShards-", rsec.Data_num+i, ":", storageShards[rsec.Data_num+i].data)
	// 	newParitys[i] = make([]byte, len(newData))
	// }

	//计算新冗余块的签名
	//方法1：重编码
	// newSigSlice := make([][]byte, 0)
	// for i := rsec.Data_num; i < len(storageShards); i++ {
	// 	newSig := sigger.GetSig2(string(storageShards[i].data), storageShards[i].time, storageShards[i].version)
	// 	newSigSlice = append(newSigSlice, newSig)
	// }
	// for i := 0; i < len(newSigSlice); i++ {
	// 	fmt.Println("newSig-", i, ":", newSigSlice[i])
	// }
	//方法2：增量计算
	//测试数据增量计算和重编码的关系
	// matrixCoef := rsec.GetMatrixElement(0)
	// fmt.Println("matrixCoef:", matrixCoef)
	// fmt.Println("incData:", incData)
	// fmt.Println("oldData[0]:", oldDataSlice[0])
	// fmt.Println("newData[0]:", storageShards[rsec.Data_num].data)
	// incParity := util.SubtractBytes(storageShards[rsec.Data_num].data, oldDataSlice[0])
	// fmt.Println("newParity1:", storageShards[rsec.Data_num].data)
	// newParity := util.AddByteSlices(incParity, oldDataSlice[0])
	// fmt.Println("newParity2:", newParity)
	// sigger.TestGCDX(oldDataSlice[0], storageShards[rsec.Data_num].data, incParity)
	// for i := 0; i < rsec.Parity_num; i++ {
	// 	sigger.TestGCADX(oldDataSlice[i], storageShards[rsec.Data_num+i].data, incData, matrixCoef[i])
	// }
	// incSigSlice := make([][]byte, 0)
	// for i := 0; i < rsec.Parity_num; i++ {
	// 	incsig := sigger.GetIncSig(incData, matrixCoef[i], oldVersionSlice[i], oldTimeSlice[i], storageShards[rsec.Data_num+i].version, storageShards[rsec.Data_num+i].time)
	// 	incSigSlice = append(incSigSlice, incsig)
	// }
	// newSigSlice2 := make([][]byte, 0)
	// for i := 0; i < rsec.Parity_num; i++ {
	// 	oldsig := storageShards[rsec.Data_num+i].sig
	// 	newsig := sigger.GetSigByInc(oldsig, incSigSlice[i])
	// 	newSigSlice2 = append(newSigSlice2, newsig)
	// }
	// for i := 0; i < len(newSigSlice2); i++ {
	// 	fmt.Println("newSigByInc-", i, ":", newSigSlice2[i])
	// }

	//存储证明验证

}
