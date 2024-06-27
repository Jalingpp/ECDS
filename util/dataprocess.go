package util

import (
	pb "ECDS/proto" // 根据实际路径修改
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"
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

// 求base的exponent次方
func Power(base, exponent int) int {
	result := 1
	for i := 0; i < exponent; i++ {
		result *= base
	}
	return result
}

func VectorMulInt32(v []int32, n int32) []int32 {
	result := make([]int32, len(v))
	for i := 0; i < len(v); i++ {
		result[i] = n * v[i]
	}
	return result
}

func VectorAddVector(v1, v2 []int32) []int32 {
	result := make([]int32, len(v1))
	for i := 0; i < len(v1); i++ {
		result[i] = v1[i] + v2[i]
	}
	return result
}

func Int32SliceToStr(is []int32) string {
	bs := make([]byte, len(is))
	for i := 0; i < len(bs); i++ {
		bs[i] = byte(is[i])
	}
	return string(bs)
}

// 计算SHA-256哈希
func Hash(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

// 生成随机数s_i并计算叶子节点H(H(s_i + dataslice[i]))
func GenerateLeaf(data []int32) ([]byte, int32) {
	// 生成随机数s_i
	rand.Seed(time.Now().UnixNano())
	s_i := rand.Int31()

	// 将s_i转换为字节数组
	s_iBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(s_iBytes, uint32(s_i))

	// 将data转换为字节数组
	dataBytes := make([]byte, 4*len(data))
	for i, v := range data {
		binary.LittleEndian.PutUint32(dataBytes[i*4:], uint32(v))
	}

	// 计算s_i + dataslice[i]
	combined := append(s_iBytes, dataBytes...)

	// 计算H(H(s_i + dataslice[i]))
	firstHash := Hash(combined)
	secondHash := Hash(firstHash)

	return secondHash, s_i
}

// 构建默克尔树
func BuildMerkleTree(leafHashes [][]byte) []byte {
	if len(leafHashes) == 1 {
		return leafHashes[0]
	}

	var newLevel [][]byte
	for i := 0; i < len(leafHashes); i += 2 {
		if i+1 < len(leafHashes) {
			combined := append(leafHashes[i], leafHashes[i+1]...)
			newHash := Hash(combined)
			newLevel = append(newLevel, newHash)
		} else {
			// 如果是奇数个叶子节点，则直接提升
			newLevel = append(newLevel, leafHashes[i])
		}
	}

	return BuildMerkleTree(newLevel)
}

// 将一个string转换为int32
func StrToInt32(str string) int32 {
	val, err := strconv.ParseInt(str, 10, 32)
	if err != nil {
		// 返回 int32 的零值 (0)，或者你可以选择在这里记录错误或采取其他措施
		return 0
	}
	return int32(val)
}

// 将二维数组转换为 Int32ArraySN 消息
func Int32SliceToInt32ArraySNSlice(int32slice [][]int32) []*pb.Int32ArraySN {
	var dataShardSlice []*pb.Int32ArraySN
	for _, shard := range int32slice {
		dataShardSlice = append(dataShardSlice, &pb.Int32ArraySN{Values: shard})
	}
	return dataShardSlice
}
