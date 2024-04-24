package util

import (
	"fmt"
	"math"
)

func SplitString(str string, n int) []string {
	// 计算每份的长度
	length := len(str)
	partLength := int(math.Ceil(float64(length) / float64(n)))

	// 切分字符串
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		start := i * partLength
		end := start + partLength
		if end > length {
			end = length
		}
		parts[i] = str[start:end]
	}

	return parts
}

func SubtractBytes(byte1, byte2 []byte) []byte {
	// 检查切片长度是否相等
	if len(byte1) != len(byte2) {
		panic("切片长度不相等，无法进行减法运算")
	}

	// 创建结果切片
	result := make([]byte, len(byte1))

	// 对每个元素进行减法运算
	for i := 0; i < len(byte1); i++ {
		result[i] = byte1[i] - byte2[i]
	}

	return result
}

func AddByteSlices(a, b []byte) []byte {
	// 确保两个 []byte 的长度相同
	if len(a) != len(b) {
		panic("Slices must have same length")
	}

	// 创建一个新的 []byte，用于存放结果
	result := make([]byte, len(a))

	// 遍历两个 []byte，按元素相加
	for i := range a {
		result[i] = a[i] + b[i]
	}

	return result
}

func ByteSliceToInt32Slice(bs []byte) []int32 {
	is := make([]int32, len(bs))
	for i := 0; i < len(bs); i++ {
		is[i] = int32(bs[i])
	}
	return is
}

func PrintInt32Slices(is [][]int32) {
	for i := 0; i < len(is); i++ {
		fmt.Println(is[i])
	}
}

func AddInt32Slice(a, b []int32) []int32 {
	// 确保两个 []byte 的长度相同
	if len(a) != len(b) {
		panic("Slices must have same length")
	}

	// 创建一个新的 []byte，用于存放结果
	result := make([]int32, len(a))

	// 遍历两个 []byte，按元素相加
	for i := range a {
		result[i] = a[i] + b[i]
	}

	return result
}
