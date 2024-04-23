package encode

import "errors"

type RSEC struct {
	DataNum      int
	ParityNum    int
	EncodeMatrix Matrix
}

func TestRSEC() {

}

func New(d, p int) *RSEC {
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
