package gossip

import (
	"encoding/hex"
	"fmt"
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

func (gossiper *Gossiper) handleTLCMessagePacket(packet *util.GossipPacket, sourceAddr *net.UDPAddr) {
	sourceAddrString := util.UDPAddrToString(sourceAddr)
	gossiper.lAllMsg.Lock()
	origin := packet.TLCMessage.Origin
	if _, ok := gossiper.lAllMsg.allMsg[origin]; !ok {
		gossiper.lAllMsg.allMsg[origin] = &util.PeerReceivedMessages{
			PeerStatus: util.PeerStatus{
				Identifier: origin,
				NextID:     1,},
			Received: nil,
		}
	}
	if gossiper.lAllMsg.allMsg[origin].GetNextID() <= packet.TLCMessage.ID && origin != gossiper.Name {
		gossiper.LDsdv.UpdateOrigin(origin, sourceAddrString, packet.TLCMessage.ID, false)
		gossiper.lAllMsg.allMsg[origin].AddMessage(packet, packet.TLCMessage.ID, false)
		gossiper.SendStatusPacket(sourceAddrString)
		gossiper.lAllMsg.Unlock()
		// send a copy of packet to random neighbor - can not send to the source of the message
		gossiper.rumormonger(sourceAddrString, "", packet, false)
	} else {
		// message already seen - still need to ack
		gossiper.SendStatusPacket(sourceAddrString)
		gossiper.lAllMsg.Unlock()
	}

	if packet.TLCMessage.Confirmed == -1 {
		// unconfirmed
		fmt.Printf("UNCONFIRMED GOSSIP origin %s ID %d file name %s size %d metahash %s\n",
			packet.TLCMessage.Origin, packet.TLCMessage.ID,
			packet.TLCMessage.TxBlock.Transaction.Name, packet.TLCMessage.TxBlock.Transaction.Size,
			hex.EncodeToString(packet.TLCMessage.TxBlock.Transaction.MetafileHash))
		// ack
		ack := &util.TLCAck{
			Origin:      gossiper.Name,
			ID:          packet.TLCMessage.ID,
			Destination: packet.TLCMessage.Origin,
			HopLimit:    gossiper.hopLimitTLC,
		}
		fmt.Printf("SENDING ACK origin %s ID %d\n", packet.TLCMessage.Origin, packet.TLCMessage.ID)
		gossiper.handleAckPacket(&util.GossipPacket{Ack: ack})
	} else {
		fmt.Printf("CONFIRMED GOSSIP origin %s ID %d file name %s size %d metahash %s\n",
			packet.TLCMessage.Origin, packet.TLCMessage.ID,
			packet.TLCMessage.TxBlock.Transaction.Name, packet.TLCMessage.TxBlock.Transaction.Size,
			hex.EncodeToString(packet.TLCMessage.TxBlock.Transaction.MetafileHash))
	}
}
