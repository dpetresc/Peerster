package gossip

import (
	"github.com/dpetresc/Peerster/util"
	"math/rand"
	"sync"
	"time"
)

type Ack struct {
	ID         uint32
	ackChannel chan util.StatusPacket
}

type LockAcks struct {
	// peer(IP:PORT) -> Origine -> Ack
	acks map[string]map[string][]Ack
	// Attention always lock lAllMsg first before locking lAcks when we need both
	mutex sync.RWMutex
}

func (gossiper *Gossiper) InitAcks(peer string, packet *util.GossipPacket) {
	gossiper.lAcks.mutex.Lock()
	defer gossiper.lAcks.mutex.Unlock()
	_, ok := gossiper.lAcks.acks[peer]
	if !ok {
		gossiper.lAcks.acks[peer] = make(map[string][]Ack)
	}
	_, ok2 := gossiper.lAcks.acks[peer][packet.Rumor.Origin]
	if !ok2 {
		gossiper.lAcks.acks[peer][packet.Rumor.Origin] = make([]Ack, 0)
	}
}

func (gossiper *Gossiper) addAck(packet *util.GossipPacket, peer string) (chan util.StatusPacket, Ack) {
	// Requires a write lock
	ackChannel := make(chan util.StatusPacket)
	ack := Ack{
		ID:         packet.Rumor.ID,
		ackChannel: ackChannel,
	}
	gossiper.lAcks.acks[peer][packet.Rumor.Origin] = append(gossiper.lAcks.acks[peer][packet.Rumor.Origin], ack)
	return ackChannel, ack
}

func (gossiper *Gossiper) removeAck(peer string, packet *util.GossipPacket, ack Ack) {
	// Requires a write lock
	gossiper.lAcks.mutex.Lock()
	newAcks := make([]Ack, 0)
	for _, ackElem := range gossiper.lAcks.acks[peer][packet.Rumor.Origin] {
		if ackElem != ack {
			newAcks = append(newAcks, ackElem)
		}
	}
	gossiper.lAcks.acks[peer][packet.Rumor.Origin] = newAcks
	gossiper.lAcks.mutex.Unlock()
}

func (gossiper *Gossiper) WaitAck(sourceAddr string, peer string, packet *util.GossipPacket) {
	gossiper.InitAcks(peer, packet)

	gossiper.lAcks.mutex.Lock()
	ackChannel, ack := gossiper.addAck(packet, peer)
	gossiper.lAcks.mutex.Unlock()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	// wait for status packet
	select {
	case sP := <-ackChannel:
		gossiper.lAllMsg.mutex.RLock()
		gossiper.removeAck(peer, packet, ack)
		packetToRumormonger, wantedStatusPacket := gossiper.compareStatuses(sP)
		gossiper.lAllMsg.mutex.RUnlock()

		if packetToRumormonger != nil {
			// we have received a newer packet
			gossiper.sendRumor(peer, packetToRumormonger, "")
			return
		} else if wantedStatusPacket != nil {
			//receiver has newer message than me
			gossiper.sendPacketToPeer(peer, wantedStatusPacket)
			return
		}
		// flip a coin
		if rand.Int()%2 == 0 {
			gossiper.Rumormonger(sourceAddr, packet, true)
		}
	case <-ticker.C:
		gossiper.removeAck(peer, packet, ack)
		gossiper.Rumormonger(sourceAddr, packet, false)
	}
}

func (gossiper *Gossiper) triggerAcks(sP util.StatusPacket, sourceAddrString string) bool {
	var isAck = false
	for _, peerStatus := range sP.Want {
		origin := peerStatus.Identifier
		// sourceAddr
		_, ok := gossiper.lAcks.acks[sourceAddrString][origin]
		if ok {
			for _, ack := range gossiper.lAcks.acks[sourceAddrString][origin] {
				if ack.ID < peerStatus.NextID {
					isAck = true
					ack.ackChannel <- sP
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