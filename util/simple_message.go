package util

import "fmt"

type SimpleMessage struct {
	OriginalName  string
	RelayPeerAddr string
	Contents      string
}

func (peerMessage *SimpleMessage) PrintSimpleMessage() {
	fmt.Printf("SIMPLE MESSAGE origin %s from %s contents %s\n", peerMessage.OriginalName,
		peerMessage.RelayPeerAddr, peerMessage.Contents)
}
