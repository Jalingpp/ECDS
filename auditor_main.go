package main

import "ECDS/nodes"

func main() {
	dn := 11
	pn := 20
	snaddrfilename := "data/snaddr"
	//创建一个审计员
	auditor := nodes.NewAuditor("localhost:50051", snaddrfilename, dn, pn)
	auditor.PrintAuditor()
	select {}
}
