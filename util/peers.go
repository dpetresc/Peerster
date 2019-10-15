package util

import (
	"fmt"
	"math/rand"
	"strings"
)

type Peers struct {
	PeersMap *map[string]bool
}

type PeerStatus struct {
	Identifier string
	NextID     uint32
}

type PeerReceivedMessages struct {
	Peer     PeerStatus
	Received []*RumorMessage
}

func (p *PeerReceivedMessages) AddMessage(packet *GossipPacket, id uint32) {
	if int(id) == (len(p.Received) + 1) {
		p.Received = append(p.Received, packet.Rumor)
	} else if int(id) <= len(p.Received) {
		p.Received[(int(id) - 1)] = packet.Rumor
	} else if int(id) > len(p.Received) {
		nbToAdd := int(id) - len(p.Received) - 1
		for i := 0; i < nbToAdd; i++ {
			p.Received = append(p.Received, nil)
		}
		p.Received = append(p.Received, packet.Rumor)
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
	return p.Peer.NextID
}

func (p *PeerReceivedMessages) setNextID(id uint32) {
	p.Peer.NextID = id
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
		PeersMap: &peersMap,
	}
}

func (peers *Peers) AddPeer(addr string) {
	_, ok := (*peers.PeersMap)[addr]
	if !ok {
		(*peers.PeersMap)[addr] = false
	}
}

func (peers *Peers) PrintPeers() {
	if len(*peers.PeersMap) > 0 {
		fmt.Print("PEERS ")
		keys := make([]string, 0, len(*peers.PeersMap))
		for k := range *peers.PeersMap {
			keys = append(keys, k)
		}
		for _, peer := range keys[:len(keys)-1] {
			fmt.Print(peer + ",")
		}
		fmt.Println(keys[len(keys)-1])
	}
}

func (peers *Peers) ChooseRandomPeer(sourcePeer string) string {
	lenMap := len(*peers.PeersMap)
	keys := make([]string, 0, lenMap)

	for k := range *peers.PeersMap {
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
