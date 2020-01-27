package gossip

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"errors"
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/routing"
	"github.com/dpetresc/Peerster/util"
	"net"
	"net/http"
	"sync"
	"time"
)

type LockAllMsg struct {
	allMsg map[string]*util.PeerReceivedMessages
	// Attention always lock lAllMsg first before locking lAcks when we need both
	sync.RWMutex
}

type LockLastPrivateMsg struct {
	LastPrivateMsg map[string][]*util.PrivateMessage
	sync.RWMutex
}

type LockConsensus struct {
	CAKey           *rsa.PublicKey
	identity        string
	privateKey      *rsa.PrivateKey
	nodesPublicKeys map[string]*rsa.PublicKey
	sync.RWMutex
}

type CAConsensus struct {
	NodesIDPublicKeys map[string]*rsa.PublicKey
	Signature         []byte
}

type Descriptor struct {
	PublicKey []byte
	Identity  []byte
	Signature []byte
}

type Gossiper struct {
	Address *net.UDPAddr
	conn    *net.UDPConn
	Name    string
	// change to sync
	Peers           *util.Peers
	simple          bool
	antiEntropy     int
	rtimer          int
	ClientAddr      *net.UDPAddr
	ClientConn      *net.UDPConn
	lAllMsg         *LockAllMsg
	LLastPrivateMsg *LockLastPrivateMsg
	lAcks           *LockAcks
	// routing
	LDsdv *routing.LockDsdv
	//files
	lFiles            *LockFiles
	lUncompletedFiles *LockUncompletedFiles
	lDownloadingChunk *lockDownloadingChunks
	lCurrentDownloads *lockCurrentDownloading
	lAllChunks        *lockAllChunks
	// search requests
	lRecentSearchRequest *LockRecentSearchRequest
	lSearchMatches       *LockSearchMatches

	// crypto
	secure      bool
	lConsensus  *LockConsensus
	connections *Connections

	// TOR
	lCircuits *LockCircuits
}

func NewGossiper(clientAddr, address, name, peersStr string, simple bool, antiEntropy int,
	rtimer int, secure bool, privateKey *rsa.PrivateKey, CAKey *rsa.PublicKey) *Gossiper {
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
	}

	lockLastPrivateMsg := LockLastPrivateMsg{
		LastPrivateMsg: make(map[string][]*util.PrivateMessage),
	}

	acks := make(map[string]map[Ack]chan util.StatusPacket)
	lacks := LockAcks{
		acks: acks,
	}

	// routing
	lDsdv := routing.NewDsdv()

	// files
	lFiles := LockFiles{
		Files: make(map[string]*MyFile),
	}
	lUncompletedFiles := LockUncompletedFiles{
		IncompleteFiles: make(map[string]map[DownloadIdentifier]*MyFile),
	}
	lDownloadingChunk := lockDownloadingChunks{
		currentDownloadingChunks: make(map[DownloadIdentifier]chan util.DataReply),
	}
	lCurrentDownloads := lockCurrentDownloading{
		currentDownloads: make(map[DownloadIdentifier]uint64),
	}

	lAllChunks := lockAllChunks{
		chunks: make(map[string][]byte),
	}

	// search requests
	lRecentSearchRequest := LockRecentSearchRequest{
		Requests: make(map[searchRequestIdentifier]bool),
	}

	lSearchMatches := LockSearchMatches{
		currNbFullMatch: 0,
		Matches:         make(map[FileSearchIdentifier]*MatchStatus),
	}

	var lConsensus *LockConsensus
	var lCircuits *LockCircuits
	if secure {
		lConsensus = &LockConsensus{
			CAKey:           CAKey,
			identity:        name,
			privateKey:      privateKey,
			nodesPublicKeys: nil,
			RWMutex:         sync.RWMutex{},
		}
		lConsensus.subscribeToConsensus()

		lCircuits = &LockCircuits{
			circuits:         make(map[uint32]*Circuit),
			initiatedCircuit: make(map[uint32]*InitiatedCircuit),
			RWMutex:          sync.RWMutex{},
		}
	} else {
		lConsensus = nil
		lCircuits = nil
	}

	return &Gossiper{
		Address:              udpAddr,
		conn:                 udpConn,
		Name:                 name,
		Peers:                peers,
		simple:               simple,
		antiEntropy:          antiEntropy,
		rtimer:               rtimer,
		ClientAddr:           udpClientAddr,
		ClientConn:           udpClientConn,
		lAllMsg:              &lockAllMsg,
		LLastPrivateMsg:      &lockLastPrivateMsg,
		lAcks:                &lacks,
		LDsdv:                &lDsdv,
		lFiles:               &lFiles,
		lUncompletedFiles:    &lUncompletedFiles,
		lDownloadingChunk:    &lDownloadingChunk,
		lCurrentDownloads:    &lCurrentDownloads,
		lAllChunks:           &lAllChunks,
		lRecentSearchRequest: &lRecentSearchRequest,
		lSearchMatches:       &lSearchMatches,
		secure:               secure,
		lConsensus:           lConsensus,
		connections:          NewConnections(),
		lCircuits:            lCircuits,
	}
}

func (consensus *LockConsensus) subscribeToConsensus() {
	consensus.RLock()
	publicKey := x509.MarshalPKCS1PublicKey(&consensus.privateKey.PublicKey)
	identity := []byte(consensus.identity)
	node := append(publicKey[:], identity[:]...)
	signature := util.SignByteMessage(node, consensus.privateKey)
	consensus.RUnlock()

	descriptor := Descriptor{
		PublicKey: publicKey,
		Identity:  identity,
		Signature: signature,
	}

	buf := new(bytes.Buffer)
	json.NewEncoder(buf).Encode(descriptor)
	r, err := http.Post("http://"+util.CAAddress+"/subscription", "application/json; charset=utf-8", buf)
	util.CheckError(err)
	util.CheckHttpError(r)
}

func (gossiper *Gossiper) getConsensus() {
	r, err := http.Get("http://" + util.CAAddress + "/consensus")
	util.CheckError(err)
	util.CheckHttpError(r)
	var CAResponse CAConsensus
	err = json.NewDecoder(r.Body).Decode(&CAResponse)
	util.CheckError(err)
	r.Body.Close()

	nodesIDPublicKeys, err := json.Marshal(CAResponse.NodesIDPublicKeys)
	gossiper.lConsensus.Lock()
	if !util.VerifySignature(nodesIDPublicKeys, CAResponse.Signature, gossiper.lConsensus.CAKey) {
		err = errors.New("CA corrupted")
		util.CheckError(err)
		return
	}
	gossiper.lConsensus.nodesPublicKeys = CAResponse.NodesIDPublicKeys
	gossiper.lConsensus.Unlock()
}

func (gossiper *Gossiper) Consensus() {
	gossiper.getConsensus()
	ticker := time.NewTicker(time.Duration(util.ConsensusTimerMin) * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			gossiper.getConsensus()
		}
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
	gossiper.Peers.RLock()
	for peer := range gossiper.Peers.PeersMap {
		if peer != source {
			gossiper.sendPacketToPeer(peer, packetToSend)
		}
	}
	gossiper.Peers.RUnlock()
}

func (gossiper *Gossiper) AddNewPrivateMessageForGUI(key string, packet *util.PrivateMessage) {
	gossiper.LLastPrivateMsg.Lock()
	if _, ok := gossiper.LLastPrivateMsg.LastPrivateMsg[key]; !ok {
		gossiper.LLastPrivateMsg.LastPrivateMsg[key] = make([]*util.PrivateMessage, 0)
	}
	gossiper.LLastPrivateMsg.LastPrivateMsg[key] = append(
		gossiper.LLastPrivateMsg.LastPrivateMsg[key], packet)
	gossiper.LLastPrivateMsg.Unlock()
}

func (gossiper *Gossiper) AntiEntropy() {
	ticker := time.NewTicker(time.Duration(gossiper.antiEntropy) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			gossiper.Peers.RLock()
			p := gossiper.Peers.ChooseRandomPeer("", "")
			gossiper.Peers.RUnlock()
			if p != "" {
				gossiper.lAllMsg.RLock()
				gossiper.SendStatusPacket(p)
				gossiper.lAllMsg.RUnlock()
			}

		}
	}
}

func (gossiper *Gossiper) RouteRumors() {
	packetToSend := gossiper.createNewPacketToSend("", true)
	gossiper.rumormonger("", "", &packetToSend, false)

	ticker := time.NewTicker(time.Duration(gossiper.rtimer) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			packetToSend := gossiper.createNewPacketToSend("", true)
			gossiper.rumormonger("", "", &packetToSend, false)
		}
	}
}

// Either for a client message or for a route rumor message (text="")
// also called in clientListener
func (gossiper *Gossiper) createNewPacketToSend(text string, routeRumor bool) util.GossipPacket {
	gossiper.lAllMsg.Lock()
	id := gossiper.lAllMsg.allMsg[gossiper.Name].GetNextID()
	packetToSend := util.GossipPacket{Rumor: &util.RumorMessage{
		Origin: gossiper.Name,
		ID:     id,
		Text:   text,
	}}
	gossiper.lAllMsg.allMsg[gossiper.Name].AddMessage(&packetToSend, id, routeRumor)
	gossiper.lAllMsg.Unlock()
	return packetToSend
}
