package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"runtime"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"
	prooftypes "github.com/filecoin-project/go-state-types/proof"
)

func main() {
	minerID := abi.ActorID(42)

	sectorCacheDirPath := requireTempDirPath("sector-cache-dir")
	defer os.RemoveAll(sectorCacheDirPath)
	stagedSectorFile := requireTempFile(bytes.NewReader([]byte{}), 0)
	defer stagedSectorFile.Close()
	sealedSectorFile := requireTempFile(bytes.NewReader([]byte{}), 0)
	defer sealedSectorFile.Close()

	// 生成一段随机的字节数据
	// someBytes := make([]byte, abi.PaddedPieceSize(2048).Unpadded())
	// io.ReadFull(rand.Reader, someBytes)
	// log.Println("len(someBytes):", len(someBytes))
	// l := len(someBytes)
	// // 将随机数组作为数据分片写入一个临时创建的文件
	// pieceFileA := requireTempFile(bytes.NewReader(someBytes), 508)

	data := []byte("your data here")
	paddedData, err := PaddleData(data)
	if err != nil {
		log.Fatalf("failed to pad data: %v", err)
	}
	log.Println("paddedData:", paddedData)
	pieceFileA := requireTempFile(bytes.NewReader(paddedData), uint64(len(paddedData)))
	// 获取该分片CID
	sealProofType := abi.RegisteredSealProof_StackedDrg2KiBV1
	pieceCIDA, err := ffi.GeneratePieceCIDFromFile(sealProofType, pieceFileA, abi.UnpaddedPieceSize(len(paddedData)))
	if err != nil {
		log.Println("GeneratePieceCIDFromFile Error:", err)
	}
	log.Println("pieceCIDA:", pieceCIDA)
	// _, err := pieceFileA.Seek(0, 0)
	pieceFileA.Seek(0, 0)
	// 将分片写入Sector
	// _, _, err = ffi.WriteWithoutAlignment(sealProofType, pieceFileA, 515, stagedSectorFile)
	ffi.WriteWithoutAlignment(sealProofType, pieceFileA, abi.UnpaddedPieceSize(len(paddedData)), stagedSectorFile)

	// pieceFileB := requireTempFile(bytes.NewReader(someBytes[0:1016]), 1016)
	// pieceCIDB, err := ffi.GeneratePieceCIDFromFile(sealProofType, pieceFileB, 1016)
	// _, err = pieceFileB.Seek(0, 0)
	// _, _, _, err = ffi.WriteWithAlignment(sealProofType, pieceFileB, 1016, stagedSectorFile, []abi.UnpaddedPieceSize{127})
	// publicPieces := []abi.PieceInfo{{
	// 	Size:     abi.UnpaddedPieceSize(127).Padded(),
	// 	PieceCID: pieceCIDA,
	// }, {
	// 	Size:     abi.UnpaddedPieceSize(1016).Padded(),
	// 	PieceCID: pieceCIDB,
	// }}

	// 构建分片的公共信息
	publicPieces := []abi.PieceInfo{{
		Size:     abi.UnpaddedPieceSize(len(paddedData)).Padded(),
		PieceCID: pieceCIDA,
	}}

	// 为sector中的数据分片生成未封装CID
	// _, err = ffi.GenerateUnsealedCID(sealProofType, publicPieces)
	ffi.GenerateUnsealedCID(sealProofType, publicPieces)
	sectorNum := abi.SectorNumber(42)
	ticket := abi.SealRandomness{5, 4, 2}
	seed := abi.InteractiveSealRandomness{7, 4, 2}
	// 预提交封装
	sealPreCommitPhase1Output, _ := ffi.SealPreCommitPhase1(sealProofType, sectorCacheDirPath, stagedSectorFile.Name(), sealedSectorFile.Name(), sectorNum, minerID, ticket, publicPieces)

	log.Println("sealPreCommitPhase1Output:", sealPreCommitPhase1Output)
	sealedCID, unsealedCID, _ := ffi.SealPreCommitPhase2(sealPreCommitPhase1Output, sectorCacheDirPath, sealedSectorFile.Name())
	// 提交封装
	sealCommitPhase1Output, _ := ffi.SealCommitPhase1(sealProofType, sealedCID, unsealedCID, sectorCacheDirPath, sealedSectorFile.Name(), sectorNum, minerID, ticket, seed, publicPieces)
	proof, _ := ffi.SealCommitPhase2(sealCommitPhase1Output, sectorNum, minerID)
	log.Println("sealproof:", proof)
	// 验证封装证明
	isValid, _ := ffi.VerifySeal(prooftypes.SealVerifyInfo{
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
	indicesInProvingSet, _ := ffi.GenerateWinningPoStSectorChallenge(winningPostProofType, minerID, randomness[:], uint64(len(provingSet)))
	var challengedSectors []prooftypes.SectorInfo
	for idx := range indicesInProvingSet {
		challengedSectors = append(challengedSectors, provingSet[indicesInProvingSet[idx]])
	}
	// 生成存储证明
	runtime.LockOSThread()
	proofs, _ := ffi.GenerateWinningPoSt(minerID, privateInfo, randomness[:])
	// 验证存储证明
	var randomnessPost abi.PoStRandomness
	copy(randomnessPost[:], randomness[:])
	isValid, _ = ffi.VerifyWinningPoSt(prooftypes.WinningPoStVerifyInfo{
		Randomness:        randomnessPost,
		Proofs:            proofs,
		ChallengedSectors: challengedSectors,
		Prover:            minerID,
	})
	runtime.UnlockOSThread()
	log.Println("verify winning post:", isValid)
}

func PaddleData(data []byte) ([]byte, error) {
	if len(data) < 127 {
		padding := make([]byte, 127-len(data))
		data = append(data, padding...)
	}

	// Calculate the next multiple of 127
	paddedSize := ((len(data) + 126) / 127) * 127

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
