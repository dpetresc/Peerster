package gossip

import (
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/util"
)

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
		id := gossiper.lAllMsg.allMsg[gossiper.Name].GetNextID()
		packetToSend := util.GossipPacket{Rumor: &util.RumorMessage{
			Origin: gossiper.Name,
			ID:     id,
			Text:   packet.Text,
		}}
		gossiper.lAllMsg.allMsg[gossiper.Name].AddMessage(&packetToSend, id)
		gossiper.lAllMsg.mutex.Unlock()
		go gossiper.Rumormonger("", &packetToSend, false)
	}
}
