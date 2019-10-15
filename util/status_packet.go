package util

import "fmt"

type StatusPacket struct {
	Want []PeerStatus
}

/* STATUS from <relay_addr> peer <name1> nextID <next_ID1> peer
<name2> nextID <next_ID2>

 */

func (peerMessage *StatusPacket) PrintStatusMessage(sourceAddr string) {
	fmt.Print("STATUS from " + sourceAddr + " ")
	for _, peer := range peerMessage.Want[:len(peerMessage.Want)-1] {
		peer.printPeerStatus()
		fmt.Print(" ")
	}
	peerMessage.Want[len(peerMessage.Want)-1].printPeerStatus()
	fmt.Println()
}

