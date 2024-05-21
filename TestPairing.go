package main

import (
	"fmt"

	"github.com/Nik-U/pbc"
)

func main() {
	// t := "12345678"
	params1 := pbc.GenerateA(160, 512).String()
	fmt.Println("params1:", params1)
	// pairing, _ := pbc.NewPairingFromString(params1)
	// fmt.Println("h1:", pairing.NewG1().SetFromStringHash(t, sha256.New()))
	params2 := pbc.GenerateA(160, 512).String()
	fmt.Println("params2:", params2)
	// pairing2, _ := pbc.NewPairingFromString(params2)
	// fmt.Println("h2:", pairing2.NewG1().SetFromStringHash(t, sha256.New()))
}
