package encode

import (
	"fmt"

	"github.com/templexxx/reedsolomon"
	xor "github.com/templexxx/xorsimd"
)

type FileCoder struct {
	rsec       *reedsolomon.RS
	Data_num   int
	Parity_num int
}

func (rsec *FileCoder) Setup(dn int, pn int) {
	//创建编码器
	ec, err := reedsolomon.New(dn, pn)
	if err != nil {
		panic(err)
	}
	rsec.rsec = ec
	rsec.Data_num = dn
	rsec.Parity_num = pn
}

func (rsec *FileCoder) Encode(dataslice [][]byte) [][]byte {
	//RS纠删码编码
	err := rsec.rsec.Encode(dataslice)
	if err != nil {
		panic(err)
	}
	// // Print the data
	// for i := data_num; i < len(dataslice); i++ {
	// 	fmt.Println("打印编码生成的冗余块：")
	// 	fmt.Println(i, string(dataslice[i]))
	// }
	return dataslice
}

// 计算数据的增量
func GetIncData(oldData []byte, newData []byte) []byte {
	buf := make([]byte, len(oldData))
	xor.Encode(buf, [][]byte{oldData, newData})
	return buf
}

// 根据数据块的增量更新冗余块
func (rsec *FileCoder) UpdateParity(incData []byte, row int, parity [][]byte) (err error) {
	vects := make([][]byte, 1+rsec.rsec.ParityNum)
	vects[0] = incData
	gm := make([]byte, rsec.rsec.ParityNum)
	for i := 0; i < rsec.rsec.ParityNum; i++ {
		col := row
		off := i*rsec.rsec.DataNum + col
		c := rsec.rsec.GenMatrix[off]
		gm[i] = c
		vects[i+1] = parity[i]
	}
	rs := &reedsolomon.RS{DataNum: 1, ParityNum: rsec.rsec.ParityNum, GenMatrix: gm, CpuFeat: rsec.rsec.CpuFeat}
	rs.Encode_update(vects, true)
	return nil
}

// 获取第row个数据块对应的每个校验块的生成系数
func (rsec *FileCoder) GetMatrixElement(row int) []byte {
	gm := make([]byte, rsec.rsec.ParityNum)
	for i := 0; i < rsec.rsec.ParityNum; i++ {
		col := row
		off := i*rsec.rsec.DataNum + col
		c := rsec.rsec.GenMatrix[off]
		gm[i] = c
	}
	return gm
}

func TestEC() {
	//创建编码器
	encoder, err := reedsolomon.New(5, 3)
	if err != nil {
		panic(err)
	}
	//创建数据分片
	data := make([][]byte, 8)
	//初始化数据分片
	for i := range data {
		data[i] = make([]byte, 7)
	}
	data[0] = []byte("abcdefg")
	data[1] = []byte("hijklmn")
	data[2] = []byte("opqrstu")
	data[3] = []byte("vwxyz01")
	data[4] = []byte("2345678")
	//RS纠删码编码
	err = encoder.Encode(data)
	if err != nil {
		panic(err)
	}
	// Print the data
	fmt.Println("第1次输出：")
	for i := range data {
		fmt.Println(i, data[i])
	}
	// //验证数据分片可恢复性
	// ok, err1 := encoder.Verify(data)
	// if err1 != nil {
	// 	panic(err1)
	// }
	// fmt.Println("Verify:", ok)
	// //删除数据分片
	// data[1] = nil
	// // Print the data
	// for i := range data {
	// 	fmt.Println(i, string(data[i]))
	// }
	// //重建数据分片
	// err = encoder.Reconstruct(data)
	// if err != nil {
	// 	panic(err)
	// }
	// // Print the data
	// for i := range data {
	// 	fmt.Println(i, string(data[i]))
	// }
	//生成一个对比数据分片
	compdata := make([][]byte, 8)
	for i := range compdata {
		compdata[i] = make([]byte, 7)
		copy(compdata[i], data[i])
	}
	//Print the compdate
	fmt.Println("第2次输出：")
	for i := range compdata {
		fmt.Println(i, compdata[i])
	}
	//更新数据分片
	newdata := []byte("1234567")
	updaterow := 1
	parityData := make([][]byte, 3)
	parityData[0] = make([]byte, 7)
	parityData[1] = data[6]
	parityData[2] = make([]byte, 7)
	err = encoder.Update(data[updaterow], newdata, updaterow, parityData)
	if err != nil {
		panic(err)
	}
	// Print the data
	fmt.Println("第3次输出：")
	for i := range data {
		fmt.Println(i, data[i])
	}

	fmt.Println("输出parityData：")
	for i := 0; i < len(parityData); i++ {
		fmt.Println(i, parityData[i])
	}

	//修改compdata，重编码
	compdata[updaterow] = newdata
	err = encoder.Encode(compdata)
	if err != nil {
		panic(err)
	}
	//Print the compdate
	fmt.Println("第4次输出：")
	for i := range compdata {
		fmt.Println(i, compdata[i])
	}

}
