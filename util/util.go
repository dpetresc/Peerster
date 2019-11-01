package util

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
)

var MaxUDPSize int = 8192
var HopLimit uint32 = 10

var SharedFilesFolderPath string
var DownloadsFolderPath string
var ChunksFolderPath string

func CheckError(err error) {
	if err != nil {
		//log.Fatal(err)
		fmt.Println(err)
	}
}

func UDPAddrToString(addr *net.UDPAddr) string {
	return addr.IP.String() + ":" + strconv.Itoa(addr.Port)
}

/********** FOR FILES **********/
func ClearDir(dir string) error {
	names, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entery := range names {
		os.RemoveAll(path.Join([]string{dir, entery.Name()}...))
	}
	return nil
}

func createOrEmptyFolder(folderPath string) {
	if _, err := os.Stat(folderPath); err == nil {
		ClearDir(folderPath)
	} else if os.IsNotExist(err) {
		os.Mkdir(folderPath, 0777)
	}
}

func InitFileFolders() {
	ex, err := os.Executable()
	CheckError(err)
	SharedFilesFolderPath = filepath.Dir(ex) + "/_SharedFiles/"
	if _, err := os.Stat(SharedFilesFolderPath); os.IsNotExist(err) {
		os.Mkdir(SharedFilesFolderPath,0777)
	}

	DownloadsFolderPath = filepath.Dir(ex) + "/​_Downloads/"
	createOrEmptyFolder(DownloadsFolderPath)

	ChunksFolderPath = filepath.Dir(ex) + "/_Chunks/"
	createOrEmptyFolder(ChunksFolderPath)
}

// Used to either record a chunk of a file or it's metafile
func WriteFileToArchive(fileName string, hashes []byte) *os.File {
	path := ChunksFolderPath + fileName + ".bin"
	metafile, err := os.Create(path)
	CheckError(err)
	_, err3 := metafile.Write(hashes)
	CheckError(err3)
	return metafile
}
