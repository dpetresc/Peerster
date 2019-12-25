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

	if packet.TLCMessage.Origin != gossiper.Name {
		if gossiper.hw3ex2 || (gossiper.hw3ex3 && gossiper.ackAll) {
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
				s := fmt.Sprintf("CONFIRMED GOSSIP origin %s ID %d file name %s size %d metahash %s\n",
					packet.TLCMessage.Origin, packet.TLCMessage.ID,
					packet.TLCMessage.TxBlock.Transaction.Name, packet.TLCMessage.TxBlock.Transaction.Size,
					hex.EncodeToString(packet.TLCMessage.TxBlock.Transaction.MetafileHash))
				fmt.Printf(s)
				// GUI
				gossiper.LCurrentPublish.Lock()
				gossiper.LCurrentPublish.GuiMessages = append(gossiper.LCurrentPublish.GuiMessages, s)
				gossiper.LCurrentPublish.Unlock()
				// END GUI
			}
		} else if gossiper.hw3ex3 {
			//check validity
			gossiper.LCurrentPublish.Lock()
			defer gossiper.LCurrentPublish.Unlock()

			gossiper.LCurrentPublish.FutureMsg = append(gossiper.LCurrentPublish.FutureMsg, packet.TLCMessage)

			for _, msg := range gossiper.LCurrentPublish.FutureMsg {
				if gossiper.SatisfyVC(msg) {
					gossiper.LCurrentPublish.RemoveFromFuture(msg)
					if msg.Confirmed == -1 {
						fmt.Printf("UNCONFIRMED GOSSIP origin %s ID %d file name %s size %d metahash %s\n",
							packet.TLCMessage.Origin, packet.TLCMessage.ID,
							packet.TLCMessage.TxBlock.Transaction.Name, packet.TLCMessage.TxBlock.Transaction.Size,
							hex.EncodeToString(packet.TLCMessage.TxBlock.Transaction.MetafileHash))

						if _, ok := gossiper.LCurrentPublish.OtherRounds[msg.Origin]; !ok {
							gossiper.LCurrentPublish.OtherRounds[msg.Origin] = 1
						} else {
							gossiper.LCurrentPublish.OtherRounds[msg.Origin] += 1
						}

						if round, ok := gossiper.LCurrentPublish.OtherRounds[packet.TLCMessage.Origin]; !ok {
							if gossiper.LCurrentPublish.MyRound != 0 {
								return
							}
						} else if round-1 < gossiper.LCurrentPublish.MyRound {
							return
						}

						ack := &util.TLCAck{
							Origin:      gossiper.Name,
							ID:          msg.ID,
							Destination: msg.Origin,
							HopLimit:    uint32(gossiper.hopLimitTLC),
						}

						fmt.Printf("SENDING ACK origin %s ID %d\n", packet.TLCMessage.Origin, packet.TLCMessage.ID)
						gossiper.handleAckPacket(&util.GossipPacket{Ack: ack})

					} else {
						s := fmt.Sprintf("CONFIRMED GOSSIP origin %s ID %d file name %s size %d metahash %s\n",
							packet.TLCMessage.Origin, packet.TLCMessage.ID,
							packet.TLCMessage.TxBlock.Transaction.Name, packet.TLCMessage.TxBlock.Transaction.Size,
							hex.EncodeToString(packet.TLCMessage.TxBlock.Transaction.MetafileHash))
						fmt.Printf(s)
						gossiper.LCurrentPublish.GuiMessages = append(gossiper.LCurrentPublish.GuiMessages, s)

						r := gossiper.LCurrentPublish.OtherRounds[msg.Origin] - 1
						gossiper.LCurrentPublish.Confirmed[r] = append(gossiper.LCurrentPublish.Confirmed[r], Confirmation{
							Origin: msg.Origin,
							ID:     msg.ID,
						})
						gossiper.TryNextRound()
					}
				}
			}
		}

	}
}

func (g *Gossiper) SatisfyVC(msg *util.TLCMessage) bool {
	g.lAllMsg.Lock()
	defer g.lAllMsg.Unlock()

	status := g.createStatusPacket()

	for _, ps := range status.Status.Want {
		for _, psMsg := range msg.VectorClock.Want {
			if ps.Identifier == psMsg.Identifier && ps.NextID < psMsg.NextID {
				return false
			}
		}
	}
	for _, psMsg := range msg.VectorClock.Want {
		inVC := false
		for _, ps := range status.Status.Want {
			if psMsg.Identifier == ps.Identifier {
				inVC = true
			}
		}

		if !inVC {
			return false
		}
	}
	return true
}

func (tm *lockCurrentPublish) RemoveFromFuture(message *util.TLCMessage) {
	for i, msg := range tm.FutureMsg {
		if message == msg {
			tm.FutureMsg[i], tm.FutureMsg[len(tm.FutureMsg)-1] = tm.FutureMsg[len(tm.FutureMsg)-1], tm.FutureMsg[i]
			tm.FutureMsg = tm.FutureMsg[:len(tm.FutureMsg)-1]
			return
		}
	}
}
