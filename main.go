package main

import (
	"fmt"

	"github.com/templexxx/reedsolomon"
)

func main() {
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
	for i := range data {
		fmt.Println(i, string(data[i]))
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
	for i := range compdata {
		fmt.Println(i, string(compdata[i]))
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
	for i := range data {
		fmt.Println(i, string(data[i]))
	}

	//修改compdata，重编码
	compdata[updaterow] = newdata
	err = encoder.Encode(compdata)
	if err != nil {
		panic(err)
	}
	//Print the compdate
	for i := range compdata {
		fmt.Println(i, string(compdata[i]))
	}

}
