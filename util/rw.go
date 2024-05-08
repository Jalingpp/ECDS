package util

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
)

func ReadStringFromFile(filepath string) string {
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

// 返回一个文件的读取器
func BufIOReader(filepath string) *bufio.Reader {
	// 打开文件
	file, err := os.Open(filepath)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return nil
	}
	defer file.Close()

	// 创建一个 bufio.Reader 对象
	reader := bufio.NewReader(file)
	return reader
}
