package gossip

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"github.com/monnand/dhkx"
	"math/rand"
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

	// add timer
}

//  All methods structures' are already locked when called

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
		NbInitiated: 0,
		Pending:     make([]*util.PrivateMessage, 0, len(privateMessages)),
	}
	newCircuit.Pending = privateMessages

	createTorMessage := gossiper.createInitiateRequest(newCircuit.ID, newCircuit.GuardNode)

	// add new Circuit to state
	gossiper.lCircuits.initiatedCircuit[dest] = newCircuit
	go gossiper.HandleTorToSecure(createTorMessage, newCircuit.GuardNode.Identity)
	gossiper.lConsensus.RUnlock()
}

/*
 *	createInitiateRequest - generate DH partial key and creates the Create TorMessage
 *	circuitID: the cicuit id
 *	toNode: the node we want to send the Create TorMessage to
 */
func (gossiper *Gossiper) createInitiateRequest(circuitID uint32, toNode *TorNode) *util.TorMessage {
	// DH
	privateDH, publicDH := util.CreateDHPartialKey()
	toNode.PartialPrivateKey = privateDH
	// encrypt with guard node key
	publicDHEncrypted := util.EncryptRSA(publicDH, gossiper.lConsensus.nodesPublicKeys[toNode.Identity])
	initiateMessage := &util.TorMessage{
		CircuitID:    circuitID,
		Flag:         util.Create,
		Type:         util.Request,
		NextHop:      "",
		DHPublic:     publicDHEncrypted,
		DHSharedHash: nil,
		Nonce:        nil,
		Payload:      nil,
	}
	return initiateMessage
}

/*
 *	findInitiatedCircuit finds the corresponding circuit when receiving a reply from the guard node
 *	torMessage the received message
 *	source the source node of the received message
 */
func (gossiper *Gossiper) findInitiatedCircuit(torMessage *util.TorMessage, source string) *InitiatedCircuit {
	var circuit *InitiatedCircuit = nil
	for _, c := range gossiper.lCircuits.initiatedCircuit {
		if c.ID == torMessage.CircuitID && c.GuardNode.Identity == source {
			circuit = c
			break
		}
	}
	return circuit
}

/*
 *	extractAndVerifySharedKeyInitiateReply
 *	torMessage the create reply torMessage received
 *	fromNode the node that replied to the create torMessage
 */
func extractAndVerifySharedKeyInitiateReply(torMessage *util.TorMessage, fromNode *TorNode) []byte {
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

func (gossiper *Gossiper) HandleTorInitiatorInitiateReply(torMessage *util.TorMessage, source string) {
	// first find corresponding circuit
	circuit := gossiper.findInitiatedCircuit(torMessage, source)
	if circuit != nil {
		// check hash of shared key
		shaKeyShared := extractAndVerifySharedKeyInitiateReply(torMessage, circuit.GuardNode)
		if shaKeyShared != nil {
			circuit.NbInitiated = circuit.NbInitiated + 1

			createTorMessage := gossiper.createInitiateRequest(circuit.ID, circuit.MiddleNode)
			torPayLoad, err := json.Marshal(createTorMessage)
			util.CheckError(err)
			ciphertext, nonce := util.EncryptGCM(torPayLoad, circuit.GuardNode.SharedKey)
			// send extend message to middle node
			extendTorMessage := &util.TorMessage{
				CircuitID:    circuit.ID,
				Flag:         util.Extend,
				Type:         util.Request,
				NextHop:      circuit.MiddleNode.Identity,
				DHPublic:     nil,
				DHSharedHash: nil,
				Nonce:        nonce,
				Payload:      ciphertext,
			}
			go gossiper.HandleTorToSecure(extendTorMessage, circuit.GuardNode.Identity)
		}
	} else {
		// TODO remove
		fmt.Println("RECEIVED INITIATE REPLY FROM " + source)
	}
}

func (gossiper *Gossiper) HandleTorIntermediateInitiateReply(torMessage *util.TorMessage, source string) {
	// encrypt with shared key the reply
	c := gossiper.lCircuits.circuits[torMessage.CircuitID]
	torMessageBytes, err := json.Marshal(torMessage)
	util.CheckError(err)
	// add type
	torExtendReply := gossiper.encryptDataIntoTor(torMessageBytes, c.SharedKey, util.Extend, util.Reply, c.ID)

	// send to previous node
	go gossiper.HandleTorToSecure(torExtendReply, c.PreviousHOP)
}

/*
 *	Already locked when called
 */
func (gossiper *Gossiper) HandleTorInitiateRequest(torMessage *util.TorMessage, source string) {
	// we haven't already received the Create tor message - ignore it otherwise
	if _, ok := gossiper.lCircuits.circuits[torMessage.CircuitID]; !ok {
		// decrpyt public DH key
		gossiper.lConsensus.RLock()
		publicDHReceived := util.DecryptRSA(torMessage.DHPublic, gossiper.lConsensus.privateKey)
		gossiper.lConsensus.RUnlock()

		// create DH shared key
		privateDH, publicDH := util.CreateDHPartialKey()
		shaKeyShared := util.CreateDHSharedKey(publicDHReceived, privateDH)

		// add circuit
		newCircuit := &Circuit{
			ID:          torMessage.CircuitID,
			PreviousHOP: source,
			NextHOP:     "",
			SharedKey:   shaKeyShared,
		}
		gossiper.lCircuits.circuits[torMessage.CircuitID] = newCircuit

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
