package util

import (
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
)

var MaxUDPSize int = 8192
var HopLimit uint32 = 10

var SharedFilesFolderPath string
var DownloadsFolderPath string
var ChunksFolderPath string

func CheckError(err error) {
	if err != nil {
		//log.Fatal(err)
		panic(err)
	}
}

func CheckHttpError(r *http.Response) {
	if r.StatusCode != 200 {
		b, _ := ioutil.ReadAll(r.Body)
		log.Fatal(string(b))
	}
}

func UDPAddrToString(addr *net.UDPAddr) string {
	return addr.IP.String() + ":" + strconv.Itoa(addr.Port)
}

func GetNonEmptyElementsFromString(s string, separator string) []string {
	elementArray := strings.Split(s, separator)
	nonEmptyElementArray := make([]string, 0, len(elementArray))
	for _, elem := range elementArray {
		if elem != "" {
			nonEmptyElementArray = append(nonEmptyElementArray, elem)
		}
	}
	return nonEmptyElementArray
}


/*
 *	check if a slice contains an item
 */
func SliceContains(slice interface{}, item interface{}) bool {
	s := reflect.ValueOf(slice)

	if s.Kind() != reflect.Slice {
		panic("Invalid data-type")
	}

	for i := 0; i < s.Len(); i++ {
		if s.Index(i).Interface() == item {
			return true
		}
	}

	return false
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
	SharedFilesFolderPath = "./_SharedFiles/"
	if _, err := os.Stat(SharedFilesFolderPath); os.IsNotExist(err) {
		os.Mkdir(SharedFilesFolderPath,0777)
	}

	DownloadsFolderPath = "./_Downloads/"
	if _, err := os.Stat(DownloadsFolderPath); os.IsNotExist(err) {
		os.Mkdir(DownloadsFolderPath,0777)
	}
}
