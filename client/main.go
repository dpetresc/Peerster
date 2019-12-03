package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/util"
	"net"
	"os"
)

var uiPort string
var dest string
var msg string
var file string
var request string
var keywords string
var budget int64

var clientAddrStr string

func init() {
	flag.StringVar(&uiPort, "UIPort", "8080", "port for the command line interface")
	flag.StringVar(&dest, "dest", "", "destination for the private message")
	flag.StringVar(&msg, "msg", "", "message to be send")
	flag.StringVar(&file, "file", "", "file to be indexed by the gossiper")
	flag.StringVar(&request, "request", "", "​file hexadecimal MetaHash")
	flag.StringVar(&keywords, "keywords", "", "keywords for the file search")
	flag.Int64Var(&budget, "budget", -1, "budget for the file search (optional)")

	flag.Parse()
}

func logBadArgumentCombination(){
	fmt.Println("​ERROR (Bad argument combination)​")
	os.Exit(1)
}

func logBadRequestHashFormat(){
	fmt.Println("ERROR (Unable to decode hex hash)")
	os.Exit(1)
}

func isCorrectArgumentCombination() bool{
	// Flags: dest + msg + file + request + keywords + budget

	// hw2 exercise 3 combination: (dest + msg) ou msg
	if (dest != "" && msg != "" && file == "" && request == "" && keywords == "" && budget == -1) ||
		(dest == "" && msg != "" && file == "" && request == "" && keywords == "" && budget == -1){
		return true
	}

	// hw2 exercise 4 combination: index file
	if dest == "" && msg == "" && file != "" && request == "" && keywords == "" && budget == -1{
		return true
	}

	// hw2 exercise 6 combination: dest + file + request
	if dest != "" && msg == "" && file != "" && request != "" && keywords == "" && budget == -1{
		return true
	}

	// hw3 search request
	if dest == "" && msg == "" && file == "" && request == "" && keywords != "" {
		return true
	}
	// hw3 download previous search requested file
	if dest == "" && msg == "" && file != "" && request != "" && keywords == "" && budget == -1{
		return true
	}
	return false
}

func isCorrectRequestHashFormat() bool{
	if request != ""{
		bytes, err := hex.DecodeString(request)
		if err != nil {
			return false
		}
		if len(bytes) != 32{
			return false
		}
	}
	return true
}

func checkClientFlags() {
	if(!isCorrectArgumentCombination()){
		logBadArgumentCombination()
	}
	if(!isCorrectRequestHashFormat()){
		logBadRequestHashFormat()
	}
}

func dealWithEmptyField(packetToSend *util.Message, requestBytes []byte) {
	if dest == "" {
		packetToSend.Destination = nil
	}
	if keywords == "" {
		packetToSend.Keywords = nil
	}
	if len(requestBytes) == 0 {
		packetToSend.Request = nil
	}
	if file == "" {
		packetToSend.File = nil
	}
}

func main() {
	checkClientFlags()

	clientAddrStr = "127.0.0.1:" + uiPort

	requestBytes, _ := hex.DecodeString(request)

	packetToSend := util.Message{
		Text: msg,
		Destination: &dest,
		File: &file,
		Request: &requestBytes,
		Keywords: &keywords,
	}
	if budget != -1 {
		uintBudget := uint64(budget)
		packetToSend.Budget = &uintBudget
	}

	dealWithEmptyField(&packetToSend, requestBytes)

	packetByte, err := protobuf.Encode(&packetToSend)
	util.CheckError(err)

	clientAddr, err := net.ResolveUDPAddr("udp4", clientAddrStr)
	util.CheckError(err)
	clientConn, err := net.DialUDP("udp4", nil, clientAddr)
	util.CheckError(err)
	defer clientConn.Close()

	_, err = clientConn.Write(packetByte)
	util.CheckError(err)
}
