package main

import "ECDS/nodes"

func main() {
	dn := 11
	pn := 20
	snaddrfilename := "/root/ECDS/data/snaddr3"
	datadir := "/root/ECDS/data/"
	//创建一个审计员
	// auditor := nodes.NewAuditor("10.0.4.29:50051", snaddrfilename, dn, pn)
	auditor := nodes.NewAuditor("localhost:50051", snaddrfilename, dn, pn, datadir)
	auditor.PrintAuditor()
	select {}
}
