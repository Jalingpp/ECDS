package pdp

import (
	"crypto/sha256"

	"github.com/Nik-U/pbc"
)

type Signature struct {
	Params  string
	G       []byte
	PrivKey []byte
	PubKey  []byte
}

// 【客户端执行】创建签名器
func NewSig(params string, gb []byte) *Signature {
	sigger := Signature{}
	pairing, _ := pbc.NewPairingFromString(params)
	sigger.Params = params
	pk := pairing.NewZr().Rand()
	sigger.G = gb
	g := pairing.NewG2().SetBytes(gb)
	sigger.PrivKey = pk.Bytes()
	sigger.PubKey = pairing.NewG2().PowZn(g, pk).Bytes()
	return &sigger
}

// 【客户端执行】对消息进行签名
func (sigger *Signature) GetSig(message []int32, t string, v int32) []byte {
	pairing, _ := pbc.NewPairingFromString(sigger.Params)
	ht := pairing.NewG1().SetFromStringHash(t, sha256.New())
	vp := pairing.NewZr().SetInt32(v)
	vt := pairing.NewG1().PowZn(ht, vp)
	g := pairing.NewG1().SetBytes(sigger.G)
	mp := pairing.NewZr().SetInt32(MessageToInt32(message))
	gm := pairing.NewG1().PowZn(g, mp)
	vtgm := pairing.NewG1().Mul(vt, gm)
	sk := pairing.NewZr().SetBytes(sigger.PrivKey)
	sig := pairing.NewG1().PowZn(vtgm, sk)
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
	pairing, _ := pbc.NewPairingFromString(sigger.Params)
	nht := pairing.NewG1().SetFromStringHash(newT, sha256.New())
	nv := pairing.NewZr().SetInt32(newV)
	nvt := pairing.NewG1().PowZn(nht, nv)
	oht := pairing.NewG1().SetFromStringHash(oldT, sha256.New())
	ov := pairing.NewZr().SetInt32(oldV)
	ovt := pairing.NewG1().PowZn(oht, ov)
	nvt_DIV_ovt := pairing.NewG1().Div(nvt, ovt)
	g := pairing.NewG1().SetBytes(sigger.G)
	im := pairing.NewZr().SetInt32(MessageToInt32(incData))
	gmcim := pairing.NewG1().PowZn(g, im)
	incgmcim := pairing.NewG1().Mul(nvt_DIV_ovt, gmcim)
	sk := pairing.NewZr().SetBytes(sigger.PrivKey)
	incsig := pairing.NewG1().PowZn(incgmcim, sk)
	return incsig.Bytes()
}

// 【存储节点执行】根据增量签名增量更新数据块签名
func UpdateSigByIncSig(params string, oldSig []byte, incSig []byte) []byte {
	pairing, _ := pbc.NewPairingFromString(params)
	os := pairing.NewG1().SetBytes(oldSig)
	is := pairing.NewG1().SetBytes(incSig)
	ns := pairing.NewG1().Mul(os, is)
	return ns.Bytes()
}

// 测试校增量签名
func (sigger *Signature) TestIncSig(oldV int32, newV int32, oldT string, newT string) bool {
	pairing, _ := pbc.NewPairingFromString(sigger.Params)
	nht := pairing.NewG1().SetFromStringHash(newT, sha256.New())
	nv := pairing.NewZr().SetInt32(newV)
	nvt := pairing.NewG1().PowZn(nht, nv)
	oht := pairing.NewG1().SetFromStringHash(oldT, sha256.New())
	ov := pairing.NewZr().SetInt32(oldV)
	ovt := pairing.NewG1().PowZn(oht, ov)
	nvt_DIV_ovt := pairing.NewG1().Div(nvt, ovt)
	nvt_DIV_ovt_MUL_ovt := pairing.NewG1().Mul(nvt_DIV_ovt, ovt)
	if !nvt_DIV_ovt_MUL_ovt.Equals(nvt) {
		// fmt.Println("*BUG* Signature check failed *BUG*")
		return false
	} else {
		// fmt.Println("【TestIncSig】Signature verified correctly")
		return true
	}
}

// 【存储节点执行】根据数据块签名和系数生成校验块的增量签名
func GetParityIncSig(params string, oldSig []byte, oldVT []byte, newVT []byte, coeff int32) []byte {
	pairing, _ := pbc.NewPairingFromString(params)
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

func VerifySig(params string, gb []byte, pubkey []byte, message []int32, signature []byte, v int32, t string) bool {
	pairing, _ := pbc.NewPairingFromString(params)
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
		// fmt.Println("*BUG* Signature check failed *BUG*", "params:")
		return false
	} else {
		// fmt.Println("Signature verified correctly")
		return true
	}
}
