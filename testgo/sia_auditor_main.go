package main

import sianodes "ECDS/baselines/sia"

func main() {
	dn := 11
	pn := 20
	snaddrfilename := "data/snaddr"
	//创建一个审计员
	// sianodes.NewSiaAC("10.0.4.29:50051", snaddrfilename, dn, pn)
	sianodes.NewSiaAC("localhost:50051", snaddrfilename, dn, pn)
	select {}
}
