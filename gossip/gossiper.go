package gossip

import (
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/routing"
	"github.com/dpetresc/Peerster/util"
	"net"
	"sync"
	"time"
)

var hopLimit uint32 = 10
var MaxUDPSize int = 8192

type LockAllMsg struct {
	allMsg map[string]*util.PeerReceivedMessages
	// Attention always lock lAllMsg first before locking lAcks when we need both
	mutex sync.RWMutex
}

type Gossiper struct {
	Address *net.UDPAddr
	conn    *net.UDPConn
	Name    string
	// change to sync
	Peers       *util.Peers
	simple      bool
	antiEntropy int
	rtimer      int
	ClientAddr  *net.UDPAddr
	ClientConn  *net.UDPConn
	lAllMsg     *LockAllMsg
	lAcks       *LockAcks
	// routing
	LDsdv *routing.LockDsdv
}

func NewGossiper(clientAddr, address, name, peersStr string, simple bool, antiEntropy int, rtimer int) *Gossiper {
	udpAddr, err := net.ResolveUDPAddr("udp4", address)
	util.CheckError(err)
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	util.CheckError(err)

	peers := util.NewPeers(peersStr)

	udpClientAddr, err := net.ResolveUDPAddr("udp4", clientAddr)
	util.CheckError(err)
	udpClientConn, err := net.ListenUDP("udp4", udpClientAddr)
	util.CheckError(err)

	allMsg := make(map[string]*util.PeerReceivedMessages)
	allMsg[name] = &util.PeerReceivedMessages{
		PeerStatus: util.PeerStatus{
			Identifier: name,
			NextID:     1,},
		Received: nil,
	}
	lockAllMsg := LockAllMsg{
		allMsg: allMsg,
		mutex:  sync.RWMutex{},
	}

	acks := make(map[string]map[string][]Ack)
	lacks := LockAcks{
		acks:  acks,
		mutex: sync.RWMutex{},
	}

	// routing
	lDsdv := routing.NewDsdv()

	return &Gossiper{
		Address:     udpAddr,
		conn:        udpConn,
		Name:        name,
		Peers:       peers,
		simple:      simple,
		antiEntropy: antiEntropy,
		rtimer:      rtimer,
		ClientAddr:  udpClientAddr,
		ClientConn:  udpClientConn,
		lAllMsg:     &lockAllMsg,
		lAcks:       &lacks,
		LDsdv:       &lDsdv,
	}
}

func (gossiper *Gossiper) sendPacketToPeer(peer string, packetToSend *util.GossipPacket) {
	packetByte, err := protobuf.Encode(packetToSend)
	util.CheckError(err)
	peerAddr, err := net.ResolveUDPAddr("udp4", peer)
	util.CheckError(err)
	_, err = gossiper.conn.WriteToUDP(packetByte, peerAddr)
	util.CheckError(err)
}

func (gossiper *Gossiper) sendPacketToPeers(source string, packetToSend *util.GossipPacket) {
	gossiper.Peers.Mutex.RLock()
	for peer := range gossiper.Peers.PeersMap {
		if peer != source {
			gossiper.sendPacketToPeer(peer, packetToSend)
		}
	}
	gossiper.Peers.Mutex.RUnlock()
}

func (gossiper *Gossiper) AntiEntropy() {
	ticker := time.NewTicker(time.Duration(gossiper.antiEntropy) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			gossiper.Peers.Mutex.RLock()
			p := gossiper.Peers.ChooseRandomPeer("")
			gossiper.Peers.Mutex.RUnlock()
			if p != "" {
				gossiper.lAllMsg.mutex.RLock()
				gossiper.SendStatusPacket(p)
				gossiper.lAllMsg.mutex.RUnlock()
			}

		}
	}
}

func (gossiper *Gossiper) RouteRumors() {
	ticker := time.NewTicker(time.Duration(gossiper.rtimer) * time.Second)
	defer ticker.Stop()

	packetToSend := gossiper.createNewPacketToSend("", true)
	gossiper.Rumormonger("", &packetToSend, false)
	// TODO vérifier rumonger ???
	/*peer := gossiper.Peers.ChooseRandomPeer("")
	if peer != "" {

		gossiper.sendPacketToPeer(peer, &packetToSend)

	}*/

	for {
		select {
		case <-ticker.C:
			packetToSend := gossiper.createNewPacketToSend("", true)
			gossiper.Rumormonger("", &packetToSend, false)
			// TODO vérifier rumonger ???
			/*peer := gossiper.Peers.ChooseRandomPeer("")
			if peer != "" {
				gossiper.sendPacketToPeer(peer, &packetToSend)
			}*/
		}
	}
}

// Either for a client message or for a route rumor message (text="")
func (gossiper *Gossiper) createNewPacketToSend(text string, routeRumor bool) util.GossipPacket {
	gossiper.lAllMsg.mutex.Lock()
	id := gossiper.lAllMsg.allMsg[gossiper.Name].GetNextID()
	packetToSend := util.GossipPacket{Rumor: &util.RumorMessage{
		Origin: gossiper.Name,
		ID:     id,
		Text:   text,
	}}
	gossiper.lAllMsg.allMsg[gossiper.Name].AddMessage(&packetToSend, id, routeRumor)
	gossiper.lAllMsg.mutex.Unlock()
	return packetToSend
}

func (gossiper *Gossiper) createStatusPacket() *util.GossipPacket {
	// Attention must acquire lock before using this method
	want := make([]util.PeerStatus, 0, len(gossiper.lAllMsg.allMsg))
	for _, peerRcvMsg := range gossiper.lAllMsg.allMsg {
		want = append(want, peerRcvMsg.PeerStatus)
	}
	return &util.GossipPacket{Status: &util.StatusPacket{
		Want: want,
	}}
}

func (gossiper *Gossiper) SendStatusPacket(dest string) {
	// Attention must acquire lock before using this method
	statusPacket := gossiper.createStatusPacket()
	if dest != "" {
		gossiper.sendPacketToPeer(dest, statusPacket)
	}
}

func (gossiper *Gossiper) HandlePrivatePacket(packet *util.GossipPacket) {
	// TODO vérifier ?
	if packet.Private.Destination == gossiper.Name {
		packet.Private.PrintPrivateMessage()

		// FOR THE GUI
		routing.AddNewPrivateMessageForGUI(packet.Private.Origin, packet.Private)
	} else {
		nextHop := gossiper.LDsdv.GetNextHopOrigin(packet.Private.Destination)
		// we have the next hop of this origin
		if nextHop != "" {
			hopValue := packet.Private.HopLimit
			if hopValue > 0 {
				packetToForward := &util.GossipPacket{Private: &util.PrivateMessage{
					Origin:      packet.Private.Origin,
					ID:          packet.Private.ID,
					Text:        packet.Private.Text,
					Destination: packet.Private.Destination,
					HopLimit:    hopValue - 1,
				}}
				gossiper.sendPacketToPeer(nextHop, packetToForward)
			}
		}
	}
}
