package main

import (
	"ECDS/util"
	"fmt"
	"strconv"
)

func main() {
	num := 5
	var leafHashes [][]byte
	for i := 0; i < num; i++ {
		l := []byte("leaf" + strconv.Itoa(i))
		leafHashes = append(leafHashes, l)
	}
	for i := 0; i < len(leafHashes); i++ {
		fmt.Println("leafHashes[", i, "]=", string(leafHashes[i]))
	}
	root, path := util.BuildMerkleTreeAndGeneratePath(leafHashes, 4)
	fmt.Printf("Root: %x\n", root)
	fmt.Printf("Path: %x\n", path)
	newroot := util.GenerateRootByPaths([]byte("leaf4"), 4, path)
	fmt.Printf("NewRoot: %x\n", newroot)
}
