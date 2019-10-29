package util

import (
	"fmt"
	"net"
	"strconv"
)

func CheckError(err error) {
	if err != nil {
		//log.Fatal(err)
		fmt.Println(err)
	}
}

func UDPAddrToString(addr *net.UDPAddr) string {
	return addr.IP.String() + ":" + strconv.Itoa(addr.Port)
}
