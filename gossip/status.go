package gossip

import (
	"github.com/dpetresc/Peerster/util"
	"net"
)

func (gossiper *Gossiper) handleStatusPacket(packet *util.GossipPacket, sourceAddr *net.UDPAddr) {
	sourceAddrString := util.UDPAddrToString(sourceAddr)
	//packet.Status.PrintStatusMessage(sourceAddrString)
	gossiper.Peers.Mutex.RLock()
	gossiper.Peers.PrintPeers()
	gossiper.Peers.Mutex.RUnlock()

	gossiper.lAcks.mutex.Lock()
	isAck := gossiper.triggerAcks(*packet.Status, sourceAddrString)
	gossiper.lAcks.mutex.Unlock()

	gossiper.lAllMsg.mutex.RLock()
	packetToRumormonger, wantedStatusPacket := gossiper.compareStatuses(*packet.Status)
	gossiper.lAllMsg.mutex.RUnlock()

	if packetToRumormonger == nil && wantedStatusPacket == nil {
		//fmt.Println("IN SYNC WITH " + sourceAddrString)
	}

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
