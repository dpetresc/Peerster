package gossip

import (
	"encoding/json"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"math/rand"
	"sync"
)

/*
 *	identity	name of the node in the Tor circuit
 *	key			shared key exhanged with the initiator of the circuit
 */
type TorNode struct {
	identity string
	key      []byte
}

/*
 *	ID	id of the Tor circuit
 *	PreviousHOP previous node in Tor
 *	NextHOP 	next node in Tor, nil if you are the destination
 *	SharedKey 	shared key exchanged with the source
 */
type Circuit struct {
	ID          uint32
	PreviousHOP string
	NextHOP     string
	SharedKey   []byte
}

/*
 *	ID	id of the TOR circuit
 *	GuardNode first node in the circuit
 *	MiddleNode 	intermediate node
 *	ExitNode	third and last node
 */
type InitiatedCircuit struct {
	ID         uint32
	GuardNode  *TorNode
	MiddleNode *TorNode
	ExitNode   *TorNode
}

type LockCircuits struct {
	circuits         map[uint32]*Circuit
	initiatedCircuit map[string]*InitiatedCircuit
	sync.RWMutex
}

func (gossiper *Gossiper) selectRandomNodeFromConsensus(destination string) string {
	// need to have at least two nodes except the source and destination
	nbNodes := len(gossiper.lConsensus.nodesPublicKeys) - 2

	randIndex := rand.Intn(nbNodes)
	for identity := range gossiper.lConsensus.nodesPublicKeys {
		if identity == gossiper.Name || identity == destination {
			continue
		}
		if randIndex <= 0 {
			return identity
		}
		randIndex = randIndex - 1
	}

	return ""
}

func (gossiper *Gossiper) selectPath(destination string) []string {
	gossiper.lConsensus.RLock()

	if len(gossiper.lConsensus.nodesPublicKeys) < 4 {
		fmt.Println("PeersTor hasn't enough active nodes, try again later")
		return nil
	}
	if _, ok := gossiper.lConsensus.nodesPublicKeys[destination]; !ok {
		fmt.Println("Destination node isn't in PeersTor")
		return nil
	}

	guardNode := gossiper.selectRandomNodeFromConsensus(destination)
	middleNode := gossiper.selectRandomNodeFromConsensus(destination)
	gossiper.lConsensus.RUnlock()
	return []string{guardNode, middleNode}
}

func (gossiper *Gossiper) sendMessageTorSecure(message *util.Message, dest string, circuit *InitiatedCircuit) {
	// Tor circuit already exists
	var clientOrigin string
	if !message.Anonyme {
		clientOrigin = gossiper.Name
	} else {
		clientOrigin = ""
	}
	// payload of TorMessage
	privateMessage := util.PrivateMessage{
		Origin:      clientOrigin,
		ID:          0,
		Text:        message.Text,
		Destination: dest,
		HopLimit:    util.HopLimit,
	}

	torPayLoad, err := json.Marshal(privateMessage)
	util.CheckError(err)
	torMessage := &util.TorMessage{
		CircuitID: circuit.ID,
		Flag:      util.Payload,
		NextHop:   dest,
		Data:      torPayLoad,
	}
	go gossiper.HandleMessageTorSecure(torMessage)
}

/*
 * 	HandleClientTorMessage handles the messages coming from the client.
 *	message *util.Message is the message sent by the client.
 */
func (gossiper *Gossiper) HandleClientTorMessage(message *util.Message) {
	// TODO add timer
	dest := *message.Destination

	// TODO ATTENTION LOCKS lCircuits puis ensuite lConsensus
	gossiper.lCircuits.Lock()
	if circuit, ok := gossiper.lCircuits.initiatedCircuit[dest]; ok {
		gossiper.sendMessageTorSecure(message, dest, circuit)
	} else {

	}
	gossiper.lCircuits.Unlock()

	// Au moins quatre noeuds dans Tor
	// checker que la destination est dans Tor
	nodes := gossiper.selectPath(dest)
	if nodes == nil {
		return
	}

	// instantier la connexion TLS avec le premier circuit
}

/*
 *	handleMessageSecureTor handles the messages coming from secure layer.
 *	torMessage	the tor message
 *	source	name of the secure source
 */
func (gossiper *Gossiper) HandleMessageSecureTor(torMessage *util.TorMessage, source string) {

}

func (gossiper *Gossiper) createCircuit(dest string) {
	gossiper.selectPath(dest)
}
