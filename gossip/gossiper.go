package gossip

import (
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/routing"
	"github.com/dpetresc/Peerster/util"
	"net"
	"sync"
	"time"
)

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
	//files
	lFiles            *LockFiles
	lDownloadingChunk *lockDownloadingChunks
	lCurrentDownloads *lockCurrentDownloading
	lAllChunks        *lockAllChunks
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

	// files
	lFiles := LockFiles{
		Files: make(map[string]*MyFile),
		Mutex: sync.RWMutex{},
	}
	lDownloadingChunk := lockDownloadingChunks{
		currentDownloadingChunks: make(map[DownloadIdentifier]chan util.DataReply),
		mutex: sync.Mutex{},
	}
	lCurrentDownloads := lockCurrentDownloading{
		currentDownloads: make(map[DownloadIdentifier]uint32),
		mutex:            sync.RWMutex{},
	}

	lAllChunks := lockAllChunks{
		chunks: make(map[string][]byte),
		mutex:  sync.RWMutex{},
	}

	return &Gossiper{
		Address:           udpAddr,
		conn:              udpConn,
		Name:              name,
		Peers:             peers,
		simple:            simple,
		antiEntropy:       antiEntropy,
		rtimer:            rtimer,
		ClientAddr:        udpClientAddr,
		ClientConn:        udpClientConn,
		lAllMsg:           &lockAllMsg,
		lAcks:             &lacks,
		LDsdv:             &lDsdv,
		lFiles:            &lFiles,
		lDownloadingChunk: &lDownloadingChunk,
		lCurrentDownloads: &lCurrentDownloads,
		lAllChunks:        &lAllChunks,
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
	packetToSend := gossiper.createNewPacketToSend("", true)
	gossiper.rumormonger("", &packetToSend, false)

	ticker := time.NewTicker(time.Duration(gossiper.rtimer) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			packetToSend := gossiper.createNewPacketToSend("", true)
			gossiper.rumormonger("", &packetToSend, false)
		}
	}
}

// Either for a client message or for a route rumor message (text="")
// also called in clientListener
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
