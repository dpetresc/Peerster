package gossip

import (
	"encoding/json"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"github.com/monnand/dhkx"
	"math/rand"
	"sync"
)

/*
 *	Identity	name of the node in the Tor circuit
 *	Key			shared Key exhanged with the initiator of the circuit
 */
type TorNode struct {
	Identity string
	Key      []byte
}

/*
 *	ID	id of the Tor circuit
 *	PreviousHOP previous node in Tor
 *	NextHOP 	next node in Tor, nil if you are the destination
 *	SharedKey 	shared Key exchanged with the source
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
 *	NbInitiated 1,2,3 - if 3 the circuit has already been initiated
 */
type InitiatedCircuit struct {
	ID          uint32
	GuardNode   *TorNode
	MiddleNode  *TorNode
	ExitNode    *TorNode
	NbInitiated uint8
	Pending     []*util.PrivateMessage
}

type LockCircuits struct {
	circuits         map[uint32]*Circuit
	initiatedCircuit map[string]*InitiatedCircuit
	sync.RWMutex
}


/*
*	Already locked when called
 *	destination to be excluded
 *	nodesToExclude nodes that either crashed or where already selected (guard node)
 */
func (gossiper *Gossiper) selectRandomNodeFromConsensus(destination string, nodesToExclude ...string) string {
	nbNodes := len(gossiper.lConsensus.nodesPublicKeys) - 2 - len(nodesToExclude)

	randIndex := rand.Intn(nbNodes)
	for identity := range gossiper.lConsensus.nodesPublicKeys {
		if identity == gossiper.Name || identity == destination ||
			util.SliceContains(nodesToExclude, identity) {
			continue
		}
		if randIndex <= 0 {
			return identity
		}
		randIndex = randIndex - 1
	}

	return ""
}

/*
*	Already locked when called
*/
func (gossiper *Gossiper) selectPath(destination string, crashedNodes ...string) []string {
	// need to have at least two other nodes except the source and destination and the nodes that crashed
	nbNodes := len(gossiper.lConsensus.nodesPublicKeys) - 2 - len(crashedNodes)
	if nbNodes < 2 {
		fmt.Println("PeersTor hasn't enough active nodes, try again later")
		return nil
	}
	// destination has to exist in consensus
	if _, ok := gossiper.lConsensus.nodesPublicKeys[destination]; !ok {
		fmt.Println("Destination node isn't in PeersTor")
		return nil
	}

	guardNode := gossiper.selectRandomNodeFromConsensus(destination, crashedNodes...)
	middleNode := gossiper.selectRandomNodeFromConsensus(destination, append(crashedNodes, guardNode)...)
	return []string{guardNode, middleNode}
}

/*
 *	Already locked when called
 *	destination of the circuit
 *	privateMessages len = 1 if called with a client message, could be more if due to change in consensus for ex.
 */
func (gossiper *Gossiper) initiateNewCircuit(dest string, privateMessages []*util.PrivateMessage) {
	gossiper.lConsensus.RLock()
	nodes := gossiper.selectPath(dest)
	if nodes == nil {
		return
	}
	newCircuit := &InitiatedCircuit{
		ID: rand.Uint32(),
		GuardNode: &TorNode{
			Identity: nodes[0],
			Key:      nil,
		},
		MiddleNode: &TorNode{
			Identity: nodes[1],
			Key:      nil,
		},
		ExitNode: &TorNode{
			Identity: dest,
			Key:      nil,
		},
		NbInitiated: 0,
		Pending:     make([]*util.PrivateMessage, 0, len(privateMessages)),
	}
	newCircuit.Pending = privateMessages
	// add new Circuit to state
	gossiper.lCircuits.initiatedCircuit[dest] = newCircuit

	// Diffie Helman
	g, err := dhkx.GetGroup(0)
	util.CheckError(err)
	privateDH, err := g.GeneratePrivateKey(nil)
	util.CheckError(err)
	DHPublic := privateDH.Bytes()

	// encrypt with guard node key
	DHPublicEncrypted := util.EncryptRSA(DHPublic, gossiper.lConsensus.nodesPublicKeys[newCircuit.GuardNode.Identity])

	torMessage := &util.TorMessage{
		CircuitID: newCircuit.ID,
		Flag:      util.Create,
		NextHop:      "",
		DHPublic:     DHPublicEncrypted,
		DHSharedHash: nil,
		Data:         nil,
	}
	gossiper.lConsensus.RUnlock()
	go gossiper.HandleMessageTorSecure(torMessage, newCircuit.GuardNode.Identity)
}

func (gossiper *Gossiper) createPivateMessageFromClient(message *util.Message, dest string) *util.PrivateMessage {
	var clientOrigin string
	if !message.Anonyme {
		clientOrigin = gossiper.Name
	} else {
		clientOrigin = ""
	}
	// payload of TorMessage
	privateMessage := &util.PrivateMessage{
		Origin:      clientOrigin,
		ID:          0,
		Text:        message.Text,
		Destination: dest,
		HopLimit:    util.HopLimit,
	}
	return privateMessage
}

func (gossiper *Gossiper) sendMessageTorSecure(privateMessage *util.PrivateMessage, circuit *InitiatedCircuit) {
	torPayLoad, err := json.Marshal(*privateMessage)
	util.CheckError(err)

	// TODO changer il faut l'encrypter avec toutes les clés shared avant de l'envoyer
	torMessage := &util.TorMessage{
		CircuitID:    circuit.ID,
		Flag:         util.Payload,
		NextHop:      "",
		DHPublic:     nil,
		DHSharedHash: nil,
		Data:         torPayLoad,
	}
	go gossiper.HandleMessageTorSecure(torMessage, circuit.GuardNode.Identity)
}

/*
 * 	HandleClientTorMessage handles the messages coming from the client.
 *	message *util.Message is the message sent by the client.
 */
func (gossiper *Gossiper) HandleClientTorMessage(message *util.Message) {
	// TODO add timer and acks
	dest := *message.Destination
	privateMessage := gossiper.createPivateMessageFromClient(message, dest)

	// TODO ATTENTION LOCKS lCircuits puis ensuite lConsensus => CHANGE ???
	gossiper.lCircuits.Lock()
	if circuit, ok := gossiper.lCircuits.initiatedCircuit[dest]; ok {
		if circuit.NbInitiated == 3 {
			// Tor circuit exists and is already initiated and ready to be used
			gossiper.sendMessageTorSecure(privateMessage, circuit)
		} else {
			circuit.Pending = append(circuit.Pending, privateMessage)
		}
	} else {
		privateMessages := make([]*util.PrivateMessage, 0, 1)
		privateMessages = append(privateMessages, privateMessage)
		gossiper.initiateNewCircuit(dest, privateMessages)
	}
	gossiper.lCircuits.Unlock()
}

/*
 *	handleMessageSecureTor handles the messages coming from secure layer.
 *	torMessage	the tor message
 *	source	name of the secure source
 */
func (gossiper *Gossiper) HandleMessageSecureTor(torMessage *util.TorMessage, source string) {
	// TODO
}

func (gossiper *Gossiper) createCircuit(dest string) {
	gossiper.selectPath(dest)
}
