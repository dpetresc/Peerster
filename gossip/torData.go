package gossip

import (
	"bytes"
	"encoding/json"
	"github.com/dpetresc/Peerster/util"
)

/*
 * pivateMessageFromClient transforms a client message to a private message
 */
func (gossiper *Gossiper) pivateMessageFromClient(message *util.Message, dest string) *util.PrivateMessage {
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

/*
 * torDataFromPrivateMessage transforms a private message to a Tor data message
 */
func torDataFromPrivateMessage(privateMessage *util.PrivateMessage, circuit *InitiatedCircuit) *util.TorMessage {
	dataMessage, err := json.Marshal(*privateMessage)
	util.CheckError(err)

	torMessage := &util.TorMessage{
		CircuitID:    circuit.ID,
		Flag:         util.TorData,
		Type:         util.Request,
		NextHop:      "",
		DHPublic:     nil,
		DHSharedHash: nil,
		Nonce:        nil,
		Payload:      dataMessage,
	}

	return torMessage
}

/*
 * privateMessageFromTorData transforms a private message to a Tor data message
 */
func privateMessageFromTorData(torDataMessage *util.TorMessage) *util.PrivateMessage {
	var privateMessage *util.PrivateMessage
	err := json.NewDecoder(bytes.NewReader(torDataMessage.Payload)).Decode(privateMessage)
	util.CheckError(err)

	return privateMessage
}

/*
 * createTorDataFromPrivate transforms a Tor data message to a private message
 */
func createTorDataFromPrivate(privateMessage *util.PrivateMessage, circuit *InitiatedCircuit) *util.TorMessage {
	dataMessage, err := json.Marshal(*privateMessage)
	util.CheckError(err)

	torMessage := &util.TorMessage{
		CircuitID:    circuit.ID,
		Flag:         util.TorData,
		Type:         util.Request,
		NextHop:      "",
		DHPublic:     nil,
		DHSharedHash: nil,
		Nonce:        nil,
		Payload:      dataMessage,
	}

	return torMessage
}

/*
 * 	HandleClientTorMessage handles the messages coming from the client.
 *	message *util.Message is the message sent by the client.
 */
func (gossiper *Gossiper) HandleClientTorMessage(message *util.Message) {
	// TODO add timer and acks
	dest := *message.Destination
	privateMessage := gossiper.pivateMessageFromClient(message, dest)

	// TODO ATTENTION LOCKS lCircuits puis ensuite lConsensus => CHANGE ???
	gossiper.lCircuits.Lock()
	if circuit, ok := gossiper.lCircuits.initiatedCircuit[dest]; ok {
		if circuit.NbInitiated == 3 {
			// Tor circuit exists and is already initiated and ready to be used
			gossiper.sendTorToSecure(privateMessage, circuit)
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
 *	Already locked when called
 */
func (gossiper *Gossiper) sendTorToSecure(privateMessage *util.PrivateMessage, circuit *InitiatedCircuit) {
	// message for Exit Node
	dataTorMessage := torDataFromPrivateMessage(privateMessage, circuit)
	relayExitPayloadBytes, err := json.Marshal(dataTorMessage)
	util.CheckError(err)

	relayExit := gossiper.encryptDataInRelay(relayExitPayloadBytes, circuit.ExitNode.SharedKey, util.Request, circuit.ID)

	// message for Middle Node
	relayMiddlePayloadBytes, err := json.Marshal(relayExit)
	util.CheckError(err)
	relayGuardPayloadBytes := gossiper.encryptDataInRelay(relayMiddlePayloadBytes, circuit.MiddleNode.SharedKey, util.Request, circuit.ID)

	// message for Guard Node
	relayGuard, err := json.Marshal(relayGuardPayloadBytes)
	util.CheckError(err)
	torMessageGuard := gossiper.encryptDataInRelay(relayGuard, circuit.GuardNode.SharedKey, util.Request, circuit.ID)

	go gossiper.HandleTorToSecure(torMessageGuard, circuit.GuardNode.Identity)
}

// TODO DO
