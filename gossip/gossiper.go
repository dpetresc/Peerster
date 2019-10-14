package gossip

import (
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/util"
	"log"
	"net"
	"sync"
)

type LockAllMsg struct {
	allMsg *map[string]*util.PeerReceivedMessages
	mutex  sync.Mutex
}

type Gossiper struct {
	address    *net.UDPAddr
	conn       *net.UDPConn
	name       string
	// change to sync
	peers      *util.Peers
	simple     bool
	clientAddr *net.UDPAddr
	clientConn *net.UDPConn

	lAllMsg     *LockAllMsg
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


	return &Gossiper{
		address:    udpAddr,
		conn:       udpConn,
		name:       name,
		peers:      peers,
		simple:     simple,
		clientAddr: udpClientAddr,
		clientConn: udpClientConn,
		lAllMsg:     &lockAllMsg,
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

func (gossiper *Gossiper) sendPacketToPeer(peer string, packetToSend *util.GossipPacket){
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

			p := gossiper.peers.ChooseRandomPeer("")
			if p != ""{
				gossiper.sendPacketToPeer(p, &packetToSend)
				// TODO
			}
		}
	}
}

func (gossiper *Gossiper) ListenPeers() {
	defer gossiper.conn.Close()

	for {
		packet, sourceAddr := gossiper.readGossipPacket()
		if gossiper.simple {
			if packet.Simple != nil {
				packet.Simple.PrintPeerMessage()

				gossiper.peers.AddPeer(packet.Simple.RelayPeerAddr)
				packetToSend := util.GossipPacket{Simple: &util.SimpleMessage{
					OriginalName:  packet.Simple.OriginalName,
					RelayPeerAddr: util.UDPAddrToString(gossiper.address),
					Contents:      packet.Simple.Contents,
				}}
				gossiper.sendPacketToPeers(packet.Simple.RelayPeerAddr, &packetToSend)
			} else {
				log.Fatal("Receive wrong packet format with simple flag !")
			}
		} else if packet.Status != nil {
			// TODO Anti-entropy case
			// If the peer who receives
			//the status message sees a difference in the sets of messages the two nodes know about,
			//that neighbor should either start rumormongering itself or send another status message in
			//response, so that the original node will know which message(s) it needs to send.

		} else if packet.Rumor != nil {
			gossiper.lAllMsg.mutex.Lock()
			// TODO
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
			}
			gossiper.lAllMsg.mutex.Unlock()
			// send a copy of packet to random neighbor - can not send to the source of the message
			p := gossiper.peers.ChooseRandomPeer(util.UDPAddrToString(sourceAddr))
			if p != "" {
				gossiper.sendPacketToPeer(p, packet)
				// TODO
			}


		} else {
			log.Fatal("Packet contains neither Status nor Rumor and gossiper wasn't used with simple flag !")
		}
	}
}

func (gossiper *Gossiper) SendStatusMessage() {
	// TODO

}
