package util

import "fmt"

type SimpleMessage struct {
	OriginalName  string
	RelayPeerAddr string
	Contents      string
}

func (peerMessage *SimpleMessage) PrintSimpleMessage() {
	fmt.Println("SIMPLE MESSAGE origin " + peerMessage.OriginalName + " from " + peerMessage.RelayPeerAddr +
		" contents " + peerMessage.Contents)
}
