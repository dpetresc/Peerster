package gossip

import (
	"fmt"
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
	fmt.Println("Lock lAcks InitAcks")
	//defer gossiper.lAcks.mutex.Unlock()
	_, ok := gossiper.lAcks.acks[peer]
	if !ok {
		gossiper.lAcks.acks[peer] = make(map[string][]Ack)
	}
	_, ok2 := gossiper.lAcks.acks[peer][packet.Rumor.Origin]
	if !ok2 {
		gossiper.lAcks.acks[peer][packet.Rumor.Origin] = make([]Ack, 0)
	}
	gossiper.lAcks.mutex.Unlock()
	fmt.Println("Unlock Lock lAcks InitAcks")
}

func (gossiper *Gossiper) addAck(packet *util.GossipPacket, peer string) (chan util.StatusPacket, Ack) {
	// Requires a write lock
	ackChannel := make(chan util.StatusPacket, 1)
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
	fmt.Println("Lock lAcks removeAck")
	newAcks := make([]Ack, 0, len(gossiper.lAcks.acks[peer][packet.Rumor.Origin])-1)
	for _, ackElem := range gossiper.lAcks.acks[peer][packet.Rumor.Origin] {
		if ackElem != ack {
			newAcks = append(newAcks, ackElem)
		}
	}
	gossiper.lAcks.acks[peer][packet.Rumor.Origin] = newAcks
	gossiper.lAcks.mutex.Unlock()
	fmt.Println("Unlock Lock lAcks removeAck")
}

func (gossiper *Gossiper) WaitAck(peer string, packet *util.GossipPacket) {
	gossiper.InitAcks(peer, packet)

	gossiper.lAcks.mutex.Lock()
	fmt.Println("Lock lAcks WaitAck")
	ackChannel, ack := gossiper.addAck(packet, peer)
	gossiper.lAcks.mutex.Unlock()
	fmt.Println("Unlock Lock lAcks WaitAck")

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	// wait for status packet
	select {
	case sP := <-ackChannel:
		gossiper.lAllMsg.mutex.RLock()
		fmt.Println("Rlock lAllMsg WaitAck")
		gossiper.removeAck(peer, packet, ack)
		packetToRumormonger, wantedStatusPacket := gossiper.compareStatuses(sP)
		gossiper.lAllMsg.mutex.RUnlock()
		fmt.Println("Unlock Rlock lAllMsg WaitAck")

		if packetToRumormonger != nil {
			// we have received a newer packet
			gossiper.sendRumor(peer, packetToRumormonger)
			return
		} else if wantedStatusPacket != nil {
			//receiver has newer message than me
			fmt.Println("ICI2")
			packet.Rumor.PrintRumorMessage("")
			wantedStatusPacket.Status.PrintStatusMessage("")
			gossiper.sendPacketToPeer(peer, wantedStatusPacket)
			return
		}

		// flip a coin
		if rand.Int()%2 == 0 {
			gossiper.rumormonger(peer, packet, true)
		}
	case <-ticker.C:
		gossiper.rumormonger("", packet, false)
		gossiper.removeAck(peer, packet, ack)
	}
}

func (gossiper *Gossiper) triggerAcks(sP util.StatusPacket, sourceAddrString string) bool {
	var isAck = false
	fmt.Println("TEST")
	for _, peerStatus := range sP.Want {
		fmt.Println("TEST2")
		origin := peerStatus.Identifier
		// sourceAddr
		_, ok := gossiper.lAcks.acks[sourceAddrString][origin]
		if ok {
			for _, ack := range gossiper.lAcks.acks[sourceAddrString][origin] {
				fmt.Println("TEST3")
				if ack.ID < peerStatus.NextID {
					fmt.Println(sourceAddrString, origin, ack.ID)
					isAck = true
					ack.ackChannel <- sP
					fmt.Println(ack.ID)
				}
			}
		}
	}
	fmt.Println("TEST4")
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