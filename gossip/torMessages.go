package gossip

import (
	"bytes"
	"encoding/json"
	"github.com/dpetresc/Peerster/util"
	"sync"
)

const torTimeoutCircuits = 6

type LockCircuits struct {
	circuits         map[uint32]*Circuit
	initiatedCircuit map[string]*InitiatedCircuit
	sync.RWMutex
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
 *	HandleTorToSecure handles the messages to send to the secure layer
 *	torMessage	the Tor message
 *	destination	name of the secure destination
 */
func (gossiper *Gossiper) HandleTorToSecure(torMessage *util.TorMessage, destination string) {
	torToSecure, err := json.Marshal(torMessage)
	util.CheckError(err)
	// TODO ADD PADDING
	gossiper.SecureBytesConsumer(torToSecure, destination)
}

/*
 * secureToTor extract the data received by the secure layer and calls the corresponding handler
 */
func (gossiper *Gossiper) secureToTor(bytesData []byte, source string) {
	// TODO REMOVE PADDING
	var torMessage util.TorMessage
	err := json.NewDecoder(bytes.NewReader(bytesData)).Decode(&torMessage)
	util.CheckError(err)
	gossiper.HandleSecureToTor(&torMessage, source)
}

/*
 *	HandleSecureToTor handles the messages coming from secure layer.
 *	torMessage	the Tor message
 *	source	name of the secure source
 */
func (gossiper *Gossiper) HandleSecureToTor(torMessage *util.TorMessage, source string) {
	gossiper.LConsensus.Lock()
	gossiper.lCircuits.Lock()
	switch torMessage.Flag {
	case util.Create:
		{
			switch torMessage.Type {
			case util.Request:
				{
					// can only receive Create Request if you are an intermediate node or exit node
					gossiper.HandleTorCreateRequest(torMessage, source)
				}
			case util.Reply:
				{
					if _, ok := gossiper.lCircuits.circuits[torMessage.CircuitID]; ok {
						// INTERMEDIATE NODE
						gossiper.HandleTorIntermediateCreateReply(torMessage, source)
					} else {
						// INITIATOR OF THE CIRCUIT AND THE GUARD NODE REPLIED
						gossiper.HandleTorInitiatorCreateReply(torMessage, source)
					}
				}
			}
		}
	case util.Relay:
		{
			switch torMessage.Type {
			case util.Request:
				{
					// can only receive Relay Request if you are an intermediate node or exit node
					gossiper.HandleTorRelayRequest(torMessage, source)
				}
			case util.Reply:
				{
					if _, ok := gossiper.lCircuits.circuits[torMessage.CircuitID]; ok {
						// INTERMEDIATE NODE
						gossiper.HandleTorIntermediateRelayReply(torMessage, source)
					} else {
						// INITIATOR OF THE CIRCUIT
						gossiper.HandleTorInitiatorRelayReply(torMessage, source)
					}
				}
			}
		}
	}
	gossiper.lCircuits.Unlock()
	gossiper.LConsensus.Unlock()
}
