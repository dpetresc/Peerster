package gossip

import (
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"github.com/monnand/dhkx"
	"math/rand"
	"sync"
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
 *	ID	id of the Tor circuit
 *	PreviousHOP previous node in Tor
 *	NextHOP 	next node in Tor, nil if you are the destination
 *	SharedKey 	shared SharedKey exchanged with the source
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

//  All methods structures' are already locked when called

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
	createTorMessage := gossiper.createCreateRequest(newCircuit.ID, newCircuit.GuardNode)

	// add new Circuit to state
	gossiper.lCircuits.initiatedCircuit[dest] = newCircuit
	go gossiper.HandleMessageTorSecure(createTorMessage, newCircuit.GuardNode.Identity)
	gossiper.lConsensus.RUnlock()
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
