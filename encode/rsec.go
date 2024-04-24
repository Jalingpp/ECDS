package encode

import (
	"ECDS/util"
	"errors"
	"fmt"
)

type RSEC struct {
	DataNum      int
	ParityNum    int
	EncodeMatrix Matrix
}

func TestRSEC() {
	dn := 5
	pn := 3
	rsec := NewRSEC(dn, pn)
	dataslice := make([][]int32, dn+pn)
	for i := 0; i < dn+pn; i++ {
		dataslice[i] = make([]int32, 6)
	}
	data1 := "abcdef"
	dataslice[0] = util.ByteSliceToInt32Slice([]byte(data1))
	fmt.Println("dataslice[0]:", dataslice[0])
	data2 := "123456"
	dataslice[1] = util.ByteSliceToInt32Slice([]byte(data2))
	fmt.Println("dataslice[1]:", dataslice[1])
	data3 := "123456"
	dataslice[2] = util.ByteSliceToInt32Slice([]byte(data3))
	fmt.Println("dataslice[2]:", dataslice[2])
	data4 := "123456"
	dataslice[3] = util.ByteSliceToInt32Slice([]byte(data4))
	fmt.Println("dataslice[3]:", dataslice[3])
	data5 := "123456"
	dataslice[4] = util.ByteSliceToInt32Slice([]byte(data5))
	fmt.Println("dataslice[4]:", dataslice[4])
	fmt.Println()
	fmt.Println("dataslice:")
	util.PrintInt32Slices(dataslice)
	fmt.Println()
	//编码
	rsec.Encode(dataslice)
	fmt.Println("after encode:")
	util.PrintInt32Slices(dataslice)
	//解码
	//剩余系数行切片
	rows := []int{0, 1, 3, 4, 7}
	//构造剩余数据矩阵
	resdata := make([][]int32, rsec.DataNum)
	for i := 0; i < rsec.DataNum; i++ {
		resdata[i] = make([]int32, len(dataslice[0]))
		copy(resdata[i], dataslice[rows[i]])
	}
	rsec.Decode(resdata, rows)

}

func NewRSEC(d, p int) *RSEC {
	encodeMatrix := MakeEncodeMatrix(d, p)
	return &RSEC{d, p, encodeMatrix}
}

func (rsec *RSEC) Encode(dataslice [][]int32) (err error) {
	err = rsec.checkEncode(dataslice)
	if err != nil {
		return
	}
	//生成校验块
	for i := rsec.DataNum; i < rsec.DataNum+rsec.ParityNum; i++ {
		dataslice[i] = rsec.GetParity(i, dataslice)
	}
	return
}

// 计算校验块：row是系数矩阵中校验块所在的行号
func (rsec *RSEC) GetParity(row int, dataslice [][]int32) []int32 {
	result := make([]int32, len(dataslice[0]))
	coffs := rsec.EncodeMatrix[row]
	for i := 0; i < len(coffs); i++ {
		result = VectorAddVector(result, VectorMulInt32(dataslice[i], coffs[i]))
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

var (
	ErrMismatchVects    = errors.New("too few/many vects given")
	ErrZeroVectSize     = errors.New("vect size is 0")
	ErrMismatchVectSize = errors.New("vects size mismatched")
)

func (rsec *RSEC) checkEncode(vects [][]int32) (err error) {
	rows := len(vects)
	if rsec.DataNum+rsec.ParityNum != rows {
		return ErrMismatchVects
	}
	size := len(vects[0])
	if size == 0 {
		return ErrZeroVectSize
	}
	for i := 1; i < rows; i++ {
		if len(vects[i]) != size {
			return ErrMismatchVectSize
		}
	}
	return
}

// 根据剩余的数据块解码出完整数据：res剩余数据块或冗余块，rows是系数矩阵中res块对应的行号
func (rsec *RSEC) Decode(res [][]int32, rows []int) [][]int32 {
	//检查res和rows
	if len(res) < rsec.DataNum || len(rows) < rsec.DataNum {
		panic("Rest data shards is not enough")
	}
	//获取剩余系数矩阵
	coffs := InitMatrix(rsec.DataNum, rsec.DataNum)
	for i := 0; i < rsec.DataNum; i++ {
		copy(coffs[i], rsec.EncodeMatrix[rows[i]])
	}
	fmt.Println()
	fmt.Println("剩余系数矩阵：")
	coffs.PrintMatrix()
	//计算剩余系数矩阵的行列式
	det := Determinant(coffs)
	fmt.Println("剩余系数矩阵的行列式：", det)
	//计算剩余系数矩阵的伴随矩阵
	adjCoffs := Adjugate(coffs)
	fmt.Println("剩余系数矩阵的伴随矩阵：")
	adjCoffs.PrintMatrix()
	fmt.Println()

	fmt.Println("剩余数据矩阵：")
	dataMatrix := InitMatrixByArray(res)
	dataMatrix.PrintMatrix()

	fmt.Println("伴随矩阵×剩余系数矩阵：")
	mulCoffs := adjCoffs.MulMatrix(coffs)
	mulCoffs.PrintMatrix()

	//计算完整数据
	fmt.Println("伴随矩阵×剩余数据矩阵：")
	result := adjCoffs.MulMatrix(res)
	result.PrintMatrix()
	return result
}
