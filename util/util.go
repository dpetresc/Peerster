package util

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
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
		// comment it for the tests
		ClearDir(folderPath)
	} else if os.IsNotExist(err) {
		os.Mkdir(folderPath, 0777)
	}
}

func InitFileFolders() {
	SharedFilesFolderPath = "./_SharedFiles/"
	if _, err := os.Stat(SharedFilesFolderPath); os.IsNotExist(err) {
		os.Mkdir(SharedFilesFolderPath,0777)
	}

	DownloadsFolderPath = "./_Downloads/"
	if _, err := os.Stat(DownloadsFolderPath); os.IsNotExist(err) {
		os.Mkdir(DownloadsFolderPath,0777)
	}
}
