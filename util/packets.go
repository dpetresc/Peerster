package util

import (
	"encoding/hex"
	"fmt"
	"strconv"
)

/******************** CLIENT MESSAGE ********************/
type Message struct {
	Text        string
	Destination *string
	File        *string
	Request     *[]byte
	Keywords    *string
	Budget      *uint64
	Secure      bool
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
	Simple        *SimpleMessage
	Rumor         *RumorMessage
	Status        *StatusPacket
	Private       *PrivateMessage
	DataRequest   *DataRequest
	DataReply     *DataReply
	SearchRequest *SearchRequest
	SearchReply   *SearchReply
	TLCMessage    *TLCMessage
	Ack           *TLCAck
	SecureMessage *SecureMessage
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
	ID     uint32
	Text   string
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
	if len(peerMessage.Want) > 0 {
		var s = ""
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
	Origin      string
	ID          uint32
	Text        string
	Destination string
	HopLimit    uint32
}

func (peerMessage *PrivateMessage) PrintPrivateMessage() {
	fmt.Printf("PRIVATE origin %s hop-limit %d contents %s\n", peerMessage.Origin,
		peerMessage.HopLimit, peerMessage.Text)
}

/******************** CHUNK AND METAFILE REQUESTS ********************/
type DataRequest struct {
	Origin string // check is already done in server_handler

	Destination string
	HopLimit    uint32
	HashValue   []byte
}

/******************** CHUNK AND METAFILE REPLIES ********************/
type DataReply struct {
	Origin      string
	Destination string
	HopLimit    uint32
	HashValue   []byte
	Data        []byte
}

/******************** SEARCH REQUEST ********************/
type SearchRequest struct {
	Origin   string
	Budget   uint64
	Keywords []string
}

/******************** SEARCH REPLY ********************/
type SearchReply struct {
	Origin      string
	Destination string
	HopLimit    uint32
	Results     []*SearchResult
}

/******************** SEARCH RESULT ********************/
type SearchResult struct {
	FileName     string
	MetafileHash []byte
	ChunkMap     []uint64
	ChunkCount   uint64
}

func (searchResult *SearchResult) PrintSearchMatch(origin string) {
	var s = ""
	s += fmt.Sprintf("FOUND match %s at %s metafile=%s chunks=", searchResult.FileName,
		origin, hex.EncodeToString(searchResult.MetafileHash))
	for _, chunkNb := range searchResult.ChunkMap[:len(searchResult.ChunkMap)-1] {
		s += fmt.Sprintf("%d", chunkNb)
		s += ","
	}
	s += fmt.Sprintf("%d", searchResult.ChunkMap[len(searchResult.ChunkMap)-1])
	fmt.Println(s)
}

/******************** BLOCK CHAIN ********************/
type TxPublish struct {
	Name         string
	Size         int64 // Size in bytes
	MetafileHash []byte
}

type BlockPublish struct {
	PrevHash    [32]byte
	Transaction TxPublish
}

type TLCMessage struct {
	Origin      string
	ID          uint32
	Confirmed   int
	TxBlock     BlockPublish
	VectorClock *StatusPacket
	Fitness     float32
}

type TLCAck PrivateMessage

/***************** SECURE MESSAGES ***********************/
/*
 *	MessageType represents the different type of messages that can be exchanged during a secure communication.
 *
 *	ClientHello is sent by the node that initiates the communication (A). It contains a nonce of 32 bytes that identifies
 *	the communication.
 *	ServerHello is sent by the node reached by the initiator (B).It contains a 32 bytes nonce (the
 *	one it previously received), its part of the Diffie-Hellman protocol and the signature of the Diffie-Hellman protocol.
 *	ChangeCipher is sent by A and contains its part of the Diffie-Hellman protocol and the signature
 *	of the Diffie-Hellman protocol.
 * 	ServerFinished is sent by B and contains the encrypted handshake (i.e., Enc(ClientHello||ServerHello||ChangeCipherSec))
 *	ClientFinished is sent by A and contains the encrypted handshake
 *	(i.e., Enc(ClientHello||ServerHello||ChangeCipherSec||ServerFinished))
 *	Data are the secure messages
 */
type MessageType uint32

const (
	ClientHello MessageType = iota
	ServerHello
	ChangeCipherSec
	ServerFinished
	ClientFinished
	ACK
	Data
)

type SecureMessage struct {
	MessageType   MessageType
	Nonce         []byte
	DHPublic      []byte
	DHSignature   []byte
	EncryptedData []byte
	GCMNonce      []byte
	Origin        string
	Destination   string
	HopLimit      uint32
}

func (secMsg *SecureMessage) Bytes() []byte{
	bytes := make([]byte,0)
	bytes = append(bytes, []byte(strconv.Itoa(int(secMsg.MessageType)))...)
	bytes = append(bytes, secMsg.Nonce...)
	bytes = append(bytes, secMsg.DHPublic...)
	bytes = append(bytes, secMsg.DHSignature...)
	bytes = append(bytes, secMsg.EncryptedData...)
	bytes = append(bytes, secMsg.GCMNonce...)
	bytes = append(bytes, []byte(secMsg.Origin)...)
	bytes = append(bytes, []byte(secMsg.Destination)...)
	bytes = append(bytes, []byte(strconv.Itoa(int(secMsg.HopLimit)))...)
	return bytes

}

func (secMsg *SecureMessage)  String() string{
	return fmt.Sprintf("TYPE: %d\nNonce: %x\nDHPublic: %x\nDHSignature: %x\nEncryptedData: %x\nGCMNonce: %x\nOrigin: %s\nDestination: %s\nHopLimit: %d\n",
		secMsg.MessageType, secMsg.Nonce, secMsg.DHPublic, secMsg.DHSignature, secMsg.EncryptedData, secMsg.GCMNonce, secMsg.Origin, secMsg.Destination, secMsg.HopLimit)
}
