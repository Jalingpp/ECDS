package main

import baselines "ECDS/baselines/filecoin"

func main() {
	f := 10
	snaddrfilename := "/root/DSN/ECDS/data/snaddrs"
	//创建一个审计员
	// baselines.NewFilecoinAC("10.0.4.29:50051", snaddrfilename, f)
	baselines.NewFilecoinAC("localhost:50051", snaddrfilename, f)
	// auditor.PrintFilecoinAuditor()
	select {}
}
