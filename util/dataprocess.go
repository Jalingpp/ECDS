package util

import (
	pb "ECDS/proto" // 根据实际路径修改
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
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

		// 如果当前部分不是最后一部分，并且长度小于partLength，则用0填充
		if i == n-1 && len(parts[i]) < partLength {
			parts[i] = strings.Repeat("0", partLength-len(parts[i])) + parts[i]
		}
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

// 根据给定的随机数为数据分片生成叶子
func GenerateLeafByRands(data []int32, rand int32) []byte {
	// 将s_i转换为字节数组
	s_iBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(s_iBytes, uint32(rand))
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
	return secondHash
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

// 生成默克尔树根
func GenerateMTRoot(datashards [][]int32, rands []int32, parityleaves [][]byte) []byte {
	var leaves [][]byte

	for i := 0; i < len(datashards); i++ {
		leaf := GenerateLeafByRands(datashards[i], rands[i])
		leaves = append(leaves, leaf)
	}

	for i := 0; i < len(parityleaves); i++ {
		leaves = append(leaves, parityleaves[i])
	}

	return BuildMerkleTree(leaves)
}

// 根据preleafs生成默克尔树根
func GenerateMTRootByPreleafs(preleafs [][]byte) []byte {
	leafs := make([][]byte, 0)

	for i := 0; i < len(preleafs); i++ {
		lf := Hash(preleafs[i])
		leafs = append(leafs, lf)
	}

	return BuildMerkleTree(leafs)
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

// BuildMerkleTreeAndGeneratePath 构建默克尔树，并返回根哈希和指定叶子节点的验证路径
func BuildMerkleTreeAndGeneratePath(leafHashes [][]byte, index int) ([]byte, [][]byte) {
	if len(leafHashes) == 1 {
		return leafHashes[0], nil // 只有一个叶子时，没有验证路径
	}
	var newLevel [][]byte
	var paths [][]byte // 存储每个叶子节点的验证路径
	for i := 0; i < len(leafHashes); i += 2 {
		if i+1 < len(leafHashes) {
			combined := append(leafHashes[i], leafHashes[i+1]...)
			newHash := Hash(combined)
			newLevel = append(newLevel, newHash)
			if i == index {
				// 如果当前叶子是目标叶子，记录兄弟节点的哈希
				paths = append(paths, leafHashes[i+1])
			} else if index == i+1 {
				paths = append(paths, leafHashes[i])
			}
		} else {
			// 如果是奇数个叶子节点，则复制自身
			combined := append(leafHashes[i], leafHashes[i]...)
			newHash := Hash(combined)
			newLevel = append(newLevel, newHash)
			if i == index {
				paths = append(paths, leafHashes[i])
			}
		}

	}
	root, path := BuildMerkleTreeAndGeneratePath(newLevel, index/2) // 递归构建下一级
	// 将当前层的路径与下一级的路径合并
	paths = append(paths, path...)
	return root, paths
}

// 根据路径生成根节点哈希值
func GenerateRootByPaths(leaf []byte, index int, paths [][]byte) []byte {
	if paths == nil && index == 0 {
		return leaf
	}
	root := leaf
	tempindex := index
	for i := 0; i < len(paths); i++ {
		if tempindex%2 == 0 {
			combined := append(root, paths[i]...)
			root = Hash(combined)
		} else {
			combined := append(paths[i], root...)
			root = Hash(combined)
		}
		tempindex = tempindex / 2
	}
	return root
}

// 比较时间a是否在b之后：a在b之后则返回1，a在b之前则返回2，相同则返回0
func IsTimeAfter(a string, b string) int {
	// 将字符串解析为 time.Time 类型
	time1, err := time.Parse("2006-01-02 15:04:05", a)
	if err != nil {
		fmt.Println("Error parsing time:", err)
		return -1
	}
	time2, err := time.Parse("2006-01-02 15:04:05", b)
	if err != nil {
		fmt.Println("Error parsing time:", err)
		return -1
	}

	// 比较两个时间
	if time1.After(time2) {
		return 1
	} else if time1.Before(time2) {
		return 2
	} else {
		return 0
	}
}
