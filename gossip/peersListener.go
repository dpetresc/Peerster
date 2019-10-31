package gossip

import (
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/util"
	"net"
)

/************************************PEERS*****************************************/
func (gossiper *Gossiper) readGossipPacket() (*util.GossipPacket, *net.UDPAddr) {
	connection := gossiper.conn
	var packet util.GossipPacket
	packetBytes := make([]byte, util.MaxUDPSize)
	n, sourceAddr, err := connection.ReadFromUDP(packetBytes)
	// In case simple flag is set, we add manually the RelayPeerAddr of the packets afterwards
	if !gossiper.simple {
		if sourceAddr != gossiper.Address {
			gossiper.Peers.Mutex.Lock()
			gossiper.Peers.AddPeer(util.UDPAddrToString(sourceAddr))
			gossiper.Peers.Mutex.Unlock()
		}
	}
	util.CheckError(err)
	errDecode := protobuf.Decode(packetBytes[:n], &packet)
	util.CheckError(errDecode)

	return &packet, sourceAddr
}

func (gossiper *Gossiper) ListenPeers() {
	defer gossiper.conn.Close()

	for {
		packet, sourceAddr := gossiper.readGossipPacket()
		if gossiper.simple {
			if packet.Simple != nil {
				go gossiper.handleSimplePacket(packet)
			} else {
				//log.Fatal("Receive wrong packet format with simple flag !")
			}
		} else if packet.Rumor != nil {
			go gossiper.handleRumorPacket(packet, sourceAddr)
		} else if packet.Status != nil {
			go gossiper.handleStatusPacket(packet, sourceAddr)
		} else if packet.Private != nil {
			go gossiper.handlePrivatePacket(packet)
		} else if packet.DataRequest != nil {
			go gossiper.handleDataRequestPacket(packet)
		} else if packet.DataReply != nil {
			go gossiper.handleDataReplyPacket(packet)
		} else {
			//log.Fatal("Packet contains neither Status nor Rumor and gossiper wasn't used with simple flag !")
		}
	}
}

func (gossiper *Gossiper) handleSimplePacket(packet *util.GossipPacket) {
	packet.Simple.PrintSimpleMessage()
	gossiper.Peers.Mutex.Lock()
	if packet.Simple.RelayPeerAddr != util.UDPAddrToString(gossiper.Address) {
		gossiper.Peers.AddPeer(packet.Simple.RelayPeerAddr)
	}
	gossiper.Peers.PrintPeers()
	gossiper.Peers.Mutex.Unlock()
	packetToSend := util.GossipPacket{Simple: &util.SimpleMessage{
		OriginalName:  packet.Simple.OriginalName,
		RelayPeerAddr: util.UDPAddrToString(gossiper.Address),
		Contents:      packet.Simple.Contents,
	}}
	gossiper.sendPacketToPeers(packet.Simple.RelayPeerAddr, &packetToSend)
}
