package gossip

import (
	"log"
	"net"
	"github.com/dpetresc/Peerster/util"
	"github.com/dedis/protobuf"
)

type Gossiper struct {
	address 		*net.UDPAddr
	conn    		*net.UDPConn
	name    		string
	peers 			*util.Peers
	simple 			bool
	clientAddr		*net.UDPAddr
	clientConn 		*net.UDPConn

	vector_clock 	[]util.PeerStatus
	seq_num 		uint32
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

	seq_num := 1


	return &Gossiper{
		address:  	  	udpAddr,
		conn:     	  	udpConn,
		name:     	  	name,
		peers: 		  	peers,
		simple:   	  	simple,
		clientAddr:   	udpClientAddr,
		clientConn:   	udpClientConn,
		seq_num: 	  	uint32(seq_num),
	}
}

func readClientPacket(connection *net.UDPConn) *util.Message{
	var packet util.Message
	packetBytes := make([]byte, 2048)
	n, _, err := connection.ReadFromUDP(packetBytes)
	util.CheckError(err)
	errDecode := protobuf.Decode(packetBytes[:n], &packet)
	util.CheckError(errDecode)
	return &packet
}

func readGossipPacket(connection *net.UDPConn) *util.GossipPacket{
	var packet util.GossipPacket
	packetBytes := make([]byte, 2048)
	n, _, err := connection.ReadFromUDP(packetBytes)
	util.CheckError(err)
	errDecode := protobuf.Decode(packetBytes[:n], &packet)
	util.CheckError(errDecode)
	return &packet
}

func (gossiper *Gossiper) sendPacketToPeers(source string, packetToSend *util.GossipPacket) {
	packetByte, err := protobuf.Encode(packetToSend)
	util.CheckError(err)
	for peer := range (*gossiper.peers.PeersMap) {
		if peer != source{
			peerAddr, err := net.ResolveUDPAddr("udp4", peer)
			util.CheckError(err)
			_, err = gossiper.conn.WriteToUDP(packetByte, peerAddr)
			util.CheckError(err)
		}
	}
	gossiper.peers.PrintPeers()
}

func (gossiper *Gossiper) ListenClient(){
	defer gossiper.clientConn.Close()

	for {
		packet := readClientPacket(gossiper.clientConn)
		if gossiper.simple  {
			// the OriginalName of the message to its own name
			// sets relay peer to its own address
			packetToSend := util.GossipPacket{Simple: &util.SimpleMessage{
				OriginalName:  gossiper.name,
				RelayPeerAddr: util.UDPAddrToString(gossiper.address),
				Contents:      packet.Text,
			}}
			packetToSend.Simple.PrintClientMessage()
			gossiper.sendPacketToPeers("", &packetToSend)
		} else {
			// TODO
		}
	}
}

func (gossiper *Gossiper) ListenPeers(){
	defer gossiper.conn.Close()

	for {
		var packet util.GossipPacket = *readGossipPacket(gossiper.conn)
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

		} else if packet.Rumor != nil {
			// TODO
		} else {
			log.Fatal("Packet contains neither Status nor Rumor and gossiper wasn't used with simple flag !")
		}
	}
}