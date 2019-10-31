package file

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/dpetresc/Peerster/util"
	"io"
	"math"
	"os"
)


type MyFile struct {
	fileName string
	fileSize int64
	Metafile *os.File
	fileId string
}

func getInfoFile(f *os.File) (int64, int) {
	fileInfo, err := f.Stat()
	util.CheckError(err)
	fileSizeBytes := fileInfo.Size()
	nbChunks := int(math.Ceil(float64(fileSizeBytes) / float64(util.MaxUDPSize)))
	return fileSizeBytes, nbChunks
}

// Used to either record a chunk of a file or it's metafile
func writeFileToArchive(fileName string, hashes []byte) *os.File {
	path := util.ChunksFolderPath + fileName + ".bin"
	metafile, err := os.Create(path)
	util.CheckError(err)
	_, err3 := metafile.Write(hashes)
	util.CheckError(err3)
	return metafile
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
			writeFileToArchive(fileName, chunk[:n])
			hashes = append(hashes, sha[:]...)
		}
	}
	return fileSizeBytes, hashes
}

func IndexFile(fileName string) *MyFile {
	f, err := os.Open(util.SharedFilesFolderPath + fileName)
	util.CheckError(err)
	defer f.Close()
	fileSizeBytes, hashes := createHashes(f)

	fileIdBytes := sha256.Sum256(hashes)
	fileId := hex.EncodeToString(fileIdBytes[:])
	metafile := writeFileToArchive(fileId, hashes)
	return &MyFile{
		fileName: fileName,
		fileSize: fileSizeBytes,
		Metafile: metafile,
		fileId:   fileId,
	}
}