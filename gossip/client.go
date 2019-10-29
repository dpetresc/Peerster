package gossip

import (
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/routing"
	"github.com/dpetresc/Peerster/util"
)

var hopLimit uint32 = 10

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
		// TODO est-ce que t'es obligé de ne pas être en simple pour les rumeurs ?
		if *packet.Destination != "" {
			// private message
			packetToSend := &util.GossipPacket{Private: &util.PrivateMessage{
				Origin: gossiper.Name,
				ID: 0,
				Text: packet.Text,
				Destination: *packet.Destination,
				HopLimit: hopLimit,
			}}
			gossiper.HandlePrivatePacket(packetToSend)

			// FOR THE GUI
			routing.AddNewPrivateMessageForGUI(*packet.Destination, packetToSend.Private)
		} else {
			packetToSend := gossiper.createNewPacketToSend(packet.Text, false)
			go gossiper.Rumormonger("", &packetToSend, false)
		}
	}
}
