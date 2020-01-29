package gossip

import (
	"encoding/json"
	"github.com/dpetresc/Peerster/util"
)

/*
 * createPivateMessageFromClient transforms a client message to a private message
 */
func (gossiper *Gossiper) createPivateMessageFromClient(message *util.Message, dest string) *util.PrivateMessage {
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
 * 	HandleClientTorMessage handles the messages coming from the client.
 *	message *util.Message is the message sent by the client.
 */
func (gossiper *Gossiper) HandleClientTorMessage(message *util.Message) {
	// TODO add timer and acks
	dest := *message.Destination
	privateMessage := gossiper.createPivateMessageFromClient(message, dest)

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
	torPayLoadExit, err := json.Marshal(*privateMessage)
	util.CheckError(err)
	torMessageExit := gossiper.encryptDataIntoTor(torPayLoadExit, circuit.ExitNode.SharedKey, util.TorData, util.Request, circuit.ID)

	// message for Middle Node
	torPayLoadMiddle, err := json.Marshal(torMessageExit)
	util.CheckError(err)
	torMessageMiddle := gossiper.encryptDataIntoTor(torPayLoadMiddle, circuit.MiddleNode.SharedKey, util.TorData, util.Request, circuit.ID)

	// message for Guard Node
	torPayLoadGuard, err := json.Marshal(torMessageMiddle)
	util.CheckError(err)
	torMessageGuard := gossiper.encryptDataIntoTor(torPayLoadGuard, circuit.GuardNode.SharedKey, util.TorData, util.Request, circuit.ID)

	go gossiper.HandleTorToSecure(torMessageGuard, circuit.GuardNode.Identity)
}

// TODO DO
