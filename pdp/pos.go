package pdp

import (
	"crypto/sha256"
	"fmt"

	"github.com/Nik-U/pbc"
)

type POS struct {
	DataProof []byte
	SigProof  []byte
}

func ProvePos(pairing *pbc.Pairing, gb []byte, pubkey []byte, message string, signature []byte, randoms int32) *POS {
	//生成data proof
	g := pairing.NewG1().SetBytes(gb)
	pk := pairing.NewG2().SetBytes(pubkey)
	egpk := pairing.NewGT().Pair(g, pk)
	m := pairing.NewZr().SetBytes([]byte(message))
	ms := pairing.NewZr().MulInt32(m, randoms)
	dp := pairing.NewGT().PowZn(egpk, ms)
	//生成sig proof
	sig := pairing.NewG1().SetBytes(signature)
	randomsz := pairing.NewZr().SetInt32(randoms)
	sp := pairing.NewG1().PowZn(sig, randomsz)
	pos := POS{dp.Bytes(), sp.Bytes()}
	return &pos
}

func VerifyPos(pos *POS, pairing *pbc.Pairing, gb []byte, pubkey []byte, v int32, t string, randoms int32) bool {
	vs := v * randoms
	h := pairing.NewG1().SetFromStringHash(t, sha256.New())
	vsz := pairing.NewZr().SetInt32(vs)
	vts := pairing.NewG1().PowZn(h, vsz)
	pk := pairing.NewG2().SetBytes(pubkey)
	evtspk := pairing.NewGT().Pair(vts, pk)
	g := pairing.NewG2().SetBytes(gb)
	sp := pairing.NewG1().SetBytes(pos.SigProof)
	espg := pairing.NewGT().Pair(sp, g)
	dp := pairing.NewGT().SetBytes(pos.DataProof)
	temp := pairing.NewGT().Mul(dp, evtspk)
	if !temp.Equals(espg) {
		fmt.Println("*BUG* Signature check failed *BUG*")
		return false
	} else {
		fmt.Println("Signature verified correctly")
		return true
	}
}
