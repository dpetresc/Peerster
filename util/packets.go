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

	// Secure and anonymity
	Secure  bool
	Anonyme bool
	CID     *uint32

	// Hidden service
	HSPort    *string
	OnionAddr *string
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
	/*fmt.Printf("RUMOR origin %s from %s ID %d contents %s\n", peerMessage.Origin,
	sourceAddr, peerMessage.ID, peerMessage.Text)*/
}

/******************** STATUS PACKET ********************/
type StatusPacket struct {
	Want []PeerStatus
}

func (peerMessage *StatusPacket) PrintStatusMessage(sourceAddr string) {
	/*if len(peerMessage.Want) > 0 {
		var s = ""
		s += fmt.Sprintf("STATUS from %s ", sourceAddr)
		for _, peer := range peerMessage.Want[:len(peerMessage.Want)-1] {
			s += peer.GetPeerStatusAsStr()
			s += " "
		}
		s += peerMessage.Want[len(peerMessage.Want)-1].GetPeerStatusAsStr()
		fmt.Println(s)
	}*/
}

type HSFlag uint32

const (
	None HSFlag = iota
	IPRequest
	KeepAlive
	Bridge
	Introduce
	NewCo
	Server
	Ready
	ClientDHFwd
	ClientDH
	ServerDHFwd
	ServerDH
	HTTPFwd
	HTTP
	HTTPRepFwd
	HTTPRep
)

/******************** PRIVATE MESSAGE ********************/
type PrivateMessage struct {
	Origin      string
	ID          uint32
	Text        string
	Destination string
	HopLimit    uint32

	//HS flags
	HsFlag        HSFlag
	RDVPoint      string
	OnionAddr     string
	IPIdentity    string
	Cookie        uint64
	PublicDH      []byte
	SignatureDH   []byte
	GCMNonce      []byte
	GCMEncryption []byte
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

/***************** SECURE MESSAGES ***********************/
/*
 *	MessageType represents the different type of messages that can be exchanged during a secure communication.
 *
 *	1) ClientHello is sent by the node that initiates the communication (A). It contains a nonce of 32 bytes that identifies
 *	the communication.
 *	2) ServerHello is sent by the node reached by the initiator (B).It contains a 32 bytes nonce (the
 *	one it previously received), its part of the Diffie-Hellman protocol and the signature of the Diffie-Hellman protocol.
 *	3) ChangeCipherSec is sent by A and contains its part of the Diffie-Hellman protocol and the signature
 *	of the Diffie-Hellman protocol.
 * 	4) ServerFinished is sent by B and contains the encrypted handshake (i.e., Enc(ClientHello||ServerHello||ChangeCipherSec))
 *	5) ClientFinished is sent by A and contains the encrypted handshake
 *	(i.e., Enc(ClientHello||ServerHello||ChangeCipherSec||ServerFinished))
 *  6) ACKClientFinished
 *	7)+ Data are the secure messages
 */
type MessageType uint32

const (
	ClientHello MessageType = iota
	ServerHello
	ChangeCipherSec
	ServerFinished
	ClientFinished
	ACKClientFinished
	Data
	ACK
)

func (mt *MessageType) String() string {
	switch *mt {
	case ClientHello:
		return "ClientHello"
	case ServerHello:
		return "ServerHello"
	case ChangeCipherSec:
		return "ChangeCipherSec"
	case ServerFinished:
		return "ServerFinished"
	case ClientFinished:
		return "ClientFinished"
	case ACKClientFinished:
		return "ACKClientFinished"
	case Data:
		return "Data"
	default:
		return "Data"
	}
}

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
	CTR           uint32
}

func (secMsg *SecureMessage) Bytes() []byte {
	bytes := make([]byte, 0)
	bytes = append(bytes, []byte(strconv.Itoa(int(secMsg.MessageType)))...)
	bytes = append(bytes, secMsg.Nonce...)
	bytes = append(bytes, secMsg.DHPublic...)
	bytes = append(bytes, secMsg.DHSignature...)
	bytes = append(bytes, secMsg.EncryptedData...)
	bytes = append(bytes, secMsg.GCMNonce...)
	bytes = append(bytes, []byte(secMsg.Origin)...)
	bytes = append(bytes, []byte(secMsg.Destination)...)
	return bytes

}

func (secMsg *SecureMessage) String() string {
	return fmt.Sprintf("TYPE: %d\nNonce: %x\nDHPublic: %x\nDHSignature: %x\nEncryptedData: %x\nGCMNonce: %x\nOrigin: %s\nDestination: %s\nHopLimit: %d\n",
		secMsg.MessageType, secMsg.Nonce, secMsg.DHPublic, secMsg.DHSignature, secMsg.EncryptedData, secMsg.GCMNonce, secMsg.Origin, secMsg.Destination, secMsg.HopLimit)
}

/////////////////////////////////////TOR///////////////////////////////////////////////
type TorFlag uint32

const (
	Create TorFlag = iota
	Extend
	Relay
	TorData
)

type TorMessageType uint32

const (
	Request TorMessageType = iota
	Reply
)

/*
 *	CircuitID: All flags. The id of the Tor circuit
 *	NextHOP: Extend flag. The next node in Tor, nil if you are the final destination
 *	DHPublic: Create or Extend flag. The DH public part (encrypted in request and in clear in response)
 *	DHSharedHash: Create or Extend flag. The hash of the shared key (in response)
 *	Nonce: Relay flag. Used for encryption when the payload is encrypted
 *	Payload: Relay or TorData. The encrypted Tor message, or the data if destination
 */
type TorMessage struct {
	CircuitID    uint32
	Flag         TorFlag
	Type         TorMessageType
	NextHop      string
	DHPublic     []byte
	DHSharedHash []byte
	Nonce        []byte
	Payload      []byte
}

func (torMessage *TorMessage) String() string {
	return fmt.Sprintf("ID: %d \n DHPublic: %x \n"+
		"DHSharedHash: %x", torMessage.CircuitID, torMessage.DHPublic, torMessage.DHSharedHash)
}
