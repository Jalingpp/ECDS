package util

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
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
	// defer file.Close()

	// 创建一个 bufio.Reader 对象
	reader := bufio.NewReader(file)
	return reader
}

// 读存储节点地址文件
func ReadSNAddrFile(filepath string) *map[string]string {
	snaddrmap := make(map[string]string)
	//读取存储节点地址
	reader := BufIOReader(filepath)
	// 逐行读取文件内容
	for {
		// 读取一行数据
		line, err := reader.ReadString('\n')
		if err != nil {
			break // 文件读取结束或者发生错误时退出循环
		}
		// 处理一行数据
		fmt.Print("ReadSNAddr:", line)
		//写入map
		lineslice := strings.Split(strings.TrimRight(line, "\n"), ",") //去除换行符
		snaddrmap[lineslice[0]] = lineslice[1]
	}
	return &snaddrmap
}

// 写一个字符串到文件末尾
func LogToFile(filepath string, content string) {
	// 打开文件以附加内容（如果文件不存在则创建）
	file, err := os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	// 写入内容到文件末尾
	if _, err := file.WriteString(content); err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}
}
