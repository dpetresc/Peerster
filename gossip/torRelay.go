package gossip

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/dpetresc/Peerster/util"
)

func (gossiper *Gossiper) encryptDataInRelay(data []byte, key []byte,
	messageType util.TorMessageType, circuitID uint32) *util.TorMessage {
	ciphertext, nonce := util.EncryptGCM(data, key)
	torMessage := &util.TorMessage{
		CircuitID:    circuitID,
		Flag:         util.Relay,
		Type:         messageType,
		NextHop:      "",
		DHPublic:     nil,
		DHSharedHash: nil,
		Nonce:        nonce,
		Payload:      ciphertext,
	}
	return torMessage
}

func (gossiper *Gossiper) decrpytTorMessageFromRelay(torMessageRelay *util.TorMessage, sharedKey []byte) *util.TorMessage {
	// decrpyt payload
	torMessageBytes := util.DecryptGCM(torMessageRelay.Payload, torMessageRelay.Nonce, sharedKey)
	var torMessage *util.TorMessage
	err := json.NewDecoder(bytes.NewReader(torMessageBytes)).Decode(torMessage)
	util.CheckError(err)
	return torMessage
}

func (gossiper *Gossiper) HandleTorRelayRequest(torMessage *util.TorMessage, source string) {
	if c, ok := gossiper.lCircuits.circuits[torMessage.CircuitID]; ok {
		torMessage := gossiper.decrpytTorMessageFromRelay(torMessage, c.SharedKey)

		switch torMessage.Flag {
		case util.Relay:
			{
				// we need to continue to relay the message
				go gossiper.HandleTorToSecure(torMessage, c.NextHOP)
			}
		case util.Extend:
			{
				// we need to send a create request to next hop
				createTorMessage := &util.TorMessage{
					CircuitID:    torMessage.CircuitID,
					Flag:         util.Create,
					Type:         util.Request,
					NextHop:      "",
					DHPublic:     torMessage.DHPublic,
					DHSharedHash: nil,
					Nonce:        nil,
					Payload:      nil,
				}
				go gossiper.HandleTorToSecure(createTorMessage, c.NextHOP)
			}
		case util.TorData:
			{
				privateMessage := privateMessageFromTorData(torMessage)
				gossiper.LLastPrivateMsg.Lock()
				gossiper.LLastPrivateMsg.LastPrivateMsgTor[torMessage.CircuitID] = append(gossiper.LLastPrivateMsg.LastPrivateMsgTor[torMessage.CircuitID], privateMessage)
				gossiper.LLastPrivateMsg.Unlock()
			}
		}
	}
}

func (gossiper *Gossiper) HandleTorIntermediateRelayReply(torMessage *util.TorMessage, source string) {
	if c, ok := gossiper.lCircuits.circuits[torMessage.CircuitID]; ok {
		torMessageBytes, err := json.Marshal(torMessage)
		util.CheckError(err)
		relayMessage := gossiper.encryptDataInRelay(torMessageBytes, c.SharedKey, util.Reply, c.ID)
		go gossiper.HandleTorToSecure(relayMessage, c.PreviousHOP)
	}
}

func (gossiper *Gossiper) HandleTorInitiatorRelayReply(torMessage *util.TorMessage, source string) {
	// first find corresponding circuit
	circuit := gossiper.findInitiatedCircuit(torMessage, source)
	if circuit != nil {
		if circuit.NbInitiated == 1 {

		} else if circuit.NbInitiated == 2 {

		} else {

		}
		// TODO traiter le cas ou c'est une reponse à une écahnge dclé avec OR2 par ex
		// decrpyt payload guard
		torMessageMiddle := gossiper.decrpytTorMessageFromRelay(torMessage, circuit.GuardNode.SharedKey)

		// decrpyt payload middle
		torMessageExit := gossiper.decrpytTorMessageFromRelay(torMessageMiddle, circuit.MiddleNode.SharedKey)

		// decrpyt payload exit
		torMessagePayload := gossiper.decrpytTorMessageFromRelay(torMessageExit, circuit.ExitNode.SharedKey)

		switch torMessagePayload.Flag {
			case util.TorData: {
				privateMessage := privateMessageFromTorData(torMessage)
				gossiper.LLastPrivateMsg.Lock()
				gossiper.LLastPrivateMsg.LastPrivateMsgTor[torMessage.CircuitID] = append(gossiper.LLastPrivateMsg.LastPrivateMsgTor[torMessage.CircuitID], privateMessage)
				gossiper.LLastPrivateMsg.Unlock()
			}
			case util.Extend: {
				// check hash of shared key
				shaKeyShared := extractAndVerifySharedKeyCreateReply(torMessage, circuit.ExitNode)
				if shaKeyShared != nil {
					circuit.NbInitiated = circuit.NbInitiated + 1

					extendMessage := gossiper.createExtendRequest(circuit.ID, circuit.MiddleNode)

					extendMessageBytes, err := json.Marshal(extendMessage)
					util.CheckError(err)
					relayMessage := gossiper.encryptDataInRelay(extendMessageBytes,  circuit.GuardNode.SharedKey, util.Request, circuit.ID)


					go gossiper.HandleTorToSecure(relayMessage, circuit.GuardNode.Identity)
				}
			}
		}
	} else {
		// TODO remove
		fmt.Println("RECEIVED INITIATE REPLY FROM " + source)
	}
}
