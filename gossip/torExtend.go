package gossip

import (
	"github.com/dpetresc/Peerster/util"
)

/*
 *	ID	id of the Tor circuit
 *	PreviousHOP previous node in Tor
 *	NextHOP 	next node in Tor, nil if you are the destination
 *	SharedKey 	shared SharedKey exchanged with the source
 *	TimeoutChan timeout for the circuit
 */
type Circuit struct {
	ID          uint32
	PreviousHOP string
	NextHOP     string
	SharedKey   []byte

	TimeoutChan chan bool
}

/*
 *	createExtendRequest - generate DH partial key and creates the corresponding Extend TorMessage
 *	circuitID: the cicuit id
 *	toNode: the node we want to send the Create TorMessage to
 */
func (gossiper *Gossiper) createExtendRequest(circuitID uint32, toNode *TorNode) *util.TorMessage {
	publicDHEncrypted := gossiper.generateAndEncryptPartialDHKey(toNode)
	extendMessage := &util.TorMessage{
		CircuitID:    circuitID,
		Flag:         util.Extend,
		Type:         util.Request,
		NextHop:      toNode.Identity,
		DHPublic:     publicDHEncrypted,
		DHSharedHash: nil,
		Nonce:        nil,
		Payload:      nil,
	}

	return extendMessage
}
