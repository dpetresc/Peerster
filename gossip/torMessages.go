package gossip

import (
	"bytes"
	"encoding/json"
	"github.com/dpetresc/Peerster/util"
	"sync"
)

type LockCircuits struct {
	circuits         map[uint32]*Circuit
	initiatedCircuit map[string]*InitiatedCircuit
	sync.RWMutex
}

func (gossiper *Gossiper) encryptDataIntoTor(data []byte, key []byte, flag util.TorFlag,
	messageType util.TorMessageType, circuitID uint32) *util.TorMessage {
	ciphertext, nonce := util.EncryptGCM(data, key)
	torMessage := &util.TorMessage{
		CircuitID:    circuitID,
		Flag:         flag,
		Type:         messageType,
		NextHop:      "",
		DHPublic:     nil,
		DHSharedHash: nil,
		Nonce:        nonce,
		Payload:      ciphertext,
	}
	return torMessage
}

/*
 *	HandleTorToSecure handles the messages to send to the secure layer
 *	torMessage	the tor message
 *	destination	name of the secure destination
 */
func (gossiper *Gossiper) HandleTorToSecure(torMessage *util.TorMessage, destination string) {
	torToSecure, err := json.Marshal(torMessage)
	util.CheckError(err)
	gossiper.SecureBytesConsumer(torToSecure, destination)
}


/*
 * secureToTor extract the data received by the secure layer and calls the corresponding handler
 */
func (gossiper *Gossiper) secureToTor(bytesData []byte, source string) {
	var torMessage util.TorMessage
	err := json.NewDecoder(bytes.NewReader(bytesData)).Decode(&torMessage)
	util.CheckError(err)
	gossiper.HandleSecureToTor(&torMessage, source)
}

/*
 *	HandleSecureToTor handles the messages coming from secure layer.
 *	torMessage	the tor message
 *	source	name of the secure source
 */
func (gossiper *Gossiper) HandleSecureToTor(torMessage *util.TorMessage, source string) {
	// TODO
	gossiper.lCircuits.Lock()
	switch torMessage.Flag {
		case util.Create: {
			switch torMessage.Type {
				case util.Request: {
					// can only receive Create Request if you are an intermediate node or exit node
					gossiper.HandleTorInitiateRequest(torMessage, source)
				}
				case util.Reply: {
					if _, ok := gossiper.lCircuits.circuits[torMessage.CircuitID]; ok {
						// INTERMEDIATE NODE
						gossiper.HandleTorIntermediateInitiateReply(torMessage, source)
					} else {
						// INITIATOR OF THE CIRCUIT AND THE GUARD NODE REPLIED
						gossiper.HandleTorInitiatorInitiateReply(torMessage, source)
					}
				}
			}
		}
		case util.Extend: {
			switch torMessage.Type {
				case util.Request: {
					// can only receive Extend Request if you are an intermediate node or exit node
					gossiper.HandleTorExtendRequest(torMessage, source)
				}
				case util.Reply: {

				}
			}
		}
		case util.TorData: {
			switch torMessage.Type {
				case util.Request: {

				}
				case util.Reply: {

				}
			}
		}
	}
	gossiper.lCircuits.Unlock()
}