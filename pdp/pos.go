package pdp

import (
	"ECDS/util"
	"crypto/sha256"
	"fmt"

	"github.com/Nik-U/pbc"
)

type POS struct {
	DSno      string
	DataProof []byte
	SigProof  []byte
}

// 【存储节点执行】生成存储证明
func ProvePos(pairing *pbc.Pairing, pi *util.PublicInfo, ds *util.DataShard, randoms int32) *POS {
	//生成data proof
	g := pairing.NewG1().SetBytes(pi.G)
	pk := pairing.NewG2().SetBytes(pi.PK)
	egpk := pairing.NewGT().Pair(g, pk)
	m := pairing.NewZr().SetInt32(MessageToInt32(ds.Data))
	ms := pairing.NewZr().MulInt32(m, randoms)
	dp := pairing.NewGT().PowZn(egpk, ms)
	//生成sig proof
	sig := pairing.NewG1().SetBytes(ds.Sig)
	randomsz := pairing.NewZr().SetInt32(randoms)
	sp := pairing.NewG1().PowZn(sig, randomsz)
	pos := POS{ds.DSno, dp.Bytes(), sp.Bytes()}
	return &pos
}

// 【客户端执行】验证存储证明
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

func (pos *POS) Print() {
	fmt.Println("data proof:", pos.DataProof)
	fmt.Println("sig proof:", pos.SigProof)
}
