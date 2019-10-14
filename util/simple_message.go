package util

import "fmt"

type SimpleMessage struct {
	OriginalName string
	RelayPeerAddr string
	Contents string
}

func (clientMessage *SimpleMessage) PrintClientMessage() {
	fmt.Println("CLIENT MESSAGE " + clientMessage.Contents)
}

func (peerMessage *SimpleMessage) PrintPeerMessage() {
	fmt.Println("SIMPLE MESSAGE origin " + peerMessage.OriginalName + " from " + peerMessage.RelayPeerAddr +
		" contents " + peerMessage.Contents)
}