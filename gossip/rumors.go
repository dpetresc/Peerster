package gossip

import (
	"github.com/dpetresc/Peerster/util"
	"net"
)

func (gossiper *Gossiper) handleRumorPacket(packet *util.GossipPacket, sourceAddr *net.UDPAddr) {
	sourceAddrString := util.UDPAddrToString(sourceAddr)

	routeRumor := packet.Rumor.Text == ""

	packet.Rumor.PrintRumorMessage(sourceAddrString)

	gossiper.Peers.RLock()
	gossiper.Peers.PrintPeers()
	gossiper.Peers.RUnlock()

	gossiper.lAllMsg.Lock()
	origin := packet.Rumor.Origin
	if _, ok := gossiper.lAllMsg.allMsg[origin]; !ok {
		gossiper.lAllMsg.allMsg[origin] = &util.PeerReceivedMessages{
			PeerStatus: util.PeerStatus{
				Identifier: origin,
				NextID:     1,},
			Received: nil,
		}
	}

	if gossiper.lAllMsg.allMsg[origin].GetNextID() <= packet.Rumor.ID && origin != gossiper.Name {
		gossiper.LDsdv.UpdateOrigin(origin, sourceAddrString, packet.Rumor.ID, routeRumor)
		gossiper.lAllMsg.allMsg[origin].AddMessage(packet, packet.Rumor.ID, routeRumor)
		gossiper.SendStatusPacket(sourceAddrString)
		gossiper.lAllMsg.Unlock()
		// send a copy of packet to random neighbor - can not send to the source of the message
		gossiper.rumormonger(sourceAddrString, "", packet, false)
	} else {
		// message already seen - still need to ack
		gossiper.SendStatusPacket(sourceAddrString)
		gossiper.lAllMsg.Unlock()
	}
}

func (gossiper *Gossiper) rumormonger(sourceAddrString string, peerPrevAddr string, packet *util.GossipPacket, flippedCoin bool) {
	// tu as reçu le message depuis sourceAddr et tu ne veux pas le lui renvoyer
	gossiper.Peers.RLock()
	p := gossiper.Peers.ChooseRandomPeer(sourceAddrString, peerPrevAddr)
	gossiper.Peers.RUnlock()
	if p != "" {
		if flippedCoin {
			//fmt.Println("FLIPPED COIN sending rumor to " + p)
		}
		gossiper.sendRumor(sourceAddrString, p, packet)
	}
}

func (gossiper *Gossiper) sendRumor(sourceAddrString string, peer string, packet *util.GossipPacket) {
	//fmt.Println("MONGERING with " + peer)
	gossiper.sendPacketToPeer(peer, packet)
	go gossiper.WaitAck(sourceAddrString, peer, packet)
}


