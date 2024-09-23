package main

// import (
// 	"ECDS/pdp"
// 	"ECDS/util"
// 	"fmt"

// 	"github.com/Nik-U/pbc"
// )

// func main() {
// 	// t := "12345678"
// 	params1 := pbc.GenerateA(160, 512).String()
// 	fmt.Println("params1:", params1)
// 	pairing, _ := pbc.NewPairingFromString(params1)
// 	m1 := pairing.NewZr().SetInt32(pdp.MessageToInt32(util.ByteSliceToInt32Slice([]byte("12345"))))
// 	ms1 := pairing.NewZr().MulInt32(m1, 2)
// 	m2 := pairing.NewZr().SetInt32(pdp.MessageToInt32(util.ByteSliceToInt32Slice([]byte("abcde"))))
// 	ms2 := pairing.NewZr().MulInt32(m2, 3)
// 	ms12 := pairing.NewZr().Add(ms1, ms2)
// 	g := pairing.NewG1().Rand()
// 	pk := pairing.NewG2().Rand()
// 	egpk := pairing.NewGT().Pair(g, pk)
// 	dp1 := pairing.NewGT().PowZn(egpk, ms1)
// 	dp2 := pairing.NewGT().PowZn(egpk, ms2)
// 	dp1a2 := pairing.NewGT().PowZn(egpk, ms12)
// 	dp1m2 := pairing.NewGT().Mul(dp1, dp2)
// 	if dp1a2 == dp1m2 {
// 		fmt.Println("dp1a2=dp1m2")
// 	} else {
// 		fmt.Println("dp1a2!=dp1m2")
// 	}

// 	// fmt.Println("h1:", pairing.NewG1().SetFromStringHash(t, sha256.New()))
// 	// params2 := pbc.GenerateA(160, 512).String()
// 	// fmt.Println("params2:", params2)
// 	// pairing2, _ := pbc.NewPairingFromString(params2)
// 	// fmt.Println("h2:", pairing2.NewG1().SetFromStringHash(t, sha256.New()))
// }
