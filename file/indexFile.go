package file

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/dpetresc/Peerster/gossip"
	"github.com/dpetresc/Peerster/util"
	"io"
	"math"
	"os"
	"path/filepath"
)

var fileFolderPath string

type MyFile struct {
	fileName string
	fileSize int64
	Metafile *os.File
	fileId string
}

func InitFileFolderPath() {
	ex, err := os.Executable()
	util.CheckError(err)
	fileFolderPath = filepath.Dir(ex) + "/_SharedFiles/"
}

func getInfoFile(f *os.File) (int64, int) {
	fileInfo, err := f.Stat()
	util.CheckError(err)
	fileSizeBytes := fileInfo.Size()
	nbChunks := int(math.Ceil(float64(fileSizeBytes) / float64(gossip.MaxUDPSize)))
	return fileSizeBytes, nbChunks
}

func createHashes(f *os.File) (int64, []byte) {
	fileSizeBytes, nbChunks := getInfoFile(f)

	chunk := make([]byte, gossip.MaxUDPSize)
	hashes := make([]byte, 0, 32*nbChunks)
	for i := int64(0); i < fileSizeBytes; i += int64(gossip.MaxUDPSize) {
		n, err := f.ReadAt(chunk, int64(i))
		if err != nil && err != io.EOF {
			util.CheckError(err)
		} else {
			sha := sha256.Sum256(chunk[:n])
			hashes = append(hashes, sha[:]...)
		}
	}
	return fileSizeBytes, hashes
}

func writeBinaryFile(fileName string, err error, hashes []byte) (string, *os.File) {
	fileIdBytes := sha256.Sum256(hashes)
	fileId := hex.EncodeToString(fileIdBytes[:])
	binaryPath := fileFolderPath + fileId + ".bin"
	metafile, err := os.Create(binaryPath)
	util.CheckError(err)
	_, err3 := metafile.Write(hashes)
	util.CheckError(err3)
	return fileId, metafile
}

func IndexFile(fileName string) *MyFile {
	f, err := os.Open(fileFolderPath + fileName)
	util.CheckError(err)
	defer f.Close()
	fileSizeBytes, hashes := createHashes(f)

	fileId, metafile := writeBinaryFile(fileName, err, hashes)
	return &MyFile{
		fileName: fileName,
		fileSize: fileSizeBytes,
		Metafile: metafile,
		fileId:   fileId,
	}
}