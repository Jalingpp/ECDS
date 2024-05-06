package encode

import (
	"ECDS/pdp"
	"ECDS/util"
	"fmt"
	"time"
)

type FileCoder struct {
	Rsec        *RSEC
	Sigger      *pdp.Signature
	MetaFileMap map[string]*Meta4File // 用于记录每个文件的元数据,key是文件名
}

type Meta4File struct {
	//用于记录一个文件所有分片的信息
	LatestVersionSlice   []int32
	LatestTimestampSlice []string
}

// 【客户端执行】创建文件元数据记录器
func NewMeta4File(n int) *Meta4File {
	lvs := make([]int32, n)
	lts := make([]string, n)
	m4f := &Meta4File{lvs, lts}
	return m4f
}

// 【客户端执行】更新一个文件的一个分片的元数据
func (m4f *Meta4File) Update(n int, newT string, newV int32) {
	m4f.LatestTimestampSlice[n] = newT
	m4f.LatestVersionSlice[n] = newV
}

// 【客户端执行】创建文件编码器
func NewFileCoder(dn int, pn int) *FileCoder {
	fc := FileCoder{}
	//创建编码器
	fc.Rsec = NewRSEC(dn, pn)
	//创建签名器
	fc.Sigger = pdp.NewSig()
	//初始化元数据记录器
	fc.MetaFileMap = make(map[string]*Meta4File)
	return &fc
}

// 【客户端执行】对文件分块，EC编码，签名
func (fc *FileCoder) Setup(filename string, filedata string) ([]util.DataShard, *util.PublicInfo) {
	dsnum := fc.Rsec.DataNum + fc.Rsec.ParityNum
	//创建一个包含数据块和校验块的[]int32切片
	dataSlice := make([][]int32, dsnum)
	//对文件进行分块后放入切片中
	dsStrings := util.SplitString(filedata, fc.Rsec.DataNum)
	for i := 0; i < fc.Rsec.DataNum; i++ {
		dataSlice[i] = util.ByteSliceToInt32Slice([]byte(dsStrings[i]))
	}
	//初始化切片中的校验块
	for i := fc.Rsec.DataNum; i < dsnum; i++ {
		dataSlice[i] = make([]int32, len(dataSlice[0]))
	}
	//RS纠删码编码
	err := fc.Rsec.Encode(dataSlice)
	if err != nil {
		panic(err)
	}
	//创建公共信息对象（发送给存储节点，用于验签、更新等操作）
	publicInfo := util.NewPublicInfo(fc.Sigger.Pairing, fc.Sigger.G, fc.Sigger.PubKey)
	//为文件初始化一个元数据记录器
	meta4file := NewMeta4File(dsnum)
	//对切片中的每个块签名后构建DataShard
	var dataShardSlice []util.DataShard
	for i := 0; i < dsnum; i++ {
		time := time.Now().Format("2006-01-02 15:04:05")
		sig := fc.Sigger.GetSig(dataSlice[i], time, 0)
		datashard := util.NewDataShard(dataSlice[i], sig, 0, time)
		dataShardSlice = append(dataShardSlice, *datashard)
		//将分片信息写入元数据记录器中
		meta4file.Update(i, datashard.Timestamp, datashard.Version)
	}
	//将该文件的元数据记录器写入fc中
	fc.MetaFileMap[filename] = meta4file
	return dataShardSlice, publicInfo
}

// 【客户端执行】（原地）更新数据分片，返回增量校验块：filename是ds所属的文件名，drow是ds数据在稀疏矩阵中对应的行号
func (fc *FileCoder) UpdateDataShard(filename string, drow int, ds *util.DataShard, dataStr string) []util.DataShard {
	newData := util.ByteSliceToInt32Slice([]byte(dataStr))
	oldData := make([]int32, len(newData))
	copy(oldData, ds.Data)
	//更新数据分片
	ds.Data = newData
	ds.Timestamp = time.Now().Format("2006-01-02 15:04:05")
	ds.Version = ds.Version + 1
	ds.Sig = fc.Sigger.GetSig(newData, ds.Timestamp, ds.Version)
	//将数据分片的更新写入到文件元数据记录器中
	fc.MetaFileMap[filename].Update(drow, ds.Timestamp, ds.Version)
	//计算数据增量
	incData := fc.Rsec.GetIncData(oldData, newData)
	//计算校验块增量，校验块签名增量，生成增量校验分片
	var incParityShards []util.DataShard
	for i := fc.Rsec.DataNum; i < fc.Rsec.DataNum+fc.Rsec.ParityNum; i++ {
		//计算校验块增量和矩阵系数
		incParity := fc.Rsec.GetIncParity(incData, i, drow)
		//计算签名增量
		oldV := fc.MetaFileMap[filename].LatestVersionSlice[i]
		oldT := fc.MetaFileMap[filename].LatestTimestampSlice[i]
		newV := oldV + 1
		newT := time.Now().Format("2006-01-02 15:04:05")
		incSig := fc.Sigger.GetIncSig(incParity, oldV, oldT, newV, newT)
		iPS := util.NewDataShard(incParity, incSig, newV, newT)
		fc.MetaFileMap[filename].Update(i, newT, newV)
		incParityShards = append(incParityShards, *iPS)
	}
	return incParityShards
}

// 【存储节点执行】根据校验块增量分片更新校验块分片
func UpdateParityShard(oldParityShard *util.DataShard, incParityShard *util.DataShard, publicInfo *util.PublicInfo) *util.DataShard {
	//更新分片数据
	oldParityShard.Data = UpdateParity(oldParityShard.Data, incParityShard.Data)
	//更新分片签名
	oldParityShard.Sig = pdp.UpdateSigByIncSig(publicInfo.Pairing, oldParityShard.Sig, incParityShard.Sig)
	//更新分片版本
	oldParityShard.Version = incParityShard.Version
	//更新分片时间戳
	oldParityShard.Timestamp = incParityShard.Timestamp
	return oldParityShard
}

func TestFileCoder() {
	dataNum := 5
	parityNum := 3
	//创建文件编码器
	fileCoder := NewFileCoder(dataNum, parityNum)
	//数据文件-字符串型
	filename := "testFile"
	fileStr := "abcdef123456123456123456123456"
	//Setup
	dataShards, publicInfo := fileCoder.Setup(filename, fileStr)
	fmt.Println("【测试Setup】")
	for i := 0; i < len(dataShards); i++ {
		dataShards[i].Print()
	}
	//验签
	fmt.Println("【测试验签】")
	for i := 0; i < len(dataShards); i++ {
		ds := dataShards[i]
		fmt.Println("Verify datashard-", i)
		pdp.VerifySig(publicInfo.Pairing, publicInfo.G, publicInfo.PK, ds.Data, ds.Sig, ds.Version, ds.Timestamp)
	}
	//更新数据块
	newDataStr := "abcdef"
	udpDataRow := 2
	incParityShards := fileCoder.UpdateDataShard(filename, udpDataRow, &dataShards[udpDataRow], newDataStr)
	fmt.Println("【测试更新数据块】")
	fmt.Println("校验块分片增量：")
	for i := 0; i < len(incParityShards); i++ {
		incParityShards[i].Print()
	}
	//更新校验块
	fmt.Println("新的校验块分片：")
	for i := dataNum; i < dataNum+parityNum; i++ {
		newParityShard := UpdateParityShard(&dataShards[i], &incParityShards[i-dataNum], publicInfo)
		newParityShard.Print()
		dataShards[i] = *newParityShard
	}
	fmt.Println("【输出更新后的所有分片】")
	for i := 0; i < dataNum+parityNum; i++ {
		dataShards[i].Print()
	}

	// //测试校验块的验签
	for i := dataNum; i < dataNum+parityNum; i++ {
		ds := dataShards[i]
		fmt.Println("Verify parityshard-", i)
		pdp.VerifySig(publicInfo.Pairing, publicInfo.G, publicInfo.PK, ds.Data, ds.Sig, ds.Version, ds.Timestamp)
	}

}
