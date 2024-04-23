package util

import (
	"fmt"
	"io/ioutil"
)

func ReadStringFromFile(filepath string) string{
	// 读取二进制文件
	data, err := ioutil.ReadFile(filepath)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return ""
	}

	// 将二进制数据解码为字符串
	str := string(data)

	// fmt.Println("String from binary file:", str)

	return str
}