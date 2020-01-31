package gossip

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/dpetresc/Peerster/util"
)

/*
 * 	HandleClientTorMessage handles the messages coming from the client.
 *	message *util.Message is the message sent by the client.
 */
func (gossiper *Gossiper) HandleClientTorMessage(message *util.Message) {
	var dest string
	if message.Destination != nil {
		dest = *message.Destination
	} else {
		dest = ""
	}
	privateMessage := gossiper.pivateMessageFromClient(message, dest)

	// TODO ATTENTION LOCKS lCircuits puis ensuite LConsensus => CHANGE ???
	gossiper.lCircuits.Lock()
	gossiper.LConsensus.Lock()
	if message.CID == nil {
		// SEND
		gossiper.HandlePrivateMessageToSend(dest, privateMessage)
	} else {
		// REPLY
		gossiper.HandlePrivateMessageToReply(*message.CID, privateMessage)
	}
	gossiper.LConsensus.Unlock()
	gossiper.lCircuits.Unlock()
}

/*
 * HandlePrivateMessageToSend send a private message to dest with Tor
 */
func (gossiper *Gossiper) HandlePrivateMessageToSend(dest string, privateMessage *util.PrivateMessage) {
	if circuit, ok := gossiper.lCircuits.initiatedCircuit[dest]; ok {
		if circuit.NbCreated == 3 {
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
}

/*
 * HandlePrivateMessageToReply send a private message on CID circuit
 */
func (gossiper *Gossiper) HandlePrivateMessageToReply(CID uint32, privateMessage *util.PrivateMessage) {
	if circuit, ok := gossiper.lCircuits.circuits[CID]; ok {
		gossiper.sendReplyPrivateMessage(privateMessage, circuit)
		gossiper.handlePrivatePacketTor(privateMessage, gossiper.Name, circuit.ID)
	} else {
		fmt.Printf("Can not reply to message on circuit %d because it expired", CID)
	}
}

/*
 * sendReplyPrivateMessage send a private message reply on a previously opened circuit (this node is the exit node)
 * privateMessage: the reply
 * circuit: the circuit on wich the private message should be sent
 */
func (gossiper *Gossiper) sendReplyPrivateMessage(privateMessage *util.PrivateMessage, circuit *Circuit) {
	//dataMessage, err := json.Marshal(*privateMessage)
	dataMessage, err := json.Marshal(privateMessage)
	util.CheckError(err)

	dataTorMessage := &util.TorMessage{
		CircuitID:    circuit.ID,
		Flag:         util.TorData,
		Type:         util.Reply,
		NextHop:      "",
		DHPublic:     nil,
		DHSharedHash: nil,
		Nonce:        nil,
		Payload:      dataMessage,
	}

	relayExitPayloadBytes, err := json.Marshal(dataTorMessage)
	util.CheckError(err)

	relayExit := gossiper.encryptDataInRelay(relayExitPayloadBytes, circuit.SharedKey, util.Reply, circuit.ID)

	go gossiper.HandleTorToSecure(relayExit, circuit.PreviousHOP)
}

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
	//dataMessage, err := json.Marshal(*privateMessage)
	dataMessage, err := json.Marshal(privateMessage)
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
	var privateMessage util.PrivateMessage
	err := json.NewDecoder(bytes.NewReader(torDataMessage.Payload)).Decode(&privateMessage)
	util.CheckError(err)

	return &privateMessage
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