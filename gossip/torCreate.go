package gossip

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"github.com/monnand/dhkx"
	"math/rand"
	"time"
)

/*
 *	Identity	name of the node in the Tor circuit
 *	PartialPrivateKey this node's partial private DH key
 *	SharedKey			shared SharedKey exchanged with the initiator of the circuit
 */
type TorNode struct {
	Identity          string
	PartialPrivateKey *dhkx.DHKey
	SharedKey         []byte
}

/*
 *	ID	id of the TOR circuit
 *	GuardNode first node in the circuit
 *	MiddleNode 	intermediate node
 *	ExitNode	third and last node
 *	NbCreated 1,2,3 - if 3 the circuit has already been initiated
 *	TimeoutChan timeout for the circuit
 */
type InitiatedCircuit struct {
	ID         uint32
	GuardNode  *TorNode
	MiddleNode *TorNode
	ExitNode   *TorNode
	NbCreated  uint8
	Pending    []*util.PrivateMessage

	TimeoutChan chan bool
}

/*
 *	Already locked when called
 *	node to exclude - when called by select path represents the destination to be excluded
 *	nodesToExclude nodes that either crashed or where already selected (guard node)
 */
func (gossiper *Gossiper) selectRandomNodeFromConsensus(nodeToExclude string, nodesToExclude ...string) string {
	nbNodes := len(gossiper.LConsensus.NodesPublicKeys) - 1 - len(nodesToExclude)
	if nodeToExclude != "" {
		nbNodes = nbNodes - 1
	}
	if nbNodes <= 0 {
		return ""
	}

	randIndex := rand.Intn(nbNodes)
	for identity := range gossiper.LConsensus.NodesPublicKeys {
		if identity == gossiper.Name || identity == nodeToExclude ||
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

//  All methods structures' are already locked when called

/*
*	Already locked when called
 */
func (gossiper *Gossiper) selectPath(destination string, crashedNodes ...string) []string {
	// need to have at least two other nodes except the source and destination and the nodes that crashed
	nbNodes := len(gossiper.LConsensus.NodesPublicKeys) - 2 - len(crashedNodes)
	if nbNodes < 2 {
		fmt.Println("PeersTor hasn't enough active nodes, try again later")
		return nil
	}
	// destination has to exist in consensus
	if _, ok := gossiper.LConsensus.NodesPublicKeys[destination]; !ok {
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
func (gossiper *Gossiper) initiateNewCircuit(dest string, privateMessages []*util.PrivateMessage, crashedNodes ...string) {
	nodes := gossiper.selectPath(dest, crashedNodes...)
	if nodes == nil {
		return
	}
	newCircuit := &InitiatedCircuit{
		ID: rand.Uint32(),
		GuardNode: &TorNode{
			Identity:          nodes[0],
			PartialPrivateKey: nil,
			SharedKey:         nil,
		},
		MiddleNode: &TorNode{
			Identity:          nodes[1],
			PartialPrivateKey: nil,
			SharedKey:         nil,
		},
		ExitNode: &TorNode{
			Identity:          dest,
			PartialPrivateKey: nil,
			SharedKey:         nil,
		},
		NbCreated:   0,
		Pending:     privateMessages,
		TimeoutChan: make(chan bool),
	}

	publicDHEncrypted := gossiper.generateAndEncryptPartialDHKey(newCircuit.GuardNode)
	createTorMessage := &util.TorMessage{
		CircuitID:    newCircuit.ID,
		Flag:         util.Create,
		Type:         util.Request,
		NextHop:      "",
		DHPublic:     publicDHEncrypted,
		DHSharedHash: nil,
		Nonce:        nil,
		Payload:      nil,
	}

	// add new Circuit to state
	gossiper.lCircuits.initiatedCircuit[dest] = newCircuit

	go gossiper.setTorExpirationTimeoutInitiator(dest, newCircuit)
	go gossiper.HandleTorToSecure(createTorMessage, newCircuit.GuardNode.Identity)
}

func (gossiper *Gossiper) generateAndEncryptPartialDHKey(toNode *TorNode) []byte {
	// DH
	privateDH, publicDH := util.CreateDHPartialKey()
	toNode.PartialPrivateKey = privateDH
	// encrypt with guard node key
	publicDHEncrypted := util.EncryptRSA(publicDH, gossiper.LConsensus.NodesPublicKeys[toNode.Identity])
	return publicDHEncrypted
}

/*
 *	extractAndVerifySharedKeyCreateReply
 *	torMessage the create reply torMessage received
 *	fromNode the node that replied to the create torMessage
 */
func extractAndVerifySharedKeyCreateReply(torMessage *util.TorMessage, fromNode *TorNode) []byte {
	publicDHReceived := torMessage.DHPublic
	shaKeyShared := util.CreateDHSharedKey(publicDHReceived, fromNode.PartialPrivateKey)
	hashSharedKey := sha256.Sum256(shaKeyShared)
	if !util.Equals(hashSharedKey[:], torMessage.DHSharedHash) {
		fmt.Println("The hash of the shared key received isn't the same ! ")
		return nil
	}
	fromNode.SharedKey = shaKeyShared
	return shaKeyShared
}

func (gossiper *Gossiper) HandleTorInitiatorCreateReply(torMessage *util.TorMessage, source string) {
	// first find corresponding circuit
	circuit := gossiper.findInitiatedCircuit(torMessage, source)
	if circuit != nil {
		circuit.TimeoutChan <- true

		// check hash of shared key
		shaKeyShared := extractAndVerifySharedKeyCreateReply(torMessage, circuit.GuardNode)
		if shaKeyShared != nil {
			circuit.NbCreated = circuit.NbCreated + 1

			extendMessage := gossiper.createExtendRequest(circuit.ID, circuit.MiddleNode)

			extendMessageBytes, err := json.Marshal(extendMessage)
			util.CheckError(err)
			relayMessage := gossiper.encryptDataInRelay(extendMessageBytes, circuit.GuardNode.SharedKey, util.Request, circuit.ID)

			go gossiper.HandleTorToSecure(relayMessage, circuit.GuardNode.Identity)
		}
	} else {
		// TODO remove
		fmt.Println("RECEIVED INITIATE REPLY FROM " + source)
	}
}

func (gossiper *Gossiper) HandleTorIntermediateCreateReply(torMessage *util.TorMessage, source string) {
	// encrypt with shared key the reply
	c := gossiper.lCircuits.circuits[torMessage.CircuitID]
	c.TimeoutChan <- true
	extendMessage := &util.TorMessage{
		CircuitID:    c.ID,
		Flag:         util.Extend,
		Type:         util.Reply,
		NextHop:      "",
		DHPublic:     torMessage.DHPublic,
		DHSharedHash: torMessage.DHSharedHash,
		Nonce:        nil,
		Payload:      nil,
	}
	extendMessageBytes, err := json.Marshal(extendMessage)
	util.CheckError(err)
	relayMessage := gossiper.encryptDataInRelay(extendMessageBytes, c.SharedKey, util.Reply, c.ID)

	// send to previous node
	go gossiper.HandleTorToSecure(relayMessage, c.PreviousHOP)
}

/*
 *	Already locked when called
 */
func (gossiper *Gossiper) HandleTorCreateRequest(torMessage *util.TorMessage, source string) {
	// we haven't already received the Create Tor message - ignore it otherwise
	if _, ok := gossiper.lCircuits.circuits[torMessage.CircuitID]; !ok {
		// decrpyt public DH key
		publicDHReceived := util.DecryptRSA(torMessage.DHPublic, gossiper.LConsensus.privateKey)

		// create DH shared key
		privateDH, publicDH := util.CreateDHPartialKey()
		shaKeyShared := util.CreateDHSharedKey(publicDHReceived, privateDH)

		// add circuit
		newCircuit := &Circuit{
			ID:          torMessage.CircuitID,
			PreviousHOP: source,
			NextHOP:     "",
			SharedKey:   shaKeyShared,
			TimeoutChan: make(chan bool),
		}
		gossiper.lCircuits.circuits[torMessage.CircuitID] = newCircuit

		go gossiper.setTorExpirationTimeoutIntermediate(newCircuit)

		// CREATE REPLY
		hashSharedKey := sha256.Sum256(shaKeyShared)
		torMessageReply := &util.TorMessage{
			CircuitID:    torMessage.CircuitID,
			Flag:         util.Create,
			Type:         util.Reply,
			NextHop:      "",
			DHPublic:     publicDH,
			DHSharedHash: hashSharedKey[:],
			Nonce:        nil,
			Payload:      nil,
		}

		go gossiper.HandleTorToSecure(torMessageReply, source)
	}
}

/*
 *	setTorExpirationTimeoutInitiator starts a new timer for the circuit that was previously opened. (for initiated circuit)
 *	When the timer expires another circuit is created for the destination if we have pending messages.
 */
func (gossiper *Gossiper) setTorExpirationTimeoutInitiator(dest string, circuit *InitiatedCircuit) {
	ticker := time.NewTicker(time.Duration(torTimeoutCircuits * time.Minute))
	for {
		select {
		case <-ticker.C:
			// time out, the circuit expires.
			fmt.Printf("EXPIRED circuit with %s\n", dest)
			ticker.Stop()
			gossiper.lCircuits.Lock()
			gossiper.LConsensus.Lock()
			if c, ok := gossiper.lCircuits.initiatedCircuit[dest]; ok {
				gossiper.changeCircuitPath(c, dest)
			}
			gossiper.LConsensus.Unlock()
			gossiper.lCircuits.Unlock()
			return
		case <-circuit.TimeoutChan:
			// circuit tunnel was used, reset the timer.
			ticker = time.NewTicker(time.Duration(torTimeoutCircuits * time.Minute))
		}
	}
}

/*
 *	Create a new circuit or remove it if no pending messages for initiated circuit.
 */
func (gossiper *Gossiper) changeCircuitPath(c *InitiatedCircuit, dest string) {
	privateMessages := c.Pending
	delete(gossiper.lCircuits.initiatedCircuit, dest)
	if len(c.Pending) > 0 {
		// try again with a new circuit
		// unlock consensus
		gossiper.initiateNewCircuit(dest, privateMessages)
	}
}

/*
 *	setTorExpirationTimeoutInitiator starts a new timer for the circuit that was previously opened. (for intermediate node)
 *	When the timer expires another circuit is created for the destination if we have pending messages.
 */
func (gossiper *Gossiper) setTorExpirationTimeoutIntermediate(circuit *Circuit) {
	ticker := time.NewTicker(time.Duration(torTimeoutCircuits * time.Minute))
	for {
		select {
		case <-ticker.C:
			// time out, the circuit expires.
			fmt.Println("EXPIRED circuit")
			ticker.Stop()
			/*gossiper.lCircuits.Lock()
			delete(gossiper.lCircuits.circuits, circuit.ID)
			gossiper.lCircuits.Unlock()*/

			gossiper.LLastPrivateMsg.Lock()
				delete(gossiper.LLastPrivateMsg.LastPrivateMsgTor, circuit.ID)
				gossiper.LLastPrivateMsg.Unlock()
				gossiper.lCircuits.Lock()
				delete(gossiper.lCircuits.circuits, circuit.ID)
				gossiper.lCircuits.Unlock()
			return
		case <-circuit.TimeoutChan:
			// circuit tunnel was used, reset the timer.
			ticker = time.NewTicker(time.Duration(torTimeoutCircuits * time.Minute))
		}
	}
}
