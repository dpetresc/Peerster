package util

import "fmt"

type SimpleMessage struct {
	OriginalName string
	RelayPeerAddr string
	Contents string
}

func PrintClientMessage(clientMessage *SimpleMessage) {
	fmt.Println("CLIENT MESSAGE " + clientMessage.Contents)
}

func PrintPeerMessage(peerMessage *SimpleMessage) {
	fmt.Println("SIMPLE MESSAGE origin " + peerMessage.OriginalName + " from " + peerMessage.RelayPeerAddr +
		" contents " + peerMessage.Contents)
}