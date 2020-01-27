package gossip

import (
	"fmt"
	"github.com/dpetresc/Peerster/util"
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
	GuardNode  TorNode
	MiddleNode TorNode
	ExitNode   TorNode
}

type LockCircuits struct {
	circuits         map[uint32]*Circuit
	initiatedCircuit map[uint32]*InitiatedCircuit
	sync.RWMutex
}

/*
 * 	HandleClientTorMessage handles the messages coming from the client.
 *	message *util.Message is the message sent by the client.
 */
func (gossiper *Gossiper) HandleClientTorMessage(message *util.Message) {
	dest := *message.Destination
	fmt.Println(dest)
}

func (gossiper *Gossiper) createCircuit(dest string) {
	selectPath()
}

func selectPath() {

}

func (gossiper *Gossiper) HandleMessageSecureTor(torMessage *util.TorMessage, source string) {
	return
}