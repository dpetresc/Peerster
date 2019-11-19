package util

import (
	"fmt"
	"math/rand"
	"sync"
)

var LastMessagesInOrder []*RumorMessage = make([]*RumorMessage, 0)

type Peers struct {
	PeersMap map[string]bool
	sync.RWMutex
}

type PeerStatus struct {
	Identifier string
	NextID     uint32
}

type PeerReceivedMessages struct {
	PeerStatus PeerStatus
	Received   []*RumorMessage
}

func (peerStatus *PeerStatus) GetPeerStatusAsStr() string {

	return fmt.Sprintf("peer %s nextID %d", peerStatus.Identifier, peerStatus.NextID)
}

func (p *PeerReceivedMessages) AddMessage(packet *GossipPacket, id uint32, routeRumor bool) {
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
	// check if a new message was added
	// don't add route rumor messages so that they won't be display in the gui
	if added  && !routeRumor {
		LastMessagesInOrder = append(LastMessagesInOrder, packet.Rumor)
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
		// TODO remove if it works
		//peersArray = strings.Split(peers, ",")
		peersArray = GetNonEmptyElementsFromString(peers, ",")
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
	if _, ok := peers.PeersMap[addr]; !ok {
		peers.PeersMap[addr] = true
	}
}

func (peers *Peers) PrintPeers() {
	/*var s string = ""
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
	}*/
}

func (peers *Peers) ChooseRandomPeer(sourcePeer string, peer string) string {
	lenMap := len(peers.PeersMap)
	numberToRemove := 0
	if sourcePeer != "" {
		numberToRemove = numberToRemove + 1
	}
	if peer != "" {
		numberToRemove = numberToRemove + 1
	}
	lenMap = lenMap - numberToRemove
	if lenMap > 0 {
		randIndex := rand.Intn(lenMap)
		for k := range peers.PeersMap {
			if k == sourcePeer || k == peer {
				continue
			}
			if randIndex <= 0 {
				return k
			}
			randIndex = randIndex - 1
		}
	}
	return ""
}
