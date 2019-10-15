package gossip

import (
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/util"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

type LockAllMsg struct {
	allMsg *map[string]*util.PeerReceivedMessages
	mutex  sync.Mutex
}

type Ack struct {
	ID         uint32
	ackChannel chan util.StatusPacket
}

type LockAcks struct {
	// peer(IP:PORT) -> Origine -> Ack
	acks  *map[string]map[string][]Ack
	mutex sync.Mutex
}

type Gossiper struct {
	address *net.UDPAddr
	conn    *net.UDPConn
	name    string
	// change to sync
	peers      *util.Peers
	simple     bool
	clientAddr *net.UDPAddr
	clientConn *net.UDPConn
	lAllMsg    *LockAllMsg
	lAcks      *LockAcks
}

func NewGossiper(clientAddr, address, name, peersStr string, simple bool) *Gossiper {
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
		address:    udpAddr,
		conn:       udpConn,
		name:       name,
		peers:      peers,
		simple:     simple,
		clientAddr: udpClientAddr,
		clientConn: udpClientConn,
		lAllMsg:    &lockAllMsg,
		lAcks:      &lacks,
	}
}

func (gossiper *Gossiper) readClientPacket() *util.Message {
	connection := gossiper.clientConn
	var packet util.Message
	packetBytes := make([]byte, 2048)
	n, _, err := connection.ReadFromUDP(packetBytes)
	util.CheckError(err)
	errDecode := protobuf.Decode(packetBytes[:n], &packet)
	util.CheckError(errDecode)
	return &packet
}

func (gossiper *Gossiper) readGossipPacket() (*util.GossipPacket, *net.UDPAddr) {
	connection := gossiper.conn
	var packet util.GossipPacket
	packetBytes := make([]byte, 2048)
	n, sourceAddr, err := connection.ReadFromUDP(packetBytes)
	// In case simple flag is set, we add manually the RelayPeerAddr of the packets afterwards
	if !gossiper.simple {
		gossiper.peers.AddPeer(util.UDPAddrToString(sourceAddr))
	}
	util.CheckError(err)
	errDecode := protobuf.Decode(packetBytes[:n], &packet)
	util.CheckError(errDecode)

	gossiper.peers.PrintPeers()

	return &packet, sourceAddr
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
	for peer := range *gossiper.peers.PeersMap {
		if peer != source {
			gossiper.sendPacketToPeer(peer, packetToSend)
		}
	}
}

func (gossiper *Gossiper) ListenClient() {
	defer gossiper.clientConn.Close()
	for {
		packet := gossiper.readClientPacket()
		go gossiper.HandleClientPacket(packet)
	}
}

func (gossiper *Gossiper) HandleClientPacket(packet *util.Message) {
	if gossiper.simple {
		// the OriginalName of the message to its own name
		// sets relay peer to its own address
		packetToSend := util.GossipPacket{Simple: &util.SimpleMessage{
			OriginalName:  gossiper.name,
			RelayPeerAddr: util.UDPAddrToString(gossiper.address),
			Contents:      packet.Text,
		}}
		packetToSend.Simple.PrintClientMessage()
		gossiper.sendPacketToPeers("", &packetToSend)
		// if simple flag is set we print the peers
		gossiper.peers.PrintPeers()
	} else {
		gossiper.lAllMsg.mutex.Lock()
		id := (*gossiper.lAllMsg.allMsg)[gossiper.name].GetNextID()
		packetToSend := util.GossipPacket{Rumor: &util.RumorMessage{
			Origin: gossiper.name,
			ID:     id,
			Text:   packet.Text,
		}}
		(*gossiper.lAllMsg.allMsg)[gossiper.name].AddMessage(&packetToSend, id)
		gossiper.lAllMsg.mutex.Unlock()
		go gossiper.Rumormonger("", &packetToSend)
	}
}

func (gossiper *Gossiper) ListenPeers() {
	defer gossiper.conn.Close()

	for {
		packet, sourceAddr := gossiper.readGossipPacket()
		if gossiper.simple {
			if packet.Simple != nil {
				go gossiper.HandleSimplePacket(packet)
			} else {
				log.Fatal("Receive wrong packet format with simple flag !")
			}
		} else if packet.Rumor != nil {
			go gossiper.HandleRumorPacket(packet, sourceAddr)
		} else if packet.Status != nil {
			go gossiper.HandleStatusPacket(packet, sourceAddr)
		} else {
			log.Fatal("Packet contains neither Status nor Rumor and gossiper wasn't used with simple flag !")
		}
	}
}

func (gossiper *Gossiper) HandleRumorPacket(packet *util.GossipPacket, sourceAddr *net.UDPAddr) {
	sourceAddrString := util.UDPAddrToString(sourceAddr)
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
	if (*gossiper.lAllMsg.allMsg)[origin].GetNextID() >= packet.Rumor.ID {
		(*gossiper.lAllMsg.allMsg)[origin].AddMessage(packet, packet.Rumor.ID)
		gossiper.lAllMsg.mutex.Unlock()
		gossiper.SendStatusPacket(sourceAddrString)
		// send a copy of packet to random neighbor - can not send to the source of the message
		gossiper.Rumormonger(sourceAddrString, packet)

	} else {
		gossiper.lAllMsg.mutex.Unlock()
		// message already seen - still need to ack
		gossiper.SendStatusPacket(sourceAddrString)
	}
}

func (gossiper *Gossiper) Rumormonger(sourceAddr string, packet *util.GossipPacket) {
	// tu as reçu le message depuis sourceAddr et tu ne veux pas le lui renvoyer
	p := gossiper.peers.ChooseRandomPeer(sourceAddr)
	if p != "" {
		gossiper.sendRumor(p, packet, sourceAddr)
	}
}

func (gossiper *Gossiper) sendRumor(peer string, packet *util.GossipPacket, sourceAddr string) {
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
		// check if we have received a newer packet
		var packetToTransmit *util.GossipPacket = nil
		for origin := range *gossiper.lAllMsg.allMsg {
			peerStatusSender := (*gossiper.lAllMsg.allMsg)[origin]
			var foundOrigin = false
			for _, peerStatusReceiver := range sP.Want {
				if peerStatusReceiver.Identifier == origin {
					if peerStatusSender.GetNextID() > peerStatusReceiver.NextID {
						packetToTransmit = &util.GossipPacket{Rumor:
						peerStatusSender.FindPacketAt(peerStatusReceiver.NextID - 1),
						}
					}
					foundOrigin = true
					break
				}
			}
			if !foundOrigin {
				// the receiver has never received any message from origin yet
				packetToTransmit = &util.GossipPacket{Rumor:
				peerStatusSender.FindPacketAt(0),
				}
			}
			if packetToTransmit != nil {
				break
			}
		}
		if packetToTransmit != nil {
			// TODO QUESTION WTF JE DOIS REAPPELER MA METHODE ? => RECURSION ?
			// TODO sourceAddr quoi mettre du coup ?!
			gossiper.sendRumor(peer, packetToTransmit, "")
		} else {
			// check if receiver has newer message than me
			for _, peerStatusReceiver := range sP.Want {
				_, ok := (*gossiper.lAllMsg.allMsg)[peerStatusReceiver.Identifier]
				if !ok  ||
					peerStatusReceiver.NextID > (*gossiper.lAllMsg.allMsg)[peerStatusReceiver.Identifier].GetNextID() {
					// gossiper don't have origin node
					packetToTransmit = gossiper.createStatusPacket()
					break
				}
			}
		}

		if packetToTransmit != nil {
			gossiper.sendPacketToPeer(peer, packetToTransmit)
		} else {
			// flip a coin
			if rand.Int() % 2 == 0 {
				gossiper.Rumormonger(sourceAddr, packet)
			}
		}
		gossiper.lAllMsg.mutex.Unlock()
	case <-ticker.C:
		gossiper.lAcks.mutex.Lock()
		newAcks := make([]Ack, 0)
		for _, ackElem := range (*gossiper.lAcks.acks)[peer][packet.Rumor.Origin] {
			if ackElem != ack {
				newAcks = append(newAcks, ackElem)
			}
		}
		(*gossiper.lAcks.acks)[peer][packet.Rumor.Origin] = newAcks
		gossiper.lAcks.mutex.Unlock()
		gossiper.Rumormonger(sourceAddr, packet)
	}
}

func (gossiper *Gossiper) HandleStatusPacket(packet *util.GossipPacket, sourceAddr *net.UDPAddr) {
	sourceAddrString := util.UDPAddrToString(sourceAddr)
	var isAck bool = false

	gossiper.lAcks.mutex.Lock()
	for _, peerStatus := range packet.Status.Want {
		origin := peerStatus.Identifier
		// sourceAddr
		_, ok := (*gossiper.lAcks.acks)[sourceAddrString][origin]
		if ok {
			for _, ack := range (*gossiper.lAcks.acks)[sourceAddrString][origin] {
				// TODO PLUS PETIT OU PLUS PETIT OU EGAL ???!!! JE PENSE PLUS PETIT MAIS BON
				if ack.ID < peerStatus.NextID {
					isAck = true
					ack.ackChannel <- *packet.Status
				}
			}
		}
	}
	gossiper.lAcks.mutex.Unlock()

	// TODO FAIRE CAS I ET II
}

func (gossiper *Gossiper) HandleSimplePacket(packet *util.GossipPacket) {
	packet.Simple.PrintPeerMessage()

	gossiper.peers.AddPeer(packet.Simple.RelayPeerAddr)
	packetToSend := util.GossipPacket{Simple: &util.SimpleMessage{
		OriginalName:  packet.Simple.OriginalName,
		RelayPeerAddr: util.UDPAddrToString(gossiper.address),
		Contents:      packet.Simple.Contents,
	}}
	gossiper.sendPacketToPeers(packet.Simple.RelayPeerAddr, &packetToSend)
}

func (gossiper *Gossiper) createStatusPacket() *util.GossipPacket {
	gossiper.lAllMsg.mutex.Lock()
	defer gossiper.lAllMsg.mutex.Unlock()
	want := make([]util.PeerStatus, 0, len(*gossiper.lAllMsg.allMsg))
	for _, peerRcvMsg := range *gossiper.lAllMsg.allMsg {
		want = append(want, peerRcvMsg.Peer)
	}
	return &util.GossipPacket{Status: &util.StatusPacket{
		Want: want,
	}}
}

func (gossiper *Gossiper) SendStatusPacket(dest string) {
	statusPacket := gossiper.createStatusPacket()
	if dest == "" {
		// Anti-entropy
		p := gossiper.peers.ChooseRandomPeer("")
		if p != "" {
			gossiper.sendPacketToPeer(p, statusPacket)
		}
	} else {
		gossiper.sendPacketToPeer(dest, statusPacket)
	}
}
