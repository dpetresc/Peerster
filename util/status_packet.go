package util

import "fmt"

type StatusPacket struct {
	Want []PeerStatus
}

func (peerMessage *StatusPacket) PrintStatusMessage(sourceAddr string) {
	var s string = ""
	s += fmt.Sprintf("STATUS from %s ", sourceAddr)
	for _, peer := range peerMessage.Want[:len(peerMessage.Want)-1] {
		s += peer.getPeerStatusAsStr()
		s += " "
	}
	s += peerMessage.Want[len(peerMessage.Want)-1].getPeerStatusAsStr()
	fmt.Println(s)
}

