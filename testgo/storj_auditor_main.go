package main

import storjnodes "ECDS/baselines/storj"

func main() {
	dn := 11
	pn := 20
	snaddrfilename := "/root/DSN/ECDS/data/snaddrs"
	datadir := "/home/ubuntu/ECDS/data/"
	//创建一个审计员
	// auditor := storjnodes.NewStorjAC("10.0.4.29:50051", snaddrfilename, dn, pn)
	auditor := storjnodes.NewStorjAC("localhost:50051", snaddrfilename, dn, pn, datadir)
	auditor.PrintStorjAuditor()
	select {}
}
