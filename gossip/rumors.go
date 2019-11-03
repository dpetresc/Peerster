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

	gossiper.lAllMsg.mutex.Lock()
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

	if gossiper.lAllMsg.allMsg[origin].GetNextID() <= packet.Rumor.ID && origin != gossiper.Name {
		gossiper.LDsdv.UpdateOrigin(origin, sourceAddrString, packet.Rumor.ID, routeRumor)
		gossiper.lAllMsg.allMsg[origin].AddMessage(packet, packet.Rumor.ID, routeRumor)
		gossiper.SendStatusPacket(sourceAddrString)
		gossiper.lAllMsg.mutex.Unlock()
		// send a copy of packet to random neighbor - can not send to the source of the message
		gossiper.rumormonger(sourceAddrString, packet, false)
	} else {
		// message already seen - still need to ack
		gossiper.SendStatusPacket(sourceAddrString)
		gossiper.lAllMsg.mutex.Unlock()
	}
}

func (gossiper *Gossiper) rumormonger(sourceAddr string, packet *util.GossipPacket, flippedCoin bool) {
	// tu as reçu le message depuis sourceAddr et tu ne veux pas le lui renvoyer
	gossiper.Peers.Mutex.RLock()
	p := gossiper.Peers.ChooseRandomPeer(sourceAddr)
	gossiper.Peers.Mutex.RUnlock()
	if p != "" {
		if flippedCoin {
			fmt.Println("FLIPPED COIN sending rumor to " + p)
		}
		gossiper.sendRumor(p, packet, sourceAddr)
	}
}

func (gossiper *Gossiper) sendRumor(peer string, packet *util.GossipPacket, sourceAddr string) {
	fmt.Println("MONGERING with " + peer)
	gossiper.sendPacketToPeer(peer, packet)
	go gossiper.WaitAck(sourceAddr, peer, packet)
}


