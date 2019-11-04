package gossip

import (
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"net"
)

func (gossiper *Gossiper) handleRumorPacket(packet *util.GossipPacket, sourceAddr *net.UDPAddr) {
	sourceAddrString := util.UDPAddrToString(sourceAddr)

	routeRumor := packet.Rumor.Text == ""

	packet.Rumor.PrintRumorMessage(sourceAddrString)

	gossiper.Peers.Mutex.RLock()
	gossiper.Peers.PrintPeers()
	gossiper.Peers.Mutex.RUnlock()

	fmt.Println("ICI3")
	gossiper.lAllMsg.mutex.Lock()
	fmt.Println("Lock lAllMsg handleRumorPacket")
	origin := packet.Rumor.Origin
	_, ok := gossiper.lAllMsg.allMsg[origin]
	if !ok {
		gossiper.lAllMsg.allMsg[origin] = &util.PeerReceivedMessages{
			PeerStatus: util.PeerStatus{
				Identifier: origin,
				NextID:     1,},
			Received: nil,
		}
	}

	fmt.Println("ICI1")
	if gossiper.lAllMsg.allMsg[origin].GetNextID() <= packet.Rumor.ID && origin != gossiper.Name {
		fmt.Println("ICI2")
		gossiper.LDsdv.UpdateOrigin(origin, sourceAddrString, packet.Rumor.ID, routeRumor)
		gossiper.lAllMsg.allMsg[origin].AddMessage(packet, packet.Rumor.ID, routeRumor)
		gossiper.SendStatusPacket(sourceAddrString)
		gossiper.lAllMsg.mutex.Unlock()
		fmt.Println("Unlock Lock lAllMsg handleRumorPacket")
		// send a copy of packet to random neighbor - can not send to the source of the message
		gossiper.rumormonger(sourceAddrString, packet, false)
	} else {
		// message already seen - still need to ack
		gossiper.SendStatusPacket(sourceAddrString)
		gossiper.lAllMsg.mutex.Unlock()
		fmt.Println("Unlock Lock lAllMsg handleRumorPacket")
	}
}

func (gossiper *Gossiper) rumormonger(previousAddr string, packet *util.GossipPacket, flippedCoin bool) {
	// tu as reçu le message depuis sourceAddr et tu ne veux pas le lui renvoyer
	gossiper.Peers.Mutex.RLock()
	p := gossiper.Peers.ChooseRandomPeer(previousAddr)
	gossiper.Peers.Mutex.RUnlock()
	if p != "" {
		if flippedCoin {
			fmt.Println("FLIPPED COIN sending rumor to " + p)
		}
		gossiper.sendRumor(p, packet)
	}
}

func (gossiper *Gossiper) sendRumor(peer string, packet *util.GossipPacket) {
	fmt.Println("MONGERING with " + peer)
	gossiper.sendPacketToPeer(peer, packet)
	fmt.Println("SEND RUMOR TO : ", peer)
	packet.Rumor.PrintRumorMessage("")
	go gossiper.WaitAck(peer, packet)
}


