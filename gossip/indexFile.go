package gossip

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/dpetresc/Peerster/util"
	"io"
	"math"
	"os"
	"sync"
)

type LockIndexedFile struct {
	IndexedFiles map[string]*MyFile
	Mutex sync.RWMutex
}

type MyFile struct {
	fileName string
	fileSize int64
	Metafile *os.File
	metahash string
}

func getInfoFile(f *os.File) (int64, int) {
	fileInfo, err := f.Stat()
	util.CheckError(err)
	fileSizeBytes := fileInfo.Size()
	nbChunks := int(math.Ceil(float64(fileSizeBytes) / float64(util.MaxUDPSize)))
	return fileSizeBytes, nbChunks
}

func createHashes(f *os.File) (int64, []byte) {
	fileSizeBytes, nbChunks := getInfoFile(f)

	chunk := make([]byte, util.MaxUDPSize)
	hashes := make([]byte, 0, 32*nbChunks)
	for i := int64(0); i < fileSizeBytes; i += int64(util.MaxUDPSize) {
		n, err := f.ReadAt(chunk, int64(i))
		if err != nil && err != io.EOF {
			util.CheckError(err)
		} else {
			sha := sha256.Sum256(chunk[:n])
			fileName := hex.EncodeToString(sha[:])
			util.WriteFileToArchive(fileName, chunk[:n])
			hashes = append(hashes, sha[:]...)
		}
	}
	return fileSizeBytes, hashes
}

func (gossiper *Gossiper) IndexFile(fileName string) *MyFile {
	f, err := os.Open(util.SharedFilesFolderPath + fileName)
	util.CheckError(err)
	defer f.Close()
	fileSizeBytes, hashes := createHashes(f)

	fileIdBytes := sha256.Sum256(hashes)
	metahash := hex.EncodeToString(fileIdBytes[:])
	metafile := util.WriteFileToArchive(metahash, hashes)
	myFile := &MyFile{
		fileName: fileName,
		fileSize: fileSizeBytes,
		Metafile: metafile,
		metahash: metahash,
	}
	gossiper.lIndexed.Mutex.Lock()
	gossiper.lIndexed.IndexedFiles[metahash] = myFile
	gossiper.lIndexed.Mutex.Unlock()
	return myFile
}