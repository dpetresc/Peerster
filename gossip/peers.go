package gossip

import (
	"fmt"
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/util"
	"net"
)

/************************************PEERS*****************************************/
func (gossiper *Gossiper) readGossipPacket() (*util.GossipPacket, *net.UDPAddr) {
	connection := gossiper.conn
	var packet util.GossipPacket
	packetBytes := make([]byte, MaxUDPSize)
	n, sourceAddr, err := connection.ReadFromUDP(packetBytes)
	// In case simple flag is set, we add manually the RelayPeerAddr of the packets afterwards
	if !gossiper.simple {
		if sourceAddr != gossiper.Address {
			gossiper.Peers.Mutex.Lock()
			gossiper.Peers.AddPeer(util.UDPAddrToString(sourceAddr))
			gossiper.Peers.Mutex.Unlock()
		}
	}
	util.CheckError(err)
	errDecode := protobuf.Decode(packetBytes[:n], &packet)
	util.CheckError(errDecode)

	return &packet, sourceAddr
}

func (gossiper *Gossiper) ListenPeers() {
	defer gossiper.conn.Close()

	for {
		packet, sourceAddr := gossiper.readGossipPacket()
		if gossiper.simple {
			if packet.Simple != nil {
				go gossiper.HandleSimplePacket(packet)
			} else {
				//log.Fatal("Receive wrong packet format with simple flag !")
			}
		} else if packet.Rumor != nil {
			go gossiper.HandleRumorPacket(packet, sourceAddr)
		} else if packet.Status != nil {
			go gossiper.HandleStatusPacket(packet, sourceAddr)
		} else if packet.Private != nil {
			go gossiper.HandlePrivatePacket(packet)
		} else {
			//log.Fatal("Packet contains neither Status nor Rumor and gossiper wasn't used with simple flag !")
		}
	}
}

func (gossiper *Gossiper) HandleSimplePacket(packet *util.GossipPacket) {
	packet.Simple.PrintSimpleMessage()
	gossiper.Peers.Mutex.Lock()
	if packet.Simple.RelayPeerAddr != util.UDPAddrToString(gossiper.Address) {
		gossiper.Peers.AddPeer(packet.Simple.RelayPeerAddr)
	}
	gossiper.Peers.PrintPeers()
	gossiper.Peers.Mutex.Unlock()
	packetToSend := util.GossipPacket{Simple: &util.SimpleMessage{
		OriginalName:  packet.Simple.OriginalName,
		RelayPeerAddr: util.UDPAddrToString(gossiper.Address),
		Contents:      packet.Simple.Contents,
	}}
	gossiper.sendPacketToPeers(packet.Simple.RelayPeerAddr, &packetToSend)
}

func (gossiper *Gossiper) HandleRumorPacket(packet *util.GossipPacket, sourceAddr *net.UDPAddr) {
	sourceAddrString := util.UDPAddrToString(sourceAddr)

	routeRumor := packet.Rumor.Text == ""

	packet.Rumor.PrintRumorMessage(sourceAddrString)

	// TODO vérifier est-ce que je dois print les peers meme si c'est un route rumor message ?
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
	// TODO vérifier je stocke pas mes propres messages obviously
	if origin != gossiper.Name {
		gossiper.LDsdv.UpdateOrigin(origin, sourceAddrString, packet.Rumor.ID, routeRumor)
	}

	if gossiper.lAllMsg.allMsg[origin].GetNextID() <= packet.Rumor.ID {
		gossiper.lAllMsg.allMsg[origin].AddMessage(packet, packet.Rumor.ID, routeRumor)
		gossiper.SendStatusPacket(sourceAddrString)
		gossiper.lAllMsg.mutex.Unlock()
		// send a copy of packet to random neighbor - can not send to the source of the message
		gossiper.Rumormonger(sourceAddrString, packet, false)
	} else {
		// message already seen - still need to ack
		gossiper.SendStatusPacket(sourceAddrString)
		gossiper.lAllMsg.mutex.Unlock()
	}
}

func (gossiper *Gossiper) HandleStatusPacket(packet *util.GossipPacket, sourceAddr *net.UDPAddr) {
	sourceAddrString := util.UDPAddrToString(sourceAddr)
	packet.Status.PrintStatusMessage(sourceAddrString)
	gossiper.Peers.Mutex.RLock()
	gossiper.Peers.PrintPeers()
	gossiper.Peers.Mutex.RUnlock()

	gossiper.lAllMsg.mutex.RLock()
	defer gossiper.lAllMsg.mutex.RUnlock()
	gossiper.lAcks.mutex.RLock()
	defer gossiper.lAcks.mutex.RUnlock()

	packetToRumormonger, wantedStatusPacket := gossiper.compareStatuses(*packet.Status)
	if packetToRumormonger == nil && wantedStatusPacket == nil {
		fmt.Println("IN SYNC WITH " + sourceAddrString)
	}

	isAck := gossiper.triggerAcks(*packet.Status, sourceAddrString)

	if !isAck {
		if packetToRumormonger != nil {
			// we have received a newer packet
			gossiper.sendRumor(sourceAddrString, packetToRumormonger, "")
		} else if wantedStatusPacket != nil {
			//receiver has newer message than me
			gossiper.sendPacketToPeer(sourceAddrString, wantedStatusPacket)
		}
	}
}

func (gossiper *Gossiper) Rumormonger(sourceAddr string, packet *util.GossipPacket, flippedCoin bool) {
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
