package gossip

import (
	"fmt"
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/util"
	"math/rand"
	"net"
	"sync"
	"time"
)

type LockPeers struct {
	Peers *util.Peers
	Mutex sync.Mutex
}

type LockAllMsg struct {
	allMsg *map[string]*util.PeerReceivedMessages
	// Attention always lock lAllMsg first before locking lAcks when we need both
	mutex sync.Mutex
}

type Ack struct {
	ID         uint32
	ackChannel chan util.StatusPacket
}

type LockAcks struct {
	// peer(IP:PORT) -> Origine -> Ack
	acks *map[string]map[string][]Ack
	// Attention always lock lAllMsg first before locking lAcks when we need both
	mutex sync.Mutex
}

type Gossiper struct {
	address *net.UDPAddr
	conn    *net.UDPConn
	Name    string
	// change to sync
	LPeers      *LockPeers
	simple      bool
	antiEntropy uint
	ClientAddr  *net.UDPAddr
	ClientConn  *net.UDPConn
	lAllMsg     *LockAllMsg
	lAcks       *LockAcks
	rumorList   []util.RumorMessage
}

func NewGossiper(clientAddr, address, name, peersStr string, simple bool, antiEntropy uint) *Gossiper {
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
		Peer: util.PeerStatus{
			Identifier: name,
			NextID:     1,},
		Received: nil,
	}
	lockAllMsg := LockAllMsg{
		allMsg: &allMsg,
		mutex:  sync.Mutex{},
	}

	acks := make(map[string]map[string][]Ack)
	lacks := LockAcks{
		acks:  &acks,
		mutex: sync.Mutex{},
	}

	return &Gossiper{
		address: udpAddr,
		conn:    udpConn,
		Name:    name,
		LPeers: &LockPeers{
			Peers: peers,
			Mutex: sync.Mutex{},
		},
		simple:      simple,
		antiEntropy: antiEntropy,
		ClientAddr:  udpClientAddr,
		ClientConn:  udpClientConn,
		lAllMsg:     &lockAllMsg,
		lAcks:       &lacks,
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
	gossiper.LPeers.Mutex.Lock()
	for peer := range *gossiper.LPeers.Peers.PeersMap {
		if peer != source {
			gossiper.sendPacketToPeer(peer, packetToSend)
		}
	}
	gossiper.LPeers.Mutex.Unlock()
}

/************************************CLIENT*****************************************/
func (gossiper *Gossiper) readClientPacket() *util.Message {
	connection := gossiper.ClientConn
	var packet util.Message
	packetBytes := make([]byte, 2048)
	n, _, err := connection.ReadFromUDP(packetBytes)
	util.CheckError(err)
	errDecode := protobuf.Decode(packetBytes[:n], &packet)
	util.CheckError(errDecode)
	return &packet
}

func (gossiper *Gossiper) ListenClient() {
	defer gossiper.ClientConn.Close()
	for {
		packet := gossiper.readClientPacket()
		packet.PrintClientMessage()
		go gossiper.HandleClientPacket(packet)
	}
}

func (gossiper *Gossiper) HandleClientPacket(packet *util.Message) {
	if gossiper.simple {
		// the OriginalName of the message to its own Name
		// sets relay peer to its own address
		packetToSend := util.GossipPacket{Simple: &util.SimpleMessage{
			OriginalName:  gossiper.Name,
			RelayPeerAddr: util.UDPAddrToString(gossiper.address),
			Contents:      packet.Text,
		}}
		gossiper.sendPacketToPeers("", &packetToSend)
	} else {
		gossiper.lAllMsg.mutex.Lock()
		id := (*gossiper.lAllMsg.allMsg)[gossiper.Name].GetNextID()
		packetToSend := util.GossipPacket{Rumor: &util.RumorMessage{
			Origin: gossiper.Name,
			ID:     id,
			Text:   packet.Text,
		}}
		(*gossiper.lAllMsg.allMsg)[gossiper.Name].AddMessage(&packetToSend, id)
		gossiper.lAllMsg.mutex.Unlock()
		go gossiper.Rumormonger("", &packetToSend, false)
	}
}

/************************************PEERS*****************************************/
func (gossiper *Gossiper) readGossipPacket() (*util.GossipPacket, *net.UDPAddr) {
	connection := gossiper.conn
	var packet util.GossipPacket
	packetBytes := make([]byte, 2048)
	n, sourceAddr, err := connection.ReadFromUDP(packetBytes)
	// In case simple flag is set, we add manually the RelayPeerAddr of the packets afterwards
	if !gossiper.simple {
		if sourceAddr != gossiper.address {
			gossiper.LPeers.Mutex.Lock()
			gossiper.LPeers.Peers.AddPeer(util.UDPAddrToString(sourceAddr))
			gossiper.LPeers.Mutex.Unlock()
		}
	}
	util.CheckError(err)
	errDecode := protobuf.Decode(packetBytes[:n], &packet)
	util.CheckError(errDecode)

	return &packet, sourceAddr
}

func (gossiper *Gossiper) ListenPeers() {
	defer gossiper.conn.Close()

	// Anti - Entropy
	// in simple mode you can't receive status packets
	// antiEntropy = 0 deactivates the entropy
	if !gossiper.simple && gossiper.antiEntropy != 0 {
		go func() {
			ticker := time.NewTicker(time.Duration(gossiper.antiEntropy) * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					gossiper.LPeers.Mutex.Lock()
					p := gossiper.LPeers.Peers.ChooseRandomPeer("")
					gossiper.LPeers.Mutex.Unlock()
					if p != "" {
						gossiper.lAllMsg.mutex.Lock()
						gossiper.SendStatusPacket(p)
						gossiper.lAllMsg.mutex.Unlock()
					}

				}
			}
		}()
	}

	for {
		packet, sourceAddr := gossiper.readGossipPacket()
		if gossiper.simple {
			if packet.Simple != nil {
				go gossiper.HandleSimplePacket(packet)
			} else {
				//log.Fatal("Receive wrong packet format with simple flag !")
			}
		} else if packet.Rumor != nil {
			go gossiper.HandleRumorPacket(packet, sourceAddr)
		} else if packet.Status != nil {
			go gossiper.HandleStatusPacket(packet, sourceAddr)
		} else {
			//log.Fatal("Packet contains neither Status nor Rumor and gossiper wasn't used with simple flag !")
		}
	}
}

func (gossiper *Gossiper) HandleSimplePacket(packet *util.GossipPacket) {
	packet.Simple.PrintSimpleMessage()
	gossiper.LPeers.Mutex.Lock()
	if packet.Simple.RelayPeerAddr != util.UDPAddrToString(gossiper.address){
		gossiper.LPeers.Peers.AddPeer(packet.Simple.RelayPeerAddr)
	}
	gossiper.LPeers.Peers.PrintPeers()
	gossiper.LPeers.Mutex.Unlock()
	packetToSend := util.GossipPacket{Simple: &util.SimpleMessage{
		OriginalName:  packet.Simple.OriginalName,
		RelayPeerAddr: util.UDPAddrToString(gossiper.address),
		Contents:      packet.Simple.Contents,
	}}
	gossiper.sendPacketToPeers(packet.Simple.RelayPeerAddr, &packetToSend)
}

func (gossiper *Gossiper) HandleRumorPacket(packet *util.GossipPacket, sourceAddr *net.UDPAddr) {
	sourceAddrString := util.UDPAddrToString(sourceAddr)
	packet.Rumor.PrintRumorMessage(sourceAddrString)
	gossiper.LPeers.Mutex.Lock()
	gossiper.LPeers.Peers.PrintPeers()
	gossiper.LPeers.Mutex.Unlock()
	gossiper.lAllMsg.mutex.Lock()
	origin := packet.Rumor.Origin
	_, ok := (*gossiper.lAllMsg.allMsg)[origin]
	if !ok {
		(*gossiper.lAllMsg.allMsg)[origin] = &util.PeerReceivedMessages{
			Peer: util.PeerStatus{
				Identifier: origin,
				NextID:     1,},
			Received: nil,
		}
	}
	if (*gossiper.lAllMsg.allMsg)[origin].GetNextID() <= packet.Rumor.ID {
		(*gossiper.lAllMsg.allMsg)[origin].AddMessage(packet, packet.Rumor.ID)
		gossiper.SendStatusPacket(sourceAddrString)
		gossiper.lAllMsg.mutex.Unlock()
		// send a copy of packet to random neighbor - can not send to the source of the message
		gossiper.Rumormonger(sourceAddrString, packet, false)

	} else {
		gossiper.SendStatusPacket(sourceAddrString)
		gossiper.lAllMsg.mutex.Unlock()
		// message already seen - still need to ack
	}
}

func (gossiper *Gossiper) Rumormonger(sourceAddr string, packet *util.GossipPacket, flippedCoin bool) {
	// tu as reçu le message depuis sourceAddr et tu ne veux pas le lui renvoyer
	gossiper.LPeers.Mutex.Lock()
	p := gossiper.LPeers.Peers.ChooseRandomPeer(sourceAddr)
	gossiper.LPeers.Mutex.Unlock()
	if p != "" {
		if flippedCoin {
			fmt.Println("FLIPPED COIN sending rumor to " + p)
		}
		gossiper.sendRumor(p, packet, sourceAddr)
	}
}

func (gossiper *Gossiper) sendRumor(peer string, packet *util.GossipPacket, sourceAddr string) {
	fmt.Println("MONGERING with " + peer)
	gossiper.sendPacketToPeer(peer, packet)
	go gossiper.WaitAck(sourceAddr, peer, packet)
}

func (gossiper *Gossiper) InitAcks(peer string, packet *util.GossipPacket) {
	gossiper.lAcks.mutex.Lock()
	defer gossiper.lAcks.mutex.Unlock()
	_, ok := (*gossiper.lAcks.acks)[peer]
	if !ok {
		(*gossiper.lAcks.acks)[peer] = make(map[string][]Ack)
	}
	_, ok2 := (*gossiper.lAcks.acks)[peer][packet.Rumor.Origin]
	if !ok2 {
		(*gossiper.lAcks.acks)[peer][packet.Rumor.Origin] = make([]Ack, 0)
	}
}

func (gossiper *Gossiper) WaitAck(sourceAddr string, peer string, packet *util.GossipPacket) {
	gossiper.InitAcks(peer, packet)
	ackChannel := make(chan util.StatusPacket)
	ack := Ack{
		ID:         packet.Rumor.ID,
		ackChannel: ackChannel,
	}
	gossiper.lAcks.mutex.Lock()
	(*gossiper.lAcks.acks)[peer][packet.Rumor.Origin] = append((*gossiper.lAcks.acks)[peer][packet.Rumor.Origin], ack)
	gossiper.lAcks.mutex.Unlock()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	// wait for status packet
	select {
	case sP := <-ackChannel:
		gossiper.lAllMsg.mutex.Lock()
		gossiper.removeAck(peer, packet, ack)
		// check if we have received a newer packet
		packetToTransmit := gossiper.checkSenderNewMessage(sP)
		if packetToTransmit != nil {
			gossiper.sendRumor(peer, packetToTransmit, "")
			gossiper.lAllMsg.mutex.Unlock()
			return
		}
		// check if receiver has newer message than me
		packetToTransmit = gossiper.checkReceiverNewMessage(sP)
		if packetToTransmit != nil {
			gossiper.sendPacketToPeer(peer, packetToTransmit)
			gossiper.lAllMsg.mutex.Unlock()
			return
		}
		gossiper.lAllMsg.mutex.Unlock()
		// flip a coin
		if rand.Int()%2 == 0 {
			gossiper.Rumormonger(sourceAddr, packet, true)
		}
	case <-ticker.C:
		gossiper.removeAck(peer, packet, ack)
		gossiper.Rumormonger(sourceAddr, packet, false)
	}
}

func (gossiper *Gossiper) removeAck(peer string, packet *util.GossipPacket, ack Ack) {
	gossiper.lAcks.mutex.Lock()
	newAcks := make([]Ack, 0)
	for _, ackElem := range (*gossiper.lAcks.acks)[peer][packet.Rumor.Origin] {
		if ackElem != ack {
			newAcks = append(newAcks, ackElem)
		}
	}
	(*gossiper.lAcks.acks)[peer][packet.Rumor.Origin] = newAcks
	gossiper.lAcks.mutex.Unlock()
}

func (gossiper *Gossiper) checkSenderNewMessage(sP util.StatusPacket) *util.GossipPacket {
	var packetToTransmit *util.GossipPacket = nil
	for origin := range *gossiper.lAllMsg.allMsg {
		peerStatusSender := (*gossiper.lAllMsg.allMsg)[origin]
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
		_, ok := (*gossiper.lAllMsg.allMsg)[peerStatusReceiver.Identifier]
		if !ok ||
			peerStatusReceiver.NextID > (*gossiper.lAllMsg.allMsg)[peerStatusReceiver.Identifier].GetNextID() {
			// gossiper don't have origin node
			if peerStatusReceiver.NextID > 1 {
				packetToTransmit = gossiper.createStatusPacket()
				break
			}
		}
	}
	return packetToTransmit
}

func (gossiper *Gossiper) HandleStatusPacket(packet *util.GossipPacket, sourceAddr *net.UDPAddr) {
	sourceAddrString := util.UDPAddrToString(sourceAddr)
	packet.Status.PrintStatusMessage(sourceAddrString)
	gossiper.LPeers.Mutex.Lock()
	gossiper.LPeers.Peers.PrintPeers()
	gossiper.LPeers.Mutex.Unlock()
	var isAck = false

	gossiper.lAllMsg.mutex.Lock()
	defer gossiper.lAllMsg.mutex.Unlock()
	gossiper.lAcks.mutex.Lock()
	defer gossiper.lAcks.mutex.Unlock()

	var packetToTransmit *util.GossipPacket
	var packetToTransmit2 *util.GossipPacket
	// check if we have received a newer packet
	packetToTransmit = gossiper.checkSenderNewMessage(*packet.Status)
	if packetToTransmit == nil {
		// check if receiver has newer message than me
		packetToTransmit2 = gossiper.checkReceiverNewMessage(*packet.Status)
		if packetToTransmit2 == nil {
			fmt.Println("IN SYNC WITH " + sourceAddrString)
		}
	}
	for _, peerStatus := range packet.Status.Want {
		origin := peerStatus.Identifier
		// sourceAddr
		_, ok := (*gossiper.lAcks.acks)[sourceAddrString][origin]
		if ok {
			for _, ack := range (*gossiper.lAcks.acks)[sourceAddrString][origin] {
				if ack.ID < peerStatus.NextID {
					isAck = true
					ack.ackChannel <- *packet.Status
				}
			}
		}
	}
	if !isAck {
		if packetToTransmit != nil {
			// we have received a newer packet
			gossiper.sendRumor(sourceAddrString, packetToTransmit, "")
		} else if packetToTransmit2 != nil {
			//receiver has newer message than me
			gossiper.sendPacketToPeer(sourceAddrString, packetToTransmit)
		}
	}
}

func (gossiper *Gossiper) createStatusPacket() *util.GossipPacket {
	// Attention must acquire lock before using this method
	want := make([]util.PeerStatus, 0, len(*gossiper.lAllMsg.allMsg))
	for _, peerRcvMsg := range *gossiper.lAllMsg.allMsg {
		want = append(want, peerRcvMsg.Peer)
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
