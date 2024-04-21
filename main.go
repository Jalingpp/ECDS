package main

import (
	"ECDS/signature"
)

func main() {
	// signature.TestSignature()
	PDP := signature.PDP{}
	PDP.Setup()
	message := "TestSignature"
	sig := PDP.GetSig(message)
	signature.VerifySig(PDP.Pairing, PDP.G, PDP.PubKey, message, sig)
}
