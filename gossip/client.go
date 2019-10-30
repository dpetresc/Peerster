package gossip

import (
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/routing"
	"github.com/dpetresc/Peerster/util"
)

/************************************CLIENT*****************************************/
func (gossiper *Gossiper) readClientPacket() *util.Message {
	connection := gossiper.ClientConn
	var packet util.Message
	packetBytes := make([]byte, MaxUDPSize)
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
		// sets relay peer to its own Address
		packetToSend := util.GossipPacket{Simple: &util.SimpleMessage{
			OriginalName:  gossiper.Name,
			RelayPeerAddr: util.UDPAddrToString(gossiper.Address),
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
			gossiper.Rumormonger("", &packetToSend, false)
		}
	}
}
