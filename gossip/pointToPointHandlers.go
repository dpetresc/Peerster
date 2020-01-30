package gossip

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/dpetresc/Peerster/util"
)

// either from a client or from another peer
// called in clientListener and peersListener
func (gossiper *Gossiper) handlePrivatePacket(packet *util.GossipPacket) {
	if packet.Private.Destination == gossiper.Name {
		packet.Private.PrintPrivateMessage()

		// FOR THE GUI
		gossiper.AddNewPrivateMessageForGUI(packet.Private.Origin, packet.Private)
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

	gossiper.lAllChunks.RLock()
	data, ok := gossiper.lAllChunks.chunks[chunkId]
	gossiper.lAllChunks.RUnlock()

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

func checkIntegrity(hash string, data []byte) bool {
	sha := sha256.Sum256(data[:])
	hashToTest := hex.EncodeToString(sha[:])
	return hashToTest == hash
}

func (gossiper *Gossiper) handleDataReplyPacket(packet *util.GossipPacket) {
	if packet.DataReply.Destination == gossiper.Name {
		hash := hex.EncodeToString(packet.DataReply.HashValue)
		data := make([]byte, 0, len(packet.DataReply.Data))
		data = append(data, packet.DataReply.Data...)

		if checkIntegrity(hash, data) || len(data) == 0 {
			from := packet.DataReply.Origin
			chunkIdentifier := DownloadIdentifier{
				from: from,
				hash: hash,
			}
			gossiper.lDownloadingChunk.RLock()
			if responseChan, ok := gossiper.lDownloadingChunk.currentDownloadingChunks[chunkIdentifier]; ok {
				responseChan <- *packet.DataReply
			}
			gossiper.lDownloadingChunk.RUnlock()
		}
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

func (gossiper *Gossiper) updateMatches(fSId FileSearchIdentifier, chunkIndex uint64, packet *util.GossipPacket, origin string, newResult bool) bool {
	isAlreadyTracked := false
	if peers, ok := gossiper.lSearchMatches.Matches[fSId].chunksDistribution[chunkIndex]; !ok {
		gossiper.lSearchMatches.Matches[fSId].chunksDistribution[chunkIndex] = make([]string, 0)
	} else {
		for _, peer := range peers {
			if peer == packet.SearchReply.Origin {
				isAlreadyTracked = true
				break
			}
		}
	}
	if !isAlreadyTracked {
		gossiper.lSearchMatches.Matches[fSId].chunksDistribution[chunkIndex] = append(gossiper.lSearchMatches.Matches[fSId].chunksDistribution[chunkIndex], origin)
		newResult = true
	}
	return newResult
}

// Search reply
func (gossiper *Gossiper) handleSearchReplyPacket(packet *util.GossipPacket) {
	if packet.SearchReply.Destination == gossiper.Name {
		gossiper.lSearchMatches.Lock()
		if gossiper.lSearchMatches.currNbFullMatch >= fullMatchThreshold {
			gossiper.lSearchMatches.Unlock()
			return
		}
		origin := packet.SearchReply.Origin
		for _,result := range packet.SearchReply.Results {
			if len(result.ChunkMap) == 0 {
				continue
			}
			metafile := hex.EncodeToString(result.MetafileHash)
			fSId := FileSearchIdentifier{
				Filename: result.FileName,
				Metahash: metafile,
			}
			if _,ok := gossiper.lSearchMatches.Matches[fSId]; !ok {
				matchStatus := MatchStatus{
					chunksDistribution: make(map[uint64][]string),
					totalNbChunk: result.ChunkCount,
				}
				gossiper.lSearchMatches.Matches[fSId] = &matchStatus
			}
			newResult := false
			for _,chunkIndex := range result.ChunkMap {
				newResult = gossiper.updateMatches(fSId, chunkIndex, packet, origin, newResult) || newResult
			}
			if newResult {
				result.PrintSearchMatch(origin)
				if uint64(len(result.ChunkMap)) == result.ChunkCount {
					gossiper.lSearchMatches.currNbFullMatch += 1
					if gossiper.lSearchMatches.currNbFullMatch >= fullMatchThreshold {
						fmt.Println("SEARCH FINISHED")
						gossiper.lSearchMatches.Unlock()
						return
					}
				}
			}
		}
		gossiper.lSearchMatches.Unlock()
	} else {
		// transfer the file
		nextHop := gossiper.LDsdv.GetNextHopOrigin(packet.SearchReply.Destination)
		// we have the next hop of this origin
		if nextHop != "" {
			hopValue := packet.SearchReply.HopLimit
			if hopValue > 0 {
				packetToForward := &util.GossipPacket{SearchReply: &util.SearchReply{
					Origin:      packet.SearchReply.Origin,
					Destination: packet.SearchReply.Destination,
					HopLimit:    hopValue - 1,
					Results:     packet.SearchReply.Results,
				}}
				gossiper.sendPacketToPeer(nextHop, packetToForward)
			}
		}
	}
}

func (gossiper *Gossiper) secureToPrivate(bytesData []byte, source string) {
	privateMessage := &util.PrivateMessage{}
	r := bytes.NewReader(bytesData)
	err := json.NewDecoder(r).Decode(privateMessage)
	util.CheckError(err)
	gossiper.handlePrivatePacket(&util.GossipPacket{
		Private: privateMessage,
	})
}

func (gossiper *Gossiper) privateToSecure(privateMessage *util.PrivateMessage){
	bytesData,err := json.Marshal(privateMessage)
	util.CheckError(err)
	gossiper.SecureBytesConsumer(bytesData, privateMessage.Destination)
}
