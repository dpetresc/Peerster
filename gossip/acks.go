package gossip

import (
	"github.com/dpetresc/Peerster/util"
	"math/rand"
	"sync"
	"time"
)

type Ack struct {
	Origin     string
	ID         uint32
}

type LockAcks struct {
	// peer(IP:PORT) -> Ack
	acks map[string]map[Ack]chan util.StatusPacket
	sync.RWMutex
}

func (gossiper *Gossiper) addAck(packet *util.GossipPacket, peer string) (bool, Ack, chan util.StatusPacket) {
	// Requires a write lock
	var ackChannel chan util.StatusPacket = nil
	ack := Ack{
		ID:         packet.Rumor.ID,
		Origin:     packet.Rumor.Origin,
	}
	isNewAck := false
	if _, ok := gossiper.lAcks.acks[peer]; !ok {
		gossiper.lAcks.acks[peer] = make(map[Ack]chan util.StatusPacket)
		isNewAck = true
	} else {
		if _, ok = gossiper.lAcks.acks[peer][ack]; !ok {
			isNewAck = true
		}
	}
	if isNewAck {
		ackChannel = make(chan util.StatusPacket, 100)
		gossiper.lAcks.acks[peer][ack] = ackChannel
	}

	return isNewAck, ack, ackChannel
}

func (gossiper *Gossiper) removeAck(peer string, ack Ack, ackChannel chan util.StatusPacket) {
	// Requires a write lock
	gossiper.lAcks.Lock()
	close(ackChannel)
	delete(gossiper.lAcks.acks[peer], ack)
	gossiper.lAcks.Unlock()
}

func (gossiper *Gossiper) WaitAck(sourceAddr string, peer string, packet *util.GossipPacket) {
	gossiper.lAcks.Lock()
	isNewAck, ack, ackChannel := gossiper.addAck(packet, peer)
	gossiper.lAcks.Unlock()

	if isNewAck {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		// wait for status packet
		select {
		case sP := <-ackChannel:
			gossiper.removeAck(peer, ack, ackChannel)

			gossiper.lAllMsg.RLock()
			packetToRumormonger, wantedStatusPacket := gossiper.compareStatuses(sP)
			gossiper.lAllMsg.RUnlock()

			if packetToRumormonger != nil {
				// we have received a newer packet
				gossiper.sendRumor("", peer, packetToRumormonger)
				return
			} else if wantedStatusPacket != nil {
				//receiver has newer message than me
				gossiper.sendPacketToPeer(peer, wantedStatusPacket)
				return
			}

			// flip a coin
			if rand.Int()%2 == 0 {
				gossiper.rumormonger(sourceAddr, peer, packet, true)
			}
		case <-ticker.C:
			gossiper.removeAck(peer, ack, ackChannel)
			gossiper.rumormonger(sourceAddr,"", packet, false)
		}
	}
}

func (gossiper *Gossiper) triggerAcks(sP util.StatusPacket, sourceAddrString string) bool {
	sP.PrintStatusMessage("")
	var isAck = false
	for _, peerStatus := range sP.Want {
		origin := peerStatus.Identifier
		// sourceAddr
		if _, ok := gossiper.lAcks.acks[sourceAddrString]; ok {
			for ack := range gossiper.lAcks.acks[sourceAddrString] {
				if ack.ID < peerStatus.NextID && ack.Origin == origin {
					isAck = true
					gossiper.lAcks.acks[sourceAddrString][ack] <- sP
				}
			}
		}
	}
	return isAck
}

func (gossiper *Gossiper) checkSenderNewMessage(sP util.StatusPacket) *util.GossipPacket {
	var packetToTransmit *util.GossipPacket = nil
	for origin := range gossiper.lAllMsg.allMsg {
		peerStatusSender := gossiper.lAllMsg.allMsg[origin]
		var foundOrigin = false
		for _, peerStatusReceiver := range sP.Want {
			if peerStatusReceiver.Identifier == origin {
				if peerStatusSender.GetNextID() > peerStatusReceiver.NextID {
					if peerStatusSender.GetNextID() > 1 {
						packetToTransmit = &util.GossipPacket{Rumor:
						peerStatusSender.FindPacketAt(peerStatusReceiver.NextID - 1),
						}
					}
				}
				foundOrigin = true
				break
			}
		}
		if !foundOrigin {
			// the receiver has never received any message from origin yet
			if peerStatusSender.GetNextID() > 1 {
				packetToTransmit = &util.GossipPacket{Rumor:
				peerStatusSender.FindPacketAt(0),
				}
			}
		}
		if packetToTransmit != nil {
			break
		}
	}
	return packetToTransmit
}

func (gossiper *Gossiper) checkReceiverNewMessage(sP util.StatusPacket) *util.GossipPacket {
	var packetToTransmit *util.GossipPacket = nil
	for _, peerStatusReceiver := range sP.Want {
		_, ok := gossiper.lAllMsg.allMsg[peerStatusReceiver.Identifier]
		if !ok ||
			peerStatusReceiver.NextID > gossiper.lAllMsg.allMsg[peerStatusReceiver.Identifier].GetNextID() {
			// gossiper don't have origin node
			if peerStatusReceiver.NextID > 1 {
				packetToTransmit = gossiper.createStatusPacket()
				break
			}
		}
	}
	return packetToTransmit
}

func (gossiper *Gossiper) compareStatuses(sP util.StatusPacket) (*util.GossipPacket, *util.GossipPacket) {
	var packetToRumormonger *util.GossipPacket = nil
	var wantedStatusPacket *util.GossipPacket = nil
	// check if we have received a newer packet
	packetToRumormonger = gossiper.checkSenderNewMessage(sP)
	if packetToRumormonger == nil {
		// check if receiver has newer message than me
		wantedStatusPacket = gossiper.checkReceiverNewMessage(sP)
	}
	return packetToRumormonger, wantedStatusPacket
}
