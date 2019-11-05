package gossip

import (
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"math/rand"
	"sync"
	"time"
)

type Ack struct {
	Origin     string
	ID         uint32
	ackChannel chan util.StatusPacket
}

type LockAcks struct {
	// peer(IP:PORT) -> Ack
	acks map[string]map[Ack]bool
	mutex sync.RWMutex
}

func (gossiper *Gossiper) addAck(packet *util.GossipPacket, peer string) (chan util.StatusPacket, Ack) {
	// Requires a write lock
	ackChannel := make(chan util.StatusPacket, 100)
	ack := Ack{
		ID:         packet.Rumor.ID,
		Origin:     packet.Rumor.Origin,
		ackChannel: ackChannel,
	}
	_, ok := gossiper.lAcks.acks[peer]
	if !ok {
		gossiper.lAcks.acks[peer] = make(map[Ack]bool)
	}
	gossiper.lAcks.acks[peer][ack] = true
	return ackChannel, ack
}

func (gossiper *Gossiper) removeAck(peer string, packet *util.GossipPacket, ack Ack) {
	// Requires a write lock
	gossiper.lAcks.mutex.Lock()
	close(ack.ackChannel)
	delete(gossiper.lAcks.acks[peer], ack)
	gossiper.lAcks.mutex.Unlock()
}

func (gossiper *Gossiper) WaitAck(peer string, packet *util.GossipPacket) {
	fmt.Println("Waiting ", packet.Rumor.Origin, packet.Rumor.ID)
	gossiper.lAcks.mutex.Lock()
	ackChannel, ack := gossiper.addAck(packet, peer)
	gossiper.lAcks.mutex.Unlock()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	// wait for status packet
	select {
	case sP := <-ackChannel:
		fmt.Println("Stop waiting ", packet.Rumor.Origin, packet.Rumor.ID)
		gossiper.removeAck(peer, packet, ack)

		gossiper.lAllMsg.mutex.RLock()
		packetToRumormonger, wantedStatusPacket := gossiper.compareStatuses(sP)
		gossiper.lAllMsg.mutex.RUnlock()

		if packetToRumormonger != nil {
			// we have received a newer packet
			gossiper.sendRumor(peer, packetToRumormonger)
			return
		} else if wantedStatusPacket != nil {
			//receiver has newer message than me
			gossiper.sendPacketToPeer(peer, wantedStatusPacket)
			return
		}

		// flip a coin
		if rand.Int()%2 == 0 {
			gossiper.rumormonger(peer, packet, true)
		}
	case <-ticker.C:
		fmt.Println("Timeout waiting ", packet.Rumor.Origin, packet.Rumor.ID)
		gossiper.rumormonger("", packet, false)
		gossiper.removeAck(peer, packet, ack)
		return
	}
}

func (gossiper *Gossiper) triggerAcks(sP util.StatusPacket, sourceAddrString string) bool {
	sP.PrintStatusMessage("")
	var isAck = false
	for _, peerStatus := range sP.Want {
		origin := peerStatus.Identifier
		// sourceAddr
		_, ok := gossiper.lAcks.acks[sourceAddrString]
		if ok {
			for ack := range gossiper.lAcks.acks[sourceAddrString] {
				if ack.ID < peerStatus.NextID && ack.Origin == origin {
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
