package gossip

import (
	"net"
	"github.com/dpetresc/Peerster/util"
	"github.com/dedis/protobuf"
)

type Gossiper struct {
	address 	*net.UDPAddr
	conn    	*net.UDPConn
	name    	string
	peers 		*util.Peers
	simple 		bool
	clientAddr	*net.UDPAddr
	clientConn 	*net.UDPConn
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

	return &Gossiper{
		address:  	udpAddr,
		conn:     	udpConn,
		name:     	name,
		peers: 		peers,
		simple:   	simple,
		clientAddr: udpClientAddr,
		clientConn: udpClientConn,
	}
}

func (gossiper *Gossiper) ListenClient(){
	defer gossiper.clientConn.Close()

	for {
		if gossiper.simple {
			var packet util.GossipPacket
			packetBytes := make([]byte, 2048)
			n, _, err := gossiper.clientConn.ReadFromUDP(packetBytes)
			util.CheckError(err)
			errDecode := protobuf.Decode(packetBytes[:n], &packet)
			util.CheckError(errDecode)

			util.PrintClientMessage(packet.Simple)

			// the OriginalName of the message to its own name
			// sets relay peer to its own address
			packetToSend := util.GossipPacket{Simple: &util.SimpleMessage{
				OriginalName:  gossiper.name,
				RelayPeerAddr: util.UDPAddrToString(gossiper.address),
				Contents:      packet.Simple.Contents,
			}}
			packetByte, err := protobuf.Encode(&packetToSend)
			util.CheckError(err)
			for peer := range (*gossiper.peers.PeersMap) {
				peerAddr, err := net.ResolveUDPAddr("udp4", peer)
				util.CheckError(err)
				_, err = gossiper.conn.WriteToUDP(packetByte, peerAddr)
				util.CheckError(err)
			}
			gossiper.peers.PrintPeers()
		}
	}
}

func (gossiper *Gossiper) ListenPeers(){
	defer gossiper.conn.Close()

	for {
		if gossiper.simple {
			var packet util.GossipPacket
			packetBytes := make([]byte, 2048)
			n, _, err := gossiper.conn.ReadFromUDP(packetBytes)
			util.CheckError(err)
			errDecode := protobuf.Decode(packetBytes[:n], &packet)
			util.CheckError(errDecode)

			util.PrintPeerMessage(packet.Simple)

			gossiper.peers.AddPeer(packet.Simple.RelayPeerAddr)
			packetToSend := util.GossipPacket{Simple: &util.SimpleMessage{
				OriginalName:  packet.Simple.OriginalName,
				RelayPeerAddr: util.UDPAddrToString(gossiper.address),
				Contents:      packet.Simple.Contents,
			}}
			packetByte, err := protobuf.Encode(&packetToSend)
			util.CheckError(err)
			for peer := range (*gossiper.peers.PeersMap) {
				if peer != packet.Simple.RelayPeerAddr{
					peerAddr, err := net.ResolveUDPAddr("udp4", peer)
					util.CheckError(err)
					_, err = gossiper.conn.WriteToUDP(packetByte, peerAddr)
					util.CheckError(err)
				}
			}
			gossiper.peers.PrintPeers()
		}
	}
}