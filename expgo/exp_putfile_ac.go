package main

import (
	baselines "ECDS/baselines/filecoin"
	sianodes "ECDS/baselines/sia"
	storjnodes "ECDS/baselines/storj"
	"ECDS/nodes"
	"ECDS/util"
	"fmt"
	"log"
	"os"
	"strconv"
)

func main() {
	//固定参数
	dn := 11
	pn := 20
	f := 10
	acAddr, _ := util.ReadOneAddr("data/acaddr")
	snAddrFilepath := "data/snaddrs"

	//传入参数
	args := os.Args
	dsnMode := args[1]                    //dsn模式
	clientNum, _ := strconv.Atoi(args[2]) //客户端数量

	util.LogToFile("data/outlog_ac", "[putfile-w1-"+dsnMode+"-clientNum"+strconv.Itoa(clientNum)+"]")
	fmt.Println("[putfile-w1-" + dsnMode + "-clientNum" + strconv.Itoa(clientNum) + "]")
	ecac, filecoinac, storjac, siaac := CreateAuditorByMode(dsnMode, acAddr, snAddrFilepath, dn, pn, f)
	if dsnMode == "ec" {
		fmt.Println(ecac.IpAddr)
	} else if dsnMode == "filecoin" {
		fmt.Println(filecoinac.IpAddr)
	} else if dsnMode == "storj" {
		fmt.Println(storjac.IpAddr)
	} else if dsnMode == "sia" {
		fmt.Println(siaac.IpAddr)
	} else {
		log.Fatalln("dsnMode error")
	}

	select {}
}

func CreateAuditorByMode(dsnMode string, acaddr string, snaddrfn string, dn int, pn int, f int) (*nodes.Auditor, *baselines.FilecoinAC, *storjnodes.StorjAC, *sianodes.SiaAC) {
	if dsnMode == "ec" {
		auditor := nodes.NewAuditor(acaddr, snaddrfn, dn, pn)
		auditor.PrintAuditor()
		return auditor, nil, nil, nil
	} else if dsnMode == "filecoin" {
		auditor := baselines.NewFilecoinAC(acaddr, snaddrfn, f)
		return nil, auditor, nil, nil
	} else if dsnMode == "storj" {
		auditor := storjnodes.NewStorjAC(acaddr, snaddrfn, dn, pn)
		auditor.PrintStorjAuditor()
		return nil, nil, auditor, nil
	} else if dsnMode == "sia" {
		auditor := sianodes.NewSiaAC(acaddr, snaddrfn, dn, pn)
		return nil, nil, nil, auditor
	} else {
		log.Fatalln("dsnMode error")
		return nil, nil, nil, nil
	}
}
