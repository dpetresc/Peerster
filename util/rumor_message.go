package util

import "fmt"

type RumorMessage struct {
	Origin string
	ID uint32
	Text string
}

func (peerMessage *RumorMessage) PrintRumorMessage(sourceAddr string) {
	fmt.Println("RUMOR origin " + peerMessage.Origin + " from " + sourceAddr + " ID " +
		string(peerMessage.ID) + " contents " + peerMessage.Text)
}