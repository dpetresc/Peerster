package util

import (
	"encoding/hex"
	"fmt"
)

/******************** CLIENT MESSAGE ********************/
type Message struct {
	Text string
	Destination *string
	File *string
	Request *[]byte
	Keywords *string
	Budget   *uint64
}

func (clientMessage *Message) PrintClientMessage() {
	if clientMessage.Destination != nil {
		fmt.Printf("CLIENT MESSAGE %s dest %s\n", clientMessage.Text, *clientMessage.Destination)
	} else {
		fmt.Printf("CLIENT MESSAGE %s\n", clientMessage.Text)
	}
}

/******************** GOSSIP PACKET ********************/
type GossipPacket struct {
	Simple *SimpleMessage
	Rumor *RumorMessage
	Status *StatusPacket
	Private *PrivateMessage
	DataRequest *DataRequest
	DataReply *DataReply
	SearchRequest *SearchRequest
	SearchReply *SearchReply
}

/******************** SIMPLE MESSAGE ********************/
type SimpleMessage struct {
	OriginalName  string
	RelayPeerAddr string
	Contents      string
}

func (peerMessage *SimpleMessage) PrintSimpleMessage() {
	fmt.Printf("SIMPLE MESSAGE origin %s from %s contents %s\n", peerMessage.OriginalName,
		peerMessage.RelayPeerAddr, peerMessage.Contents)
}

/******************** RUMOR MESSAGE ********************/
type RumorMessage struct {
	Origin string
	ID uint32
	Text string
}

func (peerMessage *RumorMessage) PrintRumorMessage(sourceAddr string) {
	fmt.Printf("RUMOR origin %s from %s ID %d contents %s\n", peerMessage.Origin,
		sourceAddr, peerMessage.ID, peerMessage.Text)
}

/******************** STATUS PACKET ********************/
type StatusPacket struct {
	Want []PeerStatus
}

func (peerMessage *StatusPacket) PrintStatusMessage(sourceAddr string) {
	if (len(peerMessage.Want) > 0) {
		var s string = ""
		s += fmt.Sprintf("STATUS from %s ", sourceAddr)
		for _, peer := range peerMessage.Want[:len(peerMessage.Want)-1] {
			s += peer.GetPeerStatusAsStr()
			s += " "
		}
		s += peerMessage.Want[len(peerMessage.Want)-1].GetPeerStatusAsStr()
		fmt.Println(s)
	}
}

/******************** PRIVATE MESSAGE ********************/
type PrivateMessage struct {
	Origin string
	ID uint32
	Text string
	Destination string
	HopLimit uint32
}

func (peerMessage *PrivateMessage) PrintPrivateMessage() {
	fmt.Printf("PRIVATE origin %s hop-limit %d contents %s\n", peerMessage.Origin,
		peerMessage.HopLimit, peerMessage.Text)
}

/******************** CHUNK AND METAFILE REQUESTS ********************/
type DataRequest struct {
	Origin string				// check is already done in server_handler

	Destination string
	HopLimit uint32
	HashValue []byte
}

/******************** CHUNK AND METAFILE REPLIES ********************/
type DataReply struct {
	Origin string
	Destination string
	HopLimit uint32
	HashValue []byte
	Data []byte
}

/******************** SEARCH REQUEST ********************/
type SearchRequest struct {
	Origin string
	Budget uint64
	Keywords []string
}

/******************** SEARCH REPLY ********************/
type SearchReply struct {
	Origin string
	Destination string
	HopLimit uint32
	Results []*SearchResult
}

/******************** SEARCH RESULT ********************/
type SearchResult struct {
	FileName string
	MetafileHash []byte
	ChunkMap []uint64
	ChunkCount uint64
}

func (searchResult *SearchResult) PrintSearchMatch(origin string) {
	var s string = ""
	s += fmt.Sprintf("FOUND match %s at %s metafile=%s chunks=", searchResult.FileName,
		origin, hex.EncodeToString(searchResult.MetafileHash))
	for _, chunkNb := range searchResult.ChunkMap[:len(searchResult.ChunkMap)-1] {
		s += fmt.Sprintf("%d",chunkNb)
		s += ","
	}
	s += fmt.Sprintf("%d", searchResult.ChunkMap[len(searchResult.ChunkMap)-1])
	fmt.Println(s)
}