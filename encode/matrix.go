package encode

import (
	"fmt"

	"gonum.org/v1/gonum/mat"
)

func TestMatrix() {
	matrix := MakeEncodeMatrix(5, 3)
	fmt.Println("Total Matrix:")
	matrix.PrintMatrix()
	rows := []int{0, 1, 3, 4, 7}
	subM := matrix.GetSubMatrix(rows)
	fmt.Println("SubM:")
	subM.PrintMatrix()
	// 计算子矩阵的行列式
	det := Determinant(subM)
	fmt.Println("det:", det)
	// 计算子矩阵的伴随矩阵
	adjM := Adjugate(subM)
	fmt.Println("adjM:")
	adjM.PrintMatrix()

}

type Matrix [][]int32

// 创建一个Matrix，并初始化所有值为0
func InitMatrix(rows int, col int) Matrix {
	matrix := make([][]int32, rows)
	for i := 0; i < rows; i++ {
		matrix[i] = make([]int32, col)
	}
	return matrix
}

// 创建一个Matrix，并初始化为给定的二维数组
func InitMatrixByArray(array [][]int32) Matrix {
	matrix := make([][]int32, len(array))
	for i := 0; i < len(array); i++ {
		matrix[i] = make([]int32, len(array[0]))
		copy(matrix[i], array[i])
	}
	return matrix
}

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

func (m Matrix) PrintMatrix() {
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

func (m Matrix) GetInverse() Matrix {
	//将m矩阵转换为float64向量
	mSlice := make([]float64, len(m)*len(m[0]))
	for i := 0; i < len(m); i++ {
		for j := 0; j < len(m[0]); j++ {
			mSlice[i*len(m)+j] = float64(m[i][j])
		}
	}
	fmt.Println("将int32矩阵转换为float64向量：")
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
			fmt.Println(inv.At(i, j), ",")
			inverM[i][j] = int32(inv.At(i, j))
		}
	}
	return inverM
}

// determinant 计算矩阵的行列式
func Determinant(matrix [][]int32) int32 {
	n := len(matrix)
	if n == 0 || len(matrix[0]) != n {
		return 0 // 如果不是方阵，则行列式为 0
	}

	if n == 1 {
		return matrix[0][0] // 如果是 1x1 矩阵，则行列式为该元素的值
	}

	// 递归计算行列式
	var result int32
	for j, val := range matrix[0] {
		subMatrix := make([][]int32, n-1)
		for i := 1; i < n; i++ {
			subMatrix[i-1] = make([]int32, n-1)
			copy(subMatrix[i-1], matrix[i][:j])
			copy(subMatrix[i-1][j:], matrix[i][j+1:])
		}
		subDet := Determinant(subMatrix)
		if j%2 == 0 {
			result += val * subDet
		} else {
			result -= val * subDet
		}
	}
	return result
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

func (m Matrix) MulMatrix(n Matrix) Matrix {
	//检查两个矩阵是否能够相乘
	rowM := len(m)
	colM := len(m[0])
	rowN := len(n)
	colN := len(n[0])
	if colM != rowN {
		panic("Number of columns in matrix M must equal the number of rows in matrix N")
	}
	result := make([][]int32, rowM)
	for i := range result {
		result[i] = make([]int32, colN)
		for j := range result[i] {
			var sum int32
			for k := 0; k < colM; k++ {
				sum += m[i][k] * n[k][j]
			}
			result[i][j] = sum
		}
	}

	return result
}

// cofactor 计算代数余子式
func Cofactor(matrix [][]int32, row, col int) int32 {
	subMatrix := [][]int32{}
	for i := 0; i < len(matrix); i++ {
		if i == row {
			continue
		}
		subRow := []int32{}
		for j := 0; j < len(matrix[i]); j++ {
			if j == col {
				continue
			}
			subRow = append(subRow, matrix[i][j])
		}
		subMatrix = append(subMatrix, subRow)
	}
	return int32((-1)^(row+col)) * Determinant(subMatrix)
}

// adjugate 计算矩阵的伴随矩阵
func Adjugate(matrix [][]int32) Matrix {
	n := len(matrix)
	adj := make([][]int32, n)
	for i := range adj {
		adj[i] = make([]int32, n)
		for j := range adj[i] {
			adj[i][j] = Cofactor(matrix, j, i) // 注意此处 j, i 的顺序，以便进行转置
		}
	}
	return adj
}
