package encode

import (
	"fmt"

	"gonum.org/v1/gonum/mat"
)

func TestMatrix() {
	matrix := MakeEncodeMatrix(5, 3)
	PrintMatrix(matrix)
	rows := []int{0, 1, 3, 4, 5}
	subM := matrix.GetSubMatrix(rows)
	PrintMatrix(subM)
	inverM := GetInverse(subM)
	PrintMatrix(inverM)
}

type Matrix [][]int32

func MakeEncodeMatrix(d, p int) Matrix {
	r := d + p
	m := make([][]int32, r)
	// Create identity matrix upper.
	for i := 0; i < d; i++ {
		m[i] = make([]int32, d)
		m[i][i] = int32(1)
	}

	// Create vandermonde matrix below.
	vandermonde := GenVandermondeMatrix(p, d)
	for i := d; i < r; i++ {
		m[i] = make([]int32, d)
		for j := 0; j < d; j++ {
			m[i][j] = vandermonde[i-d][j]
		}
	}
	return m
}

func GenVandermondeMatrix(p, d int) Matrix {
	x := make([]int32, p)
	for i := 0; i < p; i++ {
		x[i] = int32(i + 1)
	}
	// 生成 Vandermonde 矩阵
	vandermondeMatrix := make([][]int32, p)
	for i := range vandermondeMatrix {
		vandermondeMatrix[i] = make([]int32, d)
		for j := range vandermondeMatrix[i] {
			vandermondeMatrix[i][j] = pow(x[i], int32(j))
		}
	}
	// 打印 Vandermonde 矩阵
	fmt.Println("Vandermonde Matrix:")
	PrintMatrix(vandermondeMatrix)
	return vandermondeMatrix
}

// 计算 x 的 n 次方
func pow(x, n int32) int32 {
	result := int32(1)
	for i := int32(0); i < n; i++ {
		result *= x
	}
	return result
}

func PrintMatrix(m Matrix) {
	for _, row := range m {
		fmt.Println(row)
	}
}

func (m Matrix) GetSubMatrix(rows []int) Matrix {
	subM := make([][]int32, len(rows))
	for i := 0; i < len(rows); i++ {
		subM[i] = make([]int32, len(m[0]))
		copy(subM[i], m[rows[i]])
	}
	return subM
}

func GetInverse(m Matrix) Matrix {
	//将m矩阵转换为float64向量
	mSlice := make([]float64, len(m)*len(m[0]))
	for i := 0; i < len(m); i++ {
		for j := 0; j < len(m[0]); j++ {
			mSlice[i*len(m)+j] = float64(m[i][j])
		}
	}
	fmt.Println(mSlice)
	A := mat.NewDense(len(m), len(m[0]), mSlice)
	// 计算逆矩阵
	var inv mat.Dense
	if err := inv.Inverse(A); err != nil {
		fmt.Println("Cannot compute the inverse:", err)
		return nil
	}
	inverM := make([][]int32, len(m))
	for i := 0; i < len(m); i++ {
		inverM[i] = make([]int32, len(m[0]))
		for j := 0; j < len(m[0]); j++ {
			inverM[i][j] = int32(inv.At(i, j))
		}
	}
	return inverM
}

func (m Matrix) MulInt32(n int32) Matrix {
	row := len(m)
	col := len(m[0])
	result := make([][]int32, row)
	for i := range result {
		result[i] = make([]int32, col)
		for j := range result[i] {
			result[i][j] = m[i][j] * n
		}
	}
	return result
}

func (m Matrix) AddMatrix(mm Matrix) Matrix {
	row := len(m)
	col := len(m[0])
	result := make([][]int32, row)
	for i := range result {
		result[i] = make([]int32, col)
		for j := range result[i] {
			result[i][j] = m[i][j] + mm[i][j]
		}
	}
	return result
}
