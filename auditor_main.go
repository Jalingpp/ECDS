package main

import "ECDS/nodes"

func main() {
	dn := 11
	pn := 20
	snaddrfilename := "data/snaddr2"
	//创建一个审计员
	auditor := nodes.NewAuditor("10.0.4.29:50051", snaddrfilename, dn, pn)
	auditor.PrintAuditor()
	select {}
}
