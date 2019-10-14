package util

import (
	"log"
	"net"
	"strconv"
)

func CheckError(err error) {
	if err  != nil {
		log.Fatal(err)
	}
}

func UDPAddrToString(addr *net.UDPAddr) string {
	return addr.IP.String() + ":" + strconv.Itoa(addr.Port)
}