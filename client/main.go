package main

import (
	"flag"
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/util"
	"net"
)

var uiPort string
var dest string
var msg string

var clientAddrStr string

func main() {
	flag.StringVar(&uiPort, "UIPort", "8080", "port for the command line interface")
	flag.StringVar(&dest, "dest", "", "destination for the private message")
	flag.StringVar(&msg, "msg", "", "message to be send")

	flag.Parse()

	clientAddrStr = "127.0.0.1:" + uiPort

	packetToSend := util.Message{
		Text: msg,
		Destination: &dest,
	}
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
