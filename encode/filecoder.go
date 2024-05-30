package encode

import (
	"ECDS/pdp"
	"ECDS/util"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Nik-U/pbc"
)

type FileCoder struct {
	Rsec        *RSEC
	Sigger      *pdp.Signature
	MetaFileMap map[string]*util.Meta4File // 用于记录每个文件的元数据,key是文件名
	MFMMutex    sync.RWMutex               //MetaFileMap的读写锁
}

// 获取文件所有分片版本号
func (fc *FileCoder) GetVersions(filename string) map[string]int32 {
	fc.MFMMutex.RLock()
	v := fc.MetaFileMap[filename].LatestVersionSlice
	fc.MFMMutex.RUnlock()
	return v
}

// 获取文件所有分片时间戳
func (fc *FileCoder) GetTimestamps(filename string) map[string]string {
	fc.MFMMutex.RLock()
	t := fc.MetaFileMap[filename].LatestTimestampSlice
	fc.MFMMutex.RUnlock()
	return t
}

// 获取分片版本号
func (fc *FileCoder) GetVersion(filename string, dsno string) int32 {
	fc.MFMMutex.RLock()
	v := fc.MetaFileMap[filename].LatestVersionSlice[dsno]
	fc.MFMMutex.RUnlock()
	return v
}

// 获取分片时间戳
func (fc *FileCoder) GetTimestamp(filename string, dsno string) string {
	fc.MFMMutex.RLock()
	t := fc.MetaFileMap[filename].LatestTimestampSlice[dsno]
	fc.MFMMutex.RUnlock()
	return t
}

// 【客户端执行】创建文件编码器
func NewFileCoder(dn int, pn int, params string, g []byte) *FileCoder {
	fc := FileCoder{}
	//创建编码器
	fc.Rsec = NewRSEC(dn, pn)
	//创建签名器
	fc.Sigger = pdp.NewSig(params, g)
	//初始化元数据记录器
	fc.MetaFileMap = make(map[string]*util.Meta4File)
	return &fc
}

// 【客户端执行】对文件分块，EC编码，签名
func (fc *FileCoder) Setup(filename string, filedata string) []util.DataShard {
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
	//为文件初始化一个元数据记录器
	meta4file := util.NewMeta4File()
	//对切片中的每个块签名后构建DataShard
	var dataShardSlice []util.DataShard
	for i := 0; i < dsnum; i++ {
		time := time.Now().Format("2006-01-02 15:04:05")
		sig := fc.Sigger.GetSig(dataSlice[i], time, 0)
		var dsno string
		if i < fc.Rsec.DataNum {
			dsno = "d-" + strconv.Itoa(i)
		} else {
			dsno = "p-" + strconv.Itoa(i-fc.Rsec.DataNum)
		}
		datashard := util.NewDataShard(dsno, dataSlice[i], sig, 0, time)
		dataShardSlice = append(dataShardSlice, *datashard)
		//将分片信息写入元数据记录器中
		fc.MFMMutex.Lock()
		meta4file.Update(dsno, datashard.Timestamp, datashard.Version)
		fc.MFMMutex.Unlock()
	}
	//将该文件的元数据记录器写入fc中
	fc.MetaFileMap[filename] = meta4file
	return dataShardSlice
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
	fc.MFMMutex.Lock()
	fc.MetaFileMap[filename].Update(ds.DSno, ds.Timestamp, ds.Version)
	fc.MFMMutex.Unlock()
	//计算数据增量
	incData := fc.Rsec.GetIncData(oldData, newData)
	//计算校验块前缀
	dsnosplit := strings.Split(ds.DSno, "-")
	pprefix := ""
	if len(dsnosplit) == 2 {
		pprefix = "p-"
	} else {
		pprefix = "p-" + dsnosplit[1] + "-"
	}
	//计算校验块增量，校验块签名增量，生成增量校验分片
	var incParityShards []util.DataShard
	for i := 0; i < fc.Rsec.ParityNum; i++ {
		//计算校验块增量和矩阵系数
		incParity := fc.Rsec.GetIncParity(incData, i+fc.Rsec.DataNum, drow)
		//计算签名增量
		fc.MFMMutex.RLock()
		oldV := fc.MetaFileMap[filename].LatestVersionSlice[pprefix+strconv.Itoa(i)]
		oldT := fc.MetaFileMap[filename].LatestTimestampSlice[pprefix+strconv.Itoa(i)]
		fc.MFMMutex.RUnlock()
		newV := oldV + 1
		newT := time.Now().Format("2006-01-02 15:04:05")
		incSig := fc.Sigger.GetIncSig(incParity, oldV, oldT, newV, newT)
		iPS := util.NewDataShard(pprefix+strconv.Itoa(i), incParity, incSig, newV, newT)
		fc.MFMMutex.Lock()
		fc.MetaFileMap[filename].Update(pprefix+strconv.Itoa(i), newT, newV)
		fc.MFMMutex.Unlock()
		incParityShards = append(incParityShards, *iPS)
	}
	return incParityShards
}

// 【存储节点执行】根据校验块增量分片更新校验块分片
func UpdateParityShard(oldParityShard *util.DataShard, incParityShard *util.DataShard, params string) *util.DataShard {
	//更新分片数据
	oldParityShard.Data = UpdateParity(oldParityShard.Data, incParityShard.Data)
	//更新分片签名
	oldParityShard.Sig = pdp.UpdateSigByIncSig(params, oldParityShard.Sig, incParityShard.Sig)
	//更新分片版本
	oldParityShard.Version = incParityShard.Version
	//更新分片时间戳
	oldParityShard.Timestamp = incParityShard.Timestamp
	return oldParityShard
}

func TestFileCoder() {
	dataNum := 5
	parityNum := 3
	params := pbc.GenerateA(160, 512).String()
	pairing, _ := pbc.NewPairingFromString(params)
	g := pairing.NewG2().Rand().Bytes()
	//创建文件编码器
	fileCoder := NewFileCoder(dataNum, parityNum, params, g)
	//数据文件-字符串型
	filename := "testFile"
	fileStr := "abcdef123456123456123456123456"
	//Setup
	dataShards := fileCoder.Setup(filename, fileStr)
	fmt.Println("【测试Setup】")
	for i := 0; i < len(dataShards); i++ {
		dataShards[i].Print()
	}
	//验签
	fmt.Println("【测试验签】")
	for i := 0; i < len(dataShards); i++ {
		ds := dataShards[i]
		fmt.Println("Verify datashard-", i)
		pdp.VerifySig(params, fileCoder.Sigger.G, fileCoder.Sigger.PubKey, ds.Data, ds.Sig, ds.Version, ds.Timestamp)
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
		newParityShard := UpdateParityShard(&dataShards[i], &incParityShards[i-dataNum], params)
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
		pdp.VerifySig(params, fileCoder.Sigger.G, fileCoder.Sigger.PubKey, ds.Data, ds.Sig, ds.Version, ds.Timestamp)
	}

}
