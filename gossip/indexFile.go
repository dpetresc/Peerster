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
	// indexed and successfully downloaded files
	// fileName => MyFile
	Files map[string]*MyFile
	sync.RWMutex
}

type MyFile struct {
	fileName string
	fileSize int64
	Metafile [][]byte
	metahash string
	nbChunks uint64
}

type lockAllChunks struct {
	chunks map[string][]byte
	sync.RWMutex
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
		if n, err := f.ReadAt(chunk, int64(i)); err != nil && err != io.EOF {
			util.CheckError(err)
		} else {
			shaBytes := sha256.Sum256(chunk[:n])
			sha := hex.EncodeToString(shaBytes[:])
			gossiper.lAllChunks.Lock()
			gossiper.lAllChunks.chunks[sha] = chunk[:n]
			gossiper.lAllChunks.Unlock()
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

	gossiper.lAllChunks.Lock()
	gossiper.lAllChunks.chunks[metahash] = hashes
	gossiper.lAllChunks.Unlock()

	myFile := &MyFile{
		fileName: fileName,
		fileSize: fileSizeBytes,
		Metafile: chunkHashes,
		metahash: metahash,
		nbChunks: uint64(len(chunkHashes)),
	}
	fmt.Println("Metahash : " + metahash)
	gossiper.lFiles.Lock()
	gossiper.lFiles.Files[fileName] = myFile
	gossiper.lFiles.Unlock()
	return myFile
}