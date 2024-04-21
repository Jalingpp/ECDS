package signature

import (
	"crypto/sha256"
	"fmt"

	"github.com/Nik-U/pbc"
)

type PDP struct {
	Pairing *pbc.Pairing
	G       []byte
	PrivKey []byte
	PubKey  []byte
}

func (pdp *PDP) Setup() {
	params := pbc.GenerateA(160, 512).String()
	pairing, _ := pbc.NewPairingFromString(params)
	pdp.Pairing = pairing
	g := pdp.Pairing.NewG2().Rand()
	pk := pdp.Pairing.NewZr().Rand()
	pdp.G = g.Bytes()
	pdp.PrivKey = pk.Bytes()
	pdp.PubKey = pdp.Pairing.NewG2().PowZn(g, pk).Bytes()
}

func (pdp *PDP) GetSig(message string) []byte {
	h := pdp.Pairing.NewG1().SetFromStringHash(message, sha256.New())
	privkey := pdp.Pairing.NewZr().SetBytes(pdp.PrivKey)
	signature := pdp.Pairing.NewG2().PowZn(h, privkey)
	return signature.Bytes()
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

func TestSignature() {
	//setup
	params := pbc.GenerateA(160, 512)
	pairing := params.NewPairing()
	g := pairing.NewG1()
	h := pairing.NewG2()
	x := pairing.NewGT()
	g.Rand()
	h.Rand()
	fmt.Printf("g=%s\n", g)
	fmt.Printf("h=%s\n", h)
	x.Pair(g, h)
	fmt.Printf("e(g,h)=%s\n", x)
}
