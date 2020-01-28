package gossip

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"github.com/dpetresc/Peerster/util"
	"github.com/monnand/dhkx"
)

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
			gossiper.sendMessageTorSecure(privateMessage, circuit)
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
func (gossiper *Gossiper) sendMessageTorSecure(privateMessage *util.PrivateMessage, circuit *InitiatedCircuit) {
	// message for Exit Node
	torPayLoad_exit, err := json.Marshal(*privateMessage)
	util.CheckError(err)
	torMessage_exit := gossiper.encryptDataIntoTor(torPayLoad_exit, circuit.ExitNode.Key, util.Payload, circuit.ID)

	// message for Middle Node
	torPayLoad_middle, err := json.Marshal(torMessage_exit)
	util.CheckError(err)
	torMessage_middle := gossiper.encryptDataIntoTor(torPayLoad_middle, circuit.MiddleNode.Key, util.Payload, circuit.ID)

	// message for Guard Node
	torPayLoad_guard, err := json.Marshal(torMessage_middle)
	util.CheckError(err)
	torMessage_guard := gossiper.encryptDataIntoTor(torPayLoad_guard, circuit.GuardNode.Key, util.Payload, circuit.ID)

	go gossiper.HandleMessageTorSecure(torMessage_guard, circuit.GuardNode.Identity)
}

func (gossiper *Gossiper) encryptDataIntoTor(data []byte, key []byte, flag util.TorFlag, circuitID uint32) *util.TorMessage {
	ciphertext, nonce := util.EncryptGCM(data, key)
	torMessage := &util.TorMessage{
		CircuitID:    circuitID,
		Flag:         flag,
		NextHop:      "",
		DHPublic:     nil,
		DHSharedHash: nil,
		Nonce:        nonce,
		Data:         ciphertext,
	}
	return torMessage
}

/*
 *	HandleMessageTorSecure handles the messages to send to the secure layer
 *	torMessage	the tor message
 *	destination	name of the secure destination
 */
func (gossiper *Gossiper) HandleMessageTorSecure(torMessage *util.TorMessage, destination string) {
	torToSecure, err := json.Marshal(torMessage)
	util.CheckError(err)
	gossiper.SecureBytesConsumer(torToSecure, destination)
}



/*
 *	HandleMessageSecureTor handles the messages coming from secure layer.
 *	torMessage	the tor message
 *	source	name of the secure source
 */
func (gossiper *Gossiper) HandleMessageSecureTor(torMessage *util.TorMessage, source string) {
	// TODO

	switch torMessage.Flag {
	case util.Create: {
		gossiper.lCircuits.Lock()
		if torMessage.DHSharedHash == nil {
			// REQUEST
			gossiper.HandleTorCreateRequest(torMessage, source)
		} else {
			// REPLY
			if _, ok := gossiper.lCircuits.circuits[torMessage.CircuitID]; ok {
				// INTERMEDIATE NODE

				// encrypt with shared key the reply
				c := gossiper.lCircuits.circuits[torMessage.CircuitID]
				torMessageBytes, err := json.Marshal(torMessage)
				util.CheckError(err)
				torExtendReply := gossiper.encryptDataIntoTor(torMessageBytes, c.SharedKey, util.Extend, c.ID)

				// send to previous node
				go gossiper.HandleMessageTorSecure(torExtendReply, c.PreviousHOP)
			} else {
				// THIS INITIATED THE CIRCUIT

				// guard node replied - send extend to middle node
				// first fin corresponding circuit
				// TODO change
				/*for dest, c := range gossiper.lCircuits.initiatedCircuit {
					if c.ID == torMessage.CircuitID && c.GuardNode.Identity == source {

					}
				}
				fmt.Println("RECEIVED CREATE REPLY WITHOUT INITIATED THE CIRCUIT FROM " + source)*/
			}
		}
		gossiper.lCircuits.Unlock()
	}
	case util.Extend:
		{

		}
	case util.Payload:
		{

		}
	}
}


/*
 *	Already locked when called
 */
func (gossiper *Gossiper) HandleTorCreateRequest(torMessage *util.TorMessage, source string) {
	// we haven't already received the Create tor message - ignore it otherwise
	if _, ok := gossiper.lCircuits.circuits[torMessage.CircuitID]; !ok {
		// decrpyt public DH key
		gossiper.lConsensus.RLock()
		dhPublic := util.DecryptRSA(torMessage.DHPublic, gossiper.lConsensus.privateKey)
		gossiper.lConsensus.RUnlock()

		// create DH shared key
		g, err := dhkx.GetGroup(0)
		util.CheckError(err)

		privateDH, err := g.GeneratePrivateKey(nil)
		util.CheckError(err)

		DHPublic := privateDH.Bytes()

		sharedKey, err := g.ComputeKey(dhkx.NewPublicKey(dhPublic), privateDH)
		util.CheckError(err)
		shaKeyShared := sha256.Sum256(sharedKey.Bytes())
		shaKeySharedBytes := shaKeyShared[:]

		// add circuit
		newCircuit := &Circuit{
			ID:          torMessage.CircuitID,
			PreviousHOP: source,
			NextHOP:     "",
			SharedKey:   shaKeySharedBytes,
		}
		gossiper.lCircuits.circuits[torMessage.CircuitID] = newCircuit

		// CREATE REPLY
		hashSharedKey := sha256.Sum256(shaKeySharedBytes)
		torMessageReply := &util.TorMessage{
			CircuitID:    torMessage.CircuitID,
			Flag:         util.Create,
			NextHop:      "",
			DHPublic:     DHPublic,
			DHSharedHash: hashSharedKey[:],
			Nonce:        nil,
			Data:         nil,
		}

		go gossiper.HandleMessageTorSecure(torMessageReply, source)
	}
}

func (gossiper *Gossiper) secureToTor(bytesData []byte, source string) {
	var torMessage util.TorMessage
	err := json.NewDecoder(bytes.NewReader(bytesData)).Decode(&torMessage)
	util.CheckError(err)
	gossiper.HandleMessageSecureTor(&torMessage, source)
}