package util

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
)

var AllMessagesInOrder []RumorMessage = make([]RumorMessage, 0)

type Peers struct {
	PeersMap map[string]bool
	Mutex sync.RWMutex
}

type PeerStatus struct {
	Identifier string
	NextID     uint32
}

type PeerReceivedMessages struct {
	PeerStatus PeerStatus
	Received   []*RumorMessage
}

func (peerStatus *PeerStatus) getPeerStatusAsStr() string {

	return fmt.Sprintf("peer %s nextID %d", peerStatus.Identifier, peerStatus.NextID)
}

func (p *PeerReceivedMessages) AddMessage(packet *GossipPacket, id uint32) {
	// Requires a write lock
	var added bool = false
	if int(id) == (len(p.Received) + 1) {
		p.Received = append(p.Received, packet.Rumor)
		added = true
	} else if int(id) <= len(p.Received) {
		if p.Received[(int(id) - 1)] != nil {
			added = true
		}
		p.Received[(int(id) - 1)] = packet.Rumor
	} else if int(id) > len(p.Received) {
		nbToAdd := int(id) - len(p.Received) - 1
		for i := 0; i < nbToAdd; i++ {
			p.Received = append(p.Received, nil)
		}
		p.Received = append(p.Received, packet.Rumor)
		added = true
	}
	if added {
		AllMessagesInOrder = append(AllMessagesInOrder, *packet.Rumor)
	}
	p.setNextID(p.findNextID())
}

func (p *PeerReceivedMessages) findNextID() uint32 {
	var firstNil = uint32(len(p.Received))
	for i := 0; i < len(p.Received); i++ {
		if p.Received[i] == nil {
			firstNil = uint32(i)
			break
		}
	}
	return firstNil + 1
}

func (p *PeerReceivedMessages) FindPacketAt(index uint32) *RumorMessage {
	return p.Received[int(index)]
}

func (p *PeerReceivedMessages) GetNextID() uint32 {
	return p.PeerStatus.NextID
}

func (p *PeerReceivedMessages) setNextID(id uint32) {
	p.PeerStatus.NextID = id
}

func NewPeers(peers string) *Peers {
	var peersArray []string
	if peers != "" {
		peersArray = strings.Split(peers, ",")
	} else {
		peersArray = []string{}
	}
	peersMap := make(map[string]bool)
	for i := 0; i < len(peersArray); i += 1 {
		peersMap[peersArray[i]] = true
	}
	return &Peers{
		PeersMap: peersMap,
	}
}

func (peers *Peers) AddPeer(addr string) {
	// Requires a write lock
	_, ok := peers.PeersMap[addr]
	if !ok {
		peers.PeersMap[addr] = false
	}
}

func (peers *Peers) PrintPeers() {
	var s string = ""
	if len(peers.PeersMap) > 0 {
		s += "PEERS "
		keys := make([]string, 0, len(peers.PeersMap))
		for k := range peers.PeersMap {
			keys = append(keys, k)
		}
		for _, peer := range keys[:len(keys)-1] {
			s = s + peer + ","
		}
		s += keys[len(keys)-1]
		fmt.Println(s)
	}
}

func (peers *Peers) ChooseRandomPeer(sourcePeer string) string {
	lenMap := len(peers.PeersMap)
	keys := make([]string, 0, lenMap)

	for k := range peers.PeersMap {
		if k != sourcePeer {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return ""
	} else {
		min := 0
		max := len(keys) - 1
		randIndex := rand.Intn(max-min+1) + min
		return keys[randIndex]
	}
}
