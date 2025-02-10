package util

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
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

// 为字节数组创建一个临时文件
func CreateTempFile4Bytes(someBytes []byte, filename string) *os.File {
	fmt.Println("filename:", filename)
	// 创建一个临时文件
	pieceFile, err := os.CreateTemp("", filename)
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(pieceFile.Name())
	fmt.Println("after createtemp")

	// 将随机数据写入文件
	_, err = pieceFile.Write(someBytes)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("after write somebytes")

	// 关闭文件以确保数据写入
	err = pieceFile.Close()
	if err != nil {
		log.Fatal(err)
	}

	return pieceFile
}

// 创建一个临时文件
func CreateTempFile(filename string) *os.File {
	// 创建一个临时文件
	file, err := os.CreateTemp("", filename)
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(file.Name())

	// 关闭文件以确保数据写入
	err = file.Close()
	if err != nil {
		log.Fatal(err)
	}

	return file
}

// 读文件中一行地址
func ReadOneAddr(filepath string) (string, error) {
	// 打开文件
	//读取存储节点地址
	reader := BufIOReader(filepath)
	// 逐行读取文件内容
	// 读取一行数据
	line, err := reader.ReadString('\n')
	if err != nil {
		e := errors.New("read addr error")
		return "", e // 文件读取结束或者发生错误时退出循环
	}
	line = strings.TrimRight(line, "\n")
	return line, nil
}

// getDatabaseSize 计算指定目录的文件总大小
func GetDatabaseSize(dbPath string) (int64, error) {
	var size int64
	err := filepath.Walk(dbPath, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
