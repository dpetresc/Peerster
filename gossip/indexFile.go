package gossip

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"io"
	"math"
	"os"
	"sync"
)

type LockFiles struct {
	// indexed and downloaded files
	Files map[string]*MyFile
	Mutex sync.RWMutex
}

type MyFile struct {
	fileName string
	fileSize int64
	Metafile [][]byte
	metahash string
}

type lockAllChunks struct {
	chunks map[string][]byte
	mutex sync.RWMutex
}

func getInfoFile(f *os.File) (int64, int) {
	fileInfo, err := f.Stat()
	util.CheckError(err)
	fileSizeBytes := fileInfo.Size()
	nbChunks := int(math.Ceil(float64(fileSizeBytes) / float64(util.MaxUDPSize)))
	return fileSizeBytes, nbChunks
}

func (gossiper *Gossiper) createHashes(f *os.File) (int64, [][]byte, []byte) {
	fileSizeBytes, nbChunks := getInfoFile(f)

	chunkHashes := make([][]byte, 0, nbChunks)
	hashes := make([]byte, 0, 32*nbChunks)
	for i := int64(0); i < fileSizeBytes; i += int64(util.MaxUDPSize) {
		chunk := make([]byte, util.MaxUDPSize)
		n, err := f.ReadAt(chunk, int64(i))
		if err != nil && err != io.EOF {
			util.CheckError(err)
		} else {
			shaBytes := sha256.Sum256(chunk[:n])
			sha := hex.EncodeToString(shaBytes[:])
			gossiper.lAllChunks.mutex.Lock()
			gossiper.lAllChunks.chunks[sha] = chunk[:n]
			gossiper.lAllChunks.mutex.Unlock()
			chunkHashes = append(chunkHashes, shaBytes[:])
			hashes = append(hashes, shaBytes[:]...)
		}
	}
	return fileSizeBytes, chunkHashes, hashes
}

func (gossiper *Gossiper) IndexFile(fileName string) *MyFile {
	f, err := os.Open(util.SharedFilesFolderPath + fileName)
	util.CheckError(err)
	defer f.Close()
	fileSizeBytes, chunkHashes, hashes := gossiper.createHashes(f)

	fileIdBytes := sha256.Sum256(hashes)
	metahash := hex.EncodeToString(fileIdBytes[:])

	gossiper.lAllChunks.mutex.Lock()
	gossiper.lAllChunks.chunks[metahash] = hashes
	gossiper.lAllChunks.mutex.Unlock()

	myFile := &MyFile{
		fileName: fileName,
		fileSize: fileSizeBytes,
		Metafile: chunkHashes,
		metahash: metahash,
	}
	fmt.Println("Metahash : " + metahash)
	gossiper.lFiles.Mutex.Lock()
	gossiper.lFiles.Files[metahash] = myFile
	gossiper.lFiles.Mutex.Unlock()
	return myFile
}