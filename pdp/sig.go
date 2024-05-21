package pdp

import (
	"ECDS/util"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/Nik-U/pbc"
)

type Signature struct {
	Pairing *pbc.Pairing
	Params  string
	G       []byte
	PrivKey []byte
	PubKey  []byte
}

// 【客户端执行】创建签名器
func NewSig() *Signature {
	sigger := Signature{}
	params := pbc.GenerateA(160, 512).String()
	pairing, _ := pbc.NewPairingFromString(params)
	sigger.Params = params
	sigger.Pairing = pairing
	g := sigger.Pairing.NewG2().Rand()
	pk := sigger.Pairing.NewZr().Rand()
	sigger.G = g.Bytes()
	sigger.PrivKey = pk.Bytes()
	sigger.PubKey = sigger.Pairing.NewG2().PowZn(g, pk).Bytes()
	return &sigger
}

// 【客户端执行】对消息进行签名
func (sigger *Signature) GetSig(message []int32, t string, v int32) []byte {
	ht := sigger.Pairing.NewG1().SetFromStringHash(t, sha256.New())
	vp := sigger.Pairing.NewZr().SetInt32(v)
	vt := sigger.Pairing.NewG1().PowZn(ht, vp)
	g := sigger.Pairing.NewG1().SetBytes(sigger.G)
	mp := sigger.Pairing.NewZr().SetInt32(MessageToInt32(message))
	gm := sigger.Pairing.NewG1().PowZn(g, mp)
	vtgm := sigger.Pairing.NewG1().Mul(vt, gm)
	sk := sigger.Pairing.NewZr().SetBytes(sigger.PrivKey)
	sig := sigger.Pairing.NewG1().PowZn(vtgm, sk)
	return sig.Bytes()
}

func MessageToInt32(m []int32) int32 {
	// fmt.Println("m:", m)
	sum := int32(0)
	for i := 0; i < len(m); i++ {
		sum += m[i]
	}
	// fmt.Println("sum:", sum)
	return sum
}

// 【客户端执行】生成增量签名
func (sigger *Signature) GetIncSig(incData []int32, oldV int32, oldT string, newV int32, newT string) []byte {
	nht := sigger.Pairing.NewG1().SetFromStringHash(newT, sha256.New())
	nv := sigger.Pairing.NewZr().SetInt32(newV)
	nvt := sigger.Pairing.NewG1().PowZn(nht, nv)
	oht := sigger.Pairing.NewG1().SetFromStringHash(oldT, sha256.New())
	ov := sigger.Pairing.NewZr().SetInt32(oldV)
	ovt := sigger.Pairing.NewG1().PowZn(oht, ov)
	nvt_DIV_ovt := sigger.Pairing.NewG1().Div(nvt, ovt)
	g := sigger.Pairing.NewG1().SetBytes(sigger.G)
	im := sigger.Pairing.NewZr().SetInt32(MessageToInt32(incData))
	gmcim := sigger.Pairing.NewG1().PowZn(g, im)
	incgmcim := sigger.Pairing.NewG1().Mul(nvt_DIV_ovt, gmcim)
	sk := sigger.Pairing.NewZr().SetBytes(sigger.PrivKey)
	incsig := sigger.Pairing.NewG1().PowZn(incgmcim, sk)
	return incsig.Bytes()
}

// 【存储节点执行】根据增量签名增量更新数据块签名
func UpdateSigByIncSig(pairing *pbc.Pairing, oldSig []byte, incSig []byte) []byte {
	os := pairing.NewG1().SetBytes(oldSig)
	is := pairing.NewG1().SetBytes(incSig)
	ns := pairing.NewG1().Mul(os, is)
	return ns.Bytes()
}

// 测试校增量签名
func (sigger *Signature) TestIncSig(oldV int32, newV int32, oldT string, newT string) bool {
	nht := sigger.Pairing.NewG1().SetFromStringHash(newT, sha256.New())
	nv := sigger.Pairing.NewZr().SetInt32(newV)
	nvt := sigger.Pairing.NewG1().PowZn(nht, nv)
	oht := sigger.Pairing.NewG1().SetFromStringHash(oldT, sha256.New())
	ov := sigger.Pairing.NewZr().SetInt32(oldV)
	ovt := sigger.Pairing.NewG1().PowZn(oht, ov)
	nvt_DIV_ovt := sigger.Pairing.NewG1().Div(nvt, ovt)
	nvt_DIV_ovt_MUL_ovt := sigger.Pairing.NewG1().Mul(nvt_DIV_ovt, ovt)
	if !nvt_DIV_ovt_MUL_ovt.Equals(nvt) {
		fmt.Println("*BUG* Signature check failed *BUG*")
		return false
	} else {
		fmt.Println("【TestIncSig】Signature verified correctly")
		return true
	}
}

// 【存储节点执行】根据数据块签名和系数生成校验块的增量签名
func GetParityIncSig(pairing *pbc.Pairing, oldSig []byte, oldVT []byte, newVT []byte, coeff int32) []byte {
	//将oldSig去时间戳和版本号
	oldsig := pairing.NewG1().SetBytes(oldSig)
	olgvt := pairing.NewG1().SetBytes(oldVT)
	oldGM := pairing.NewG1().Div(oldsig, olgvt)
	//将oldGM幂乘系数
	coeff_p := pairing.NewZr().SetInt32(coeff)
	newGM := pairing.NewG1().PowZn(oldGM, coeff_p)
	//给newGM添加新时间戳和版本号
	newvt := pairing.NewG1().SetBytes(newVT)
	newsig := pairing.NewG1().Mul(newvt, newGM)
	return newsig.Bytes()
}

func (sigger *Signature) TestParityIncSig(v1 int32, t1 string, m []int32, v2 int32, t2 string, coef int32) bool {
	//生成sig1
	sig1 := sigger.GetSig(m, t1, v1)
	//方法一计算sig2
	m2 := util.VectorMulInt32(m, coef)
	sig2 := sigger.Pairing.NewG1().SetBytes(sigger.GetSig(m2, t2, v2))
	//方法二计算sig3
	oht := sigger.Pairing.NewG1().SetFromStringHash(t1, sha256.New())
	ov := sigger.Pairing.NewZr().SetInt32(v1)
	ovt := sigger.Pairing.NewG1().PowZn(oht, ov)
	sk := sigger.Pairing.NewZr().SetBytes(sigger.PrivKey)
	ovtsk := sigger.Pairing.NewG1().PowZn(ovt, sk)
	nht := sigger.Pairing.NewG1().SetFromStringHash(t2, sha256.New())
	nv := sigger.Pairing.NewZr().SetInt32(v2)
	nvt := sigger.Pairing.NewG1().PowZn(nht, nv)
	nvtsk := sigger.Pairing.NewG1().PowZn(nvt, sk)
	sig3 := sigger.Pairing.NewG1().SetBytes(GetParityIncSig(sigger.Pairing, sig1, ovtsk.Bytes(), nvtsk.Bytes(), coef))
	//比较sig2和sig3是否一致
	if !sig2.Equals(sig3) {
		fmt.Println("*BUG* Signature check failed *BUG*")
		return false
	} else {
		fmt.Println("【TestParityIncSig】Signature verified correctly")
		return true
	}
}

func VerifySig(pairing *pbc.Pairing, gb []byte, pubkey []byte, message []int32, signature []byte, v int32, t string) bool {
	ht := pairing.NewG1().SetFromStringHash(t, sha256.New())
	vp := pairing.NewZr().SetInt32(v)
	vt := pairing.NewG1().PowZn(ht, vp)
	g := pairing.NewG2().SetBytes(gb)
	mp := pairing.NewZr().SetInt32(MessageToInt32(message))
	gm := pairing.NewG1().PowZn(g, mp)
	vtgm := pairing.NewG1().Mul(vt, gm)
	pk := pairing.NewG2().SetBytes(pubkey)
	sig := pairing.NewG1().SetBytes(signature)
	temp1 := pairing.NewGT().Pair(vtgm, pk)
	temp2 := pairing.NewGT().Pair(sig, g)
	if !temp1.Equals(temp2) {
		fmt.Println("*BUG* Signature check failed *BUG*")
		return false
	} else {
		fmt.Println("Signature verified correctly")
		return true
	}
}

func TestSignature() {
	sigger := NewSig()
	message := "TestSignature"
	messageInt32 := util.ByteSliceToInt32Slice([]byte(message))
	t1 := time.Now().Format("2006-01-02 15:04:05")
	v1 := int32(1)
	// sig := sigger.GetSig(messageInt32, t, v)
	time.Sleep(time.Second)
	t2 := time.Now().Format("2006-01-02 15:04:05")
	v2 := int32(2)
	coef := int32(8)
	sigger.TestParityIncSig(v1, t1, messageInt32, v2, t2, coef)
	// VerifySig(sigger.Pairing, sigger.G, sigger.PubKey, messageInt32, sig, v, t)
}
