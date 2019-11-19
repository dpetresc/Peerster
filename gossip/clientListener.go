package gossip

import (
	"fmt"
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/util"
)

/************************************CLIENT*****************************************/
func (gossiper *Gossiper) readClientPacket() *util.Message {
	connection := gossiper.ClientConn
	var packet util.Message
	packetBytes := make([]byte, util.MaxUDPSize + 2000)
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
		// we already checked that we have a correct combination of flag
		if packet.Keywords != nil {
			// search request
			keywords := util.GetNonEmptyElementsFromString(*packet.Keywords, ",")
			if len(keywords) > 0 {
				var searchPacket *util.GossipPacket
				searchPacket = &util.GossipPacket{SearchRequest: &util.SearchRequest{
					Origin: gossiper.Name,
					//Budget: *packet.Budget,
					Keywords: keywords,
				}}
				if packet.Budget == nil {
					searchPacket.SearchRequest.Budget = 2
					go gossiper.handleSearchRequestPacket(searchPacket, false)
				} else {
					searchPacket.SearchRequest.Budget = *packet.Budget
					go gossiper.handleSearchRequestPacket(searchPacket, true)
				}
			} else {
				// check is already done in server_handler
				// but not in command line mode
				fmt.Println("Empty keywords")
			}
		} else if packet.Text != "" {
			if packet.Destination != nil {
				// private message
				packetToSend := &util.GossipPacket{Private: &util.PrivateMessage{
					Origin: gossiper.Name,
					ID: 0,
					Text: packet.Text,
					Destination: *packet.Destination,
					HopLimit: util.HopLimit,
				}}
				go gossiper.handlePrivatePacket(packetToSend)

				// FOR THE GUI
				gossiper.AddNewPrivateMessageForGUI(*packet.Destination, packetToSend.Private)
			} else {
				// "public" message
				packetToSend := gossiper.createNewPacketToSend(packet.Text, false)
				go gossiper.rumormonger("", "", &packetToSend, false)
			}
		}else if packet.Destination != nil {
			// request file
			go gossiper.startDownload(packet)
		}else {
			// index file
			go gossiper.IndexFile(*packet.File)
		}

	}
}
