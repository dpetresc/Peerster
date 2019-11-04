package gossip

import (
	"encoding/hex"
	"github.com/dpetresc/Peerster/routing"
	"github.com/dpetresc/Peerster/util"
)

// either from a client or from another peer
// called in clientListener and peersListener
func (gossiper *Gossiper) handlePrivatePacket(packet *util.GossipPacket) {
	if packet.Private.Destination == gossiper.Name {
		packet.Private.PrintPrivateMessage()

		// FOR THE GUI
		routing.AddNewPrivateMessageForGUI(packet.Private.Origin, packet.Private)
	} else {
		nextHop := gossiper.LDsdv.GetNextHopOrigin(packet.Private.Destination)
		// we have the next hop of this origin
		if nextHop != "" {
			hopValue := packet.Private.HopLimit
			if hopValue > 0 {
				packetToForward := &util.GossipPacket{Private: &util.PrivateMessage{
					Origin:      packet.Private.Origin,
					ID:          packet.Private.ID,
					Text:        packet.Private.Text,
					Destination: packet.Private.Destination,
					HopLimit:    hopValue - 1,
				}}
				gossiper.sendPacketToPeer(nextHop, packetToForward)
			}
		}
	}
}

func (gossiper *Gossiper) sendRequestedChunk(packet *util.GossipPacket) {
	hashValue := packet.DataRequest.HashValue
	chunkId := hex.EncodeToString(hashValue)

	gossiper.lAllChunks.mutex.RLock()
	data, ok := gossiper.lAllChunks.chunks[chunkId]
	gossiper.lAllChunks.mutex.RUnlock()

	if !ok {
		data = make([]byte, 0)
	}
	dataReply := &util.GossipPacket{DataReply: &util.DataReply{
		Origin:      gossiper.Name,
		Destination: packet.DataRequest.Origin,
		HopLimit:    util.HopLimit,
		HashValue:   hashValue,
		Data:        data,
	}}
	gossiper.handleDataReplyPacket(dataReply)
}

// either from a client or from another peer
// called in clientListener and peersListener
func (gossiper *Gossiper) handleDataRequestPacket(packet *util.GossipPacket) {
	if packet.DataRequest.Destination == gossiper.Name {
		// someone wants my file / chunk
		gossiper.sendRequestedChunk(packet)
	} else {
		// transfer the file
		nextHop := gossiper.LDsdv.GetNextHopOrigin(packet.DataRequest.Destination)
		// we have the next hop of this origin
		if nextHop != "" {
			hopValue := packet.DataRequest.HopLimit
			if hopValue > 0 {
				packetToForward := &util.GossipPacket{DataRequest: &util.DataRequest{
					Origin:      packet.DataRequest.Origin,
					Destination: packet.DataRequest.Destination,
					HopLimit:    hopValue - 1,
					HashValue:   packet.DataRequest.HashValue,
				}}
				gossiper.sendPacketToPeer(nextHop, packetToForward)
			}
		}
	}
}

func (gossiper *Gossiper) handleDataReplyPacket(packet *util.GossipPacket) {
	if packet.DataReply.Destination == gossiper.Name {
		gossiper.lDownloadingChunk.mutex.Lock()
		hash := hex.EncodeToString(packet.DataReply.HashValue)
		from := packet.DataReply.Origin
		chunkIdentifier := DownloadIdentifier{
			from: from,
			hash: hash,
		}
		_, ok := gossiper.lDownloadingChunk.currentDownloadingChunks[chunkIdentifier]
		if ok {
			responseChan := gossiper.lDownloadingChunk.currentDownloadingChunks[chunkIdentifier]
			responseChan <- *packet.DataReply
		}
		gossiper.lDownloadingChunk.mutex.Unlock()
	} else {
		nextHop := gossiper.LDsdv.GetNextHopOrigin(packet.DataReply.Destination)
		// we have the next hop of this origin
		if nextHop != "" {
			hopValue := packet.DataReply.HopLimit
			if hopValue > 0 {
				packetToForward := &util.GossipPacket{DataReply: &util.DataReply{
					Origin: packet.DataReply.Origin,
					Destination: packet.DataReply.Destination,
					HopLimit: hopValue - 1,
					HashValue: packet.DataReply.HashValue,
					Data: packet.DataReply.Data,
				}}
				gossiper.sendPacketToPeer(nextHop, packetToForward)
			}
		}
	}
}
