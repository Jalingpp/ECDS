//go:build cgo
// +build cgo

package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"
	prooftypes "github.com/filecoin-project/go-state-types/proof"
)

func main() {
	minerID := abi.ActorID(42)
	sealProofType := abi.RegisteredSealProof_StackedDrg2KiBV1

	sectorCacheDirPath := requireTempDirPath("sector-cache-dir")
	defer os.RemoveAll(sectorCacheDirPath)
	stagedSectorFile := requireTempFile(bytes.NewReader([]byte{}), 0)
	defer stagedSectorFile.Close()
	sealedSectorFile := requireTempFile(bytes.NewReader([]byte{}), 0)
	defer sealedSectorFile.Close()

	data := []byte("your data here")
	paddedData, err := PaddleData(data)
	if err != nil {
		log.Fatalf("failed to pad data: %v", err)
	}
	log.Println("paddedData:", paddedData)

	someBytes := make([]byte, abi.PaddedPieceSize(2048).Unpadded())

	pieceFileA := requireTempFile(bytes.NewReader(paddedData[0:1016]), 1016)
	//将data写入pieceFileA的方法一
	pieceCIDA, err := ffi.GeneratePieceCIDFromFile(sealProofType, pieceFileA, 1016)
	if err != nil {
		log.Println("GeneratePieceCIDFromFile Error:", err)
	}
	pieceFileA.Seek(0, 0)

	// 将data写入pieceFileA的方法二
	// _, _, err = ffi.WriteWithoutAlignment(sealProofType, pieceFileA, 515, stagedSectorFile)
	_, _, err = ffi.WriteWithoutAlignment(sealProofType, pieceFileA, 1016, stagedSectorFile)
	if err != nil {
		fmt.Println("WriteWithoutAlignment", err.Error())
	}
	pieceFileA.Seek(0, 0)

	pieceFileB := requireTempFile(bytes.NewReader(someBytes[0:508]), 508)
	pieceCIDB, err := ffi.GeneratePieceCIDFromFile(sealProofType, pieceFileB, 508)
	if err != nil {
		fmt.Println("GeneratePieceCIDFromFile", err.Error())
	}
	_, err = pieceFileB.Seek(0, 0)
	if err != nil {
		fmt.Println("Seek", err.Error())
	}
	_, _, _, err = ffi.WriteWithAlignment(sealProofType, pieceFileB, 508, stagedSectorFile, []abi.UnpaddedPieceSize{1016})
	if err != nil {
		fmt.Println("WriteWithAlignment", err.Error())
	}

	// 构建分片的公共信息
	// publicPieces := []abi.PieceInfo{{
	// 	Size:     abi.UnpaddedPieceSize(len(paddedData)).Padded(),
	// 	PieceCID: pieceCIDA,
	// }}
	publicPieces := []abi.PieceInfo{{
		Size:     abi.UnpaddedPieceSize(1016).Padded(),
		PieceCID: pieceCIDA,
	}, {
		Size:     abi.UnpaddedPieceSize(508).Padded(),
		PieceCID: pieceCIDB,
	}}

	// 为sector中的数据分片生成未封装CID
	// _, err = ffi.GenerateUnsealedCID(sealProofType, publicPieces)
	// ffi.GenerateUnsealedCID(sealProofType, publicPieces)
	sectorNum := abi.SectorNumber(1)
	ticket := abi.SealRandomness{5, 4, 2}
	seed := abi.InteractiveSealRandomness{7, 4, 2}
	// 预提交封装
	sealPreCommitPhase1Output, err := ffi.SealPreCommitPhase1(sealProofType, sectorCacheDirPath, stagedSectorFile.Name(), sealedSectorFile.Name(), sectorNum, minerID, ticket, publicPieces)
	if err != nil {
		fmt.Println("SealPreCommitPhase1", err.Error())
	}
	// log.Println("sealPreCommitPhase1Output:", sealPreCommitPhase1Output)
	sealedCID, unsealedCID, err := ffi.SealPreCommitPhase2(sealPreCommitPhase1Output, sectorCacheDirPath, sealedSectorFile.Name())
	if err != nil {
		fmt.Println("SealPreCommitPhase2", err.Error())
	}
	// 提交封装
	sealCommitPhase1Output, err := ffi.SealCommitPhase1(sealProofType, sealedCID, unsealedCID, sectorCacheDirPath, sealedSectorFile.Name(), sectorNum, minerID, ticket, seed, publicPieces)
	if err != nil {
		fmt.Println("SealCommitPhase1", err.Error())
	}
	proof, err := ffi.SealCommitPhase2(sealCommitPhase1Output, sectorNum, minerID)
	if err != nil {
		fmt.Println("SealCommitPhase2", err.Error())
	}
	// log.Println("sealproof:", proof)
	// // 验证封装证明
	isValid, err := ffi.VerifySeal(prooftypes.SealVerifyInfo{
		SectorID: abi.SectorID{
			Miner:  minerID,
			Number: sectorNum,
		},
		SealedCID:             sealedCID,
		SealProof:             sealProofType,
		Proof:                 proof,
		DealIDs:               []abi.DealID{},
		Randomness:            ticket,
		InteractiveRandomness: seed,
		UnsealedCID:           unsealedCID,
	})
	if err != nil {
		fmt.Println("VerifySeal", err.Error())
	}
	log.Println("verify seal:", isValid)
	// 构建Sector的私有信息
	winningPostProofType := abi.RegisteredPoStProof_StackedDrgWinning2KiBV1
	privateInfo := ffi.NewSortedPrivateSectorInfo(ffi.PrivateSectorInfo{
		SectorInfo: prooftypes.SectorInfo{
			SectorNumber: sectorNum,
			SealedCID:    sealedCID,
		},
		CacheDirPath:     sectorCacheDirPath,
		PoStProofType:    winningPostProofType,
		SealedSectorPath: sealedSectorFile.Name(),
	})
	// 构建待证明的扇区信息集合
	provingSet := []prooftypes.SectorInfo{{
		SealProof:    sealProofType,
		SectorNumber: sectorNum,
		SealedCID:    sealedCID,
	}}

	// 生成存储证明审计挑战
	randomness := [32]byte{9, 9, 9}
	indicesInProvingSet, err := ffi.GenerateWinningPoStSectorChallenge(winningPostProofType, minerID, randomness[:], uint64(len(provingSet)))
	if err != nil {
		fmt.Println("GenerateWinningPoStSectorChallenge", err.Error())
	}
	var challengedSectors []prooftypes.SectorInfo
	for idx := range indicesInProvingSet {
		challengedSectors = append(challengedSectors, provingSet[indicesInProvingSet[idx]])
	}
	// 生成存储证明
	proofs, err := ffi.GenerateWinningPoSt(minerID, privateInfo, randomness[:])
	if err != nil {
		fmt.Println("GenerateWinningPoSt", err.Error())
	}
	// 验证存储证明
	isValid, err = ffi.VerifyWinningPoSt(prooftypes.WinningPoStVerifyInfo{
		Randomness:        randomness[:],
		Proofs:            proofs,
		ChallengedSectors: challengedSectors,
		Prover:            minerID,
	})
	if err != nil {
		fmt.Println("VerifyWinningPoSt", err.Error())
	}
	log.Println("verify winning post:", isValid)
}

// 508
func PaddleData(data []byte) ([]byte, error) {
	if len(data) < 1016 {
		padding := make([]byte, 1016-len(data))
		data = append(data, padding...)
	}

	// Calculate the next multiple of 508
	paddedSize := ((len(data) + 1015) / 1016) * 1016

	buf := bytes.NewBuffer(data)
	paddedReader, _ := padreader.New(buf, uint64(paddedSize))

	paddedData := make([]byte, paddedSize)
	_, err := io.ReadFull(paddedReader, paddedData)
	if err != nil {
		return nil, err
	}

	return paddedData, nil
}

func requireTempDirPath(prefix string) string {
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		log.Fatalf("temp dirpath err:%v", err)
	}
	fmt.Println(dir)
	return dir
}

func requireTempFile(fileContentsReader io.Reader, size uint64) *os.File {
	file, err := os.CreateTemp("", "")
	if err != nil {
		log.Fatalf("temp file err:%v", err)
	}

	// 预留文件大小
	err = file.Truncate(int64(size))
	if err != nil {
		log.Fatalf("temp file err:%v", err)
	}

	_, err = io.Copy(file, fileContentsReader)
	if err != nil {
		log.Fatalf("temp file err:%v", err)
	}
	// seek to the beginning
	_, err = file.Seek(0, 0)
	if err != nil {
		log.Fatalf("temp file err:%v", err)
	}
	return file
}
