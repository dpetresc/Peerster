package gossip

import (
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"net"
)

func (gossiper *Gossiper) handleStatusPacket(packet *util.GossipPacket, sourceAddr *net.UDPAddr) {
	sourceAddrString := util.UDPAddrToString(sourceAddr)
	packet.Status.PrintStatusMessage(sourceAddrString)
	gossiper.Peers.Mutex.RLock()
	gossiper.Peers.PrintPeers()
	gossiper.Peers.Mutex.RUnlock()

	gossiper.lAllMsg.mutex.RLock()
	fmt.Println("RLock lAllMsg handleStatusPacket")
	//defer fmt.Println("FINISH")
	//defer gossiper.lAllMsg.mutex.RUnlock()
	gossiper.lAcks.mutex.RLock()
	fmt.Println("Lock lAcks handleStatusPacket")
	//defer gossiper.lAcks.mutex.RUnlock()

	fmt.Println("ICI lack")
	packetToRumormonger, wantedStatusPacket := gossiper.compareStatuses(*packet.Status)
	if packetToRumormonger == nil && wantedStatusPacket == nil {
		fmt.Println("IN SYNC WITH " + sourceAddrString)
	}

	fmt.Println("ICI2 lack")
	isAck := gossiper.triggerAcks(*packet.Status, sourceAddrString)
	fmt.Println("ICI3 lack")
	gossiper.lAcks.mutex.RUnlock()
	fmt.Println("Unlock RLock lAllMsg handleStatusPacket")
	gossiper.lAllMsg.mutex.RUnlock()
	fmt.Println("Unlock Lock lAcks handleStatusPacket")

	if !isAck {
		if packetToRumormonger != nil {
			// we have received a newer packet
			gossiper.sendRumor(sourceAddrString, packetToRumormonger)
		} else if wantedStatusPacket != nil {
			//receiver has newer message than me
			gossiper.sendPacketToPeer(sourceAddrString, wantedStatusPacket)
		}
	}
}

func (gossiper *Gossiper) createStatusPacket() *util.GossipPacket {
	// Attention must acquire lock before using this method
	want := make([]util.PeerStatus, 0, len(gossiper.lAllMsg.allMsg))
	for _, peerRcvMsg := range gossiper.lAllMsg.allMsg {
		want = append(want, peerRcvMsg.PeerStatus)
	}
	return &util.GossipPacket{Status: &util.StatusPacket{
		Want: want,
	}}
}

func (gossiper *Gossiper) SendStatusPacket(dest string) {
	// Attention must acquire lock before using this method
	statusPacket := gossiper.createStatusPacket()
	if dest != "" {
		gossiper.sendPacketToPeer(dest, statusPacket)
	}
}
