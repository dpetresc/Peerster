package util

import "fmt"

type RumorMessage struct {
	Origin string
	ID uint32
	Text string
}

func (peerMessage *RumorMessage) PrintRumorMessage(sourceAddr string) {
	fmt.Printf("RUMOR origin %s from %s ID %d contents %s\n", peerMessage.Origin,
		sourceAddr, peerMessage.ID, peerMessage.Text)
}