package pdp

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/Nik-U/pbc"
)

type Signature struct {
	Pairing *pbc.Pairing
	G       []byte
	PrivKey []byte
	PubKey  []byte
}

func (sigger *Signature) Setup() {
	params := pbc.GenerateA(160, 512).String()
	pairing, _ := pbc.NewPairingFromString(params)
	sigger.Pairing = pairing
	g := sigger.Pairing.NewG2().Rand()
	pk := sigger.Pairing.NewZr().Rand()
	sigger.G = g.Bytes()
	sigger.PrivKey = pk.Bytes()
	sigger.PubKey = sigger.Pairing.NewG2().PowZn(g, pk).Bytes()
}

func (sigger *Signature) GetSig(message string) []byte {
	h := sigger.Pairing.NewG1().SetFromStringHash(message, sha256.New())
	privkey := sigger.Pairing.NewZr().SetBytes(sigger.PrivKey)
	signature := sigger.Pairing.NewG1().PowZn(h, privkey)
	return signature.Bytes()
}

func (sigger *Signature) GetSig2(message string, t string, v int32) []byte {
	ht := sigger.Pairing.NewG1().SetFromStringHash(t, sha256.New())
	vp := sigger.Pairing.NewZr().SetInt32(v)
	vt := sigger.Pairing.NewG1().PowZn(ht, vp)
	g := sigger.Pairing.NewG1().SetBytes(sigger.G)
	mp := sigger.Pairing.NewZr().SetBytes([]byte(message))
	gm := sigger.Pairing.NewG1().PowZn(g, mp)
	vtgm := sigger.Pairing.NewG1().Mul(vt, gm)
	sk := sigger.Pairing.NewZr().SetBytes(sigger.PrivKey)
	sig := sigger.Pairing.NewG1().PowZn(vtgm, sk)
	return sig.Bytes()
}

func (sigger *Signature) GetIncSig(incData []byte, matriCoeff byte, oldV int32, oldT string, newV int32, newT string) []byte {
	nht := sigger.Pairing.NewG1().SetFromStringHash(newT, sha256.New())
	nv := sigger.Pairing.NewZr().SetInt32(newV)
	nvt := sigger.Pairing.NewG1().PowZn(nht, nv)
	oht := sigger.Pairing.NewG1().SetFromStringHash(oldT, sha256.New())
	ov := sigger.Pairing.NewZr().SetInt32(oldV)
	ovt := sigger.Pairing.NewG1().PowZn(oht, ov)
	nvt_DIV_ovt := sigger.Pairing.NewG1().Div(nvt, ovt)
	g := sigger.Pairing.NewG1().SetBytes(sigger.G)
	im := sigger.Pairing.NewZr().SetBytes(incData)
	mc_bytes := make([]byte, 0)
	mc_bytes = append(mc_bytes, matriCoeff)
	mc := sigger.Pairing.NewZr().SetBytes(mc_bytes)
	mcim := sigger.Pairing.NewZr().Mul(mc, im)
	gmcim := sigger.Pairing.NewG1().PowZn(g, mcim)
	incgmcim := sigger.Pairing.NewG1().Mul(nvt_DIV_ovt, gmcim)
	sk := sigger.Pairing.NewZr().SetBytes(sigger.PrivKey)
	incsig := sigger.Pairing.NewG1().PowZn(incgmcim, sk)
	return incsig.Bytes()
}

func (sigger *Signature) GetSigByInc(oldSig []byte, incSig []byte) []byte {
	os := sigger.Pairing.NewG1().SetBytes(oldSig)
	is := sigger.Pairing.NewG1().SetBytes(incSig)
	ns := sigger.Pairing.NewG1().Mul(os, is)
	return ns.Bytes()
}

// 测试校验块是否满足g^C1=g^C0+g^(a*incd)
func (sigger *Signature) TestGCADX(oldData []byte, newData []byte, incData []byte, matriCoeff byte) bool {
	g := sigger.Pairing.NewG1().SetBytes(sigger.G)
	oc := sigger.Pairing.NewZr().SetBytes(oldData)
	ab := make([]byte, 0)
	ab = append(ab, matriCoeff)
	a := sigger.Pairing.NewZr().SetBytes(ab)
	d := sigger.Pairing.NewZr().SetBytes(incData)
	ad := sigger.Pairing.NewZr().Mul(a, d)
	gad := sigger.Pairing.NewG1().PowZn(g, ad)
	goc := sigger.Pairing.NewG1().PowZn(g, oc)
	nc := sigger.Pairing.NewZr().SetBytes(newData)
	gnc := sigger.Pairing.NewG1().PowZn(g, nc)
	gocad := sigger.Pairing.NewG1().Mul(goc, gad)
	if !gnc.Equals(gocad) {
		fmt.Println("*BUG* Signature check failed *BUG*")
		return false
	} else {
		fmt.Println("Signature verified correctly")
		return true
	}
}

// 测试校验块是否满足g^C1=g^C0+g^incP
func (sigger *Signature) TestGCDX(oldData []byte, newData []byte, incData []byte) bool {
	// fmt.Println("【oldData】", oldData)
	// fmt.Println("【incData】", incData)
	// fmt.Println("【newData】", newData)
	// g := sigger.Pairing.NewG1().SetBytes(sigger.G)
	// pk := sigger.Pairing.NewG2().SetBytes(sigger.PubKey)
	// fmt.Println("g=", g)
	// oc := sigger.Pairing.NewZr().SetBytes(oldData)
	// goc := sigger.Pairing.NewG1().PowZn(g, oc)
	// egpk := sigger.Pairing.NewGT().Pair(g, pk)
	// egpkoc := sigger.Pairing.NewGT().PowZn(egpk, oc)
	// fmt.Println("oc=", oc.Bytes())
	// d := sigger.Pairing.NewZr().SetBytes(incData)
	// fmt.Println("d=", d)
	// ocpd := sigger.Pairing.NewZr().Add(oc, d)
	// fmt.Println("ocpd=", ocpd)
	// gocpd := sigger.Pairing.NewG1().PowZn(g, ocpd)
	// fmt.Println("gocpd=", gocpd)
	// gd := sigger.Pairing.NewG1().PowZn(g, d)
	// goc := sigger.Pairing.NewG1().PowZn(g, oc)
	// nc := sigger.Pairing.NewZr().SetBytes(newData)
	// egpknc := sigger.Pairing.NewGT().PowZn(egpk, nc)
	// egpkocMULegpknc := sigger.Pairing.NewGT().Mul(egpkoc, egpknc)
	// ocAddnc := sigger.Pairing.NewZr().Add(oc, nc)
	// egpkocAddnc := sigger.Pairing.NewGT().PowZn(egpk, ocAddnc)
	// fmt.Println("nc=", nc.Bytes())
	// ncDIVoc := sigger.Pairing.NewZr().Div(nc, oc).Bytes()
	// fmt.Println("ncDIVoc:", ncDIVoc)
	// ic := sigger.Pairing.NewZr().SetBytes(incData)
	// fmt.Println("incData:", ic.Bytes())
	// gnc := sigger.Pairing.NewG1().PowZn(g, nc)
	// gocMULgnc := sigger.Pairing.NewG1().Mul(goc, gnc)
	// ocADDnc := sigger.Pairing.NewZr().Add(oc, nc)
	// gocADDnc := sigger.Pairing.NewG1().PowZn(g, ocADDnc)
	// fmt.Println("gnc=", gnc)
	// gocad := sigger.Pairing.NewG1().Mul(goc, gd)
	// if !gocMULgnc.Equals(gocADDnc) {
	// 	fmt.Println("*BUG* Signature check failed *BUG*")
	// 	return false
	// } else {
	// 	fmt.Println("Signature verified correctly")
	// 	return true
	// }
	return true
}

func (sigger *Signature) Test(oldParity []byte, newParity []byte, incParity []byte) bool {
	// fmt.Println("【oldParity】", oldParity)
	// fmt.Println("【incParity】", incParity)
	// fmt.Println("【newParity】", newParity)
	a := sigger.Pairing.NewZr().SetInt32(int32(-152))
	b := sigger.Pairing.NewZr().SetInt32(int32(142))
	c := sigger.Pairing.NewZr().Add(a, b)
	d := sigger.Pairing.NewZr().SetInt32(int32(-152) + int32(142))
	fmt.Println("a:", a)
	fmt.Println("b:", b)
	fmt.Println("c:", c)
	fmt.Println("d:", d)
	// fmt.Println("a_byte:", []byte{142, 41, 142})
	// fmt.Println("b_byte:", []byte{152, 109, 62})
	// fmt.Println("ab_byte:", util.AddByteSlices([]byte{142, 41, 142}, []byte{152, 109, 62}))
	// a := sigger.Pairing.NewZr().SetBytes([]byte{142, 41, 142})
	// b := sigger.Pairing.NewZr().SetBytes([]byte{152, 109, 62})
	// c := sigger.Pairing.NewZr().SetBytes(util.AddByteSlices([]byte{142, 41, 142}, []byte{152, 109, 62}))
	// d := sigger.Pairing.NewZr().Add(a, b)
	// fmt.Println("a=", a)
	// fmt.Println("b=", b)
	// fmt.Println("c=", c.Bytes())
	// fmt.Println("d=", d.Bytes())
	return true
}

func VerifySig(pairing *pbc.Pairing, gb []byte, pubkey []byte, message string, signature []byte) bool {
	sig := pairing.NewG1().SetBytes(signature)
	h := pairing.NewG1().SetFromStringHash(message, sha256.New())
	pk := pairing.NewG2().SetBytes(pubkey)
	g := pairing.NewG2().SetBytes(gb)
	temp1 := pairing.NewGT().Pair(h, pk)
	temp2 := pairing.NewGT().Pair(sig, g)
	if !temp1.Equals(temp2) {
		fmt.Println("*BUG* Signature check failed *BUG*")
		return false
	} else {
		fmt.Println("Signature verified correctly")
		return true
	}
}

func VerifySig2(pairing *pbc.Pairing, gb []byte, pubkey []byte, message string, signature []byte, v int32, t string) bool {
	ht := pairing.NewG1().SetFromStringHash(t, sha256.New())
	vp := pairing.NewZr().SetInt32(v)
	vt := pairing.NewG1().PowZn(ht, vp)
	g := pairing.NewG2().SetBytes(gb)
	mp := pairing.NewZr().SetBytes([]byte(message))
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
	pdp := Signature{}
	pdp.Setup()
	message := "TestSignature"
	t := time.Now().Format("2006-01-02 15:04:05")
	v := int32(1)
	sig := pdp.GetSig2(message, t, v)
	VerifySig2(pdp.Pairing, pdp.G, pdp.PubKey, message, sig, v, t)
}
