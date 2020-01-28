package gossip

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
 *	createCreateRequest - generate DH partial key and creates the Create TorMessage
 *	circuitID: the cicuit id
 *	toNode: the node we want to send the Create TorMessageTo
 */
func (gossiper *Gossiper) createCreateRequest(circuitID uint32, toNode *TorNode) *util.TorMessage {
	// DH
	privateDH, publicDH := util.CreateDHPartialKey()
	toNode.PartialPrivateKey = privateDH
	// encrypt with guard node key
	publicDHEncrypted := util.EncryptRSA(publicDH, gossiper.lConsensus.nodesPublicKeys[toNode.Identity])
	createTorMessage := &util.TorMessage{
		CircuitID:    circuitID,
		Flag:         util.Create,
		NextHop:      "",
		DHPublic:     publicDHEncrypted,
		DHSharedHash: nil,
		Nonce:        nil,
		Data:         nil,
	}
	return createTorMessage
}

/*
 *	extractAndVerifySharedKeyCreateReply
 *	torMessage the create reply torMessage received
 *	fromNode the node that replied to the create torMessage
 */
func extractAndVerifySharedKeyCreateReply(torMessage *util.TorMessage, fromNode *TorNode) ([]byte) {
	publicDHReceived := torMessage.DHPublic
	shaKeyShared := util.CreateDHSharedKey(publicDHReceived, fromNode.PartialPrivateKey)
	hashSharedKey := sha256.Sum256(shaKeyShared)
	if !util.Equals(hashSharedKey[:], torMessage.DHSharedHash) {
		fmt.Println("The hash of the shared key received isn't the same ! ")
		return nil
	}
	fromNode.SharedKey = shaKeyShared
	return shaKeyShared
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
	torPayLoadExit, err := json.Marshal(*privateMessage)
	util.CheckError(err)
	torMessageExit := gossiper.encryptDataIntoTor(torPayLoadExit, circuit.ExitNode.SharedKey, util.Payload, circuit.ID)

	// message for Middle Node
	torPayLoadMiddle, err := json.Marshal(torMessageExit)
	util.CheckError(err)
	torMessageMiddle := gossiper.encryptDataIntoTor(torPayLoadMiddle, circuit.MiddleNode.SharedKey, util.Payload, circuit.ID)

	// message for Guard Node
	torPayLoadGuard, err := json.Marshal(torMessageMiddle)
	util.CheckError(err)
	torMessageGuard := gossiper.encryptDataIntoTor(torPayLoadGuard, circuit.GuardNode.SharedKey, util.Payload, circuit.ID)

	go gossiper.HandleMessageTorSecure(torMessageGuard, circuit.GuardNode.Identity)
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
 *	findInitiatedCircuit finds the corresponding circuit when receiving a reply from the guard node
 *	torMessage the received message
 *	source the source node of the received message
 */
func (gossiper *Gossiper) findInitiatedCircuit(torMessage *util.TorMessage, source string) *InitiatedCircuit {
	var circuit *InitiatedCircuit = nil
	for _, c := range gossiper.lCircuits.initiatedCircuit {
		if c.ID == torMessage.CircuitID && c.GuardNode.Identity == source {
			circuit = c
		}
	}
	return circuit
}

/*
 *	HandleMessageSecureTor handles the messages coming from secure layer.
 *	torMessage	the tor message
 *	source	name of the secure source
 */
func (gossiper *Gossiper) HandleMessageSecureTor(torMessage *util.TorMessage, source string) {
	// TODO

	switch torMessage.Flag {
	case util.Create:
		{
			gossiper.lCircuits.Lock()
			if torMessage.DHSharedHash == nil {
				// REQUEST
				gossiper.HandleTorCreateRequest(torMessage, source)
			} else {
				// REPLY
				gossiper.HandleTorCreateReply(torMessage, source)
			}
			gossiper.lCircuits.Unlock()
		}
	case util.Extend:
		{
			gossiper.lCircuits.Lock()
			if c, ok := gossiper.lCircuits.circuits[torMessage.CircuitID]; ok {
				// intermediate node
				if c.PreviousHOP == source {
					// REQUEST

				} else {
					// REPLY

				}
			} else {
				// INITIATOR - REPLY
				if torMessage.Nonce == nil {
					fmt.Println("The extend TorMessage isn't well formed at source")
					return
				}
				c := gossiper.findInitiatedCircuit(torMessage, source)
				if c == nil {
					fmt.Println("RECEIVED CREATE REPLY WITHOUT INITIATED THE CIRCUIT FROM " + source)
					return
					// TODO vérifier
					// could also happen if the circuit timeout
				}

				// Can contain a create Reply from middle node or from exit node
				if c.MiddleNode.SharedKey != nil {
					// create Reply is from exit node
					extendGuardBytes := util.DecryptGCM(torMessage.Data, torMessage.Nonce, c.GuardNode.SharedKey)
					var extendGuardMessage *util.TorMessage
					err := json.NewDecoder(bytes.NewReader(extendGuardBytes)).Decode(extendGuardMessage)
					util.CheckError(err)

					createExitReplyBytes := util.DecryptGCM(extendGuardMessage.Data, extendGuardMessage.Nonce, c.MiddleNode.SharedKey)
					var createExitReply *util.TorMessage
					err = json.NewDecoder(bytes.NewReader(createExitReplyBytes)).Decode(createExitReply)
					util.CheckError(err)

					shaKeyShared := extractAndVerifySharedKeyCreateReply(createExitReply, c.ExitNode)
					if shaKeyShared == nil {
						// TODO faire quelque chose ?
						return
					}
					c.NbInitiated = c.NbInitiated + 1

					// TODO envoyer les private message dans pending s'ils y en a
					//c.Pending
				} else {
					createMiddleReplyBytes := util.DecryptGCM(torMessage.Data, torMessage.Nonce, c.GuardNode.SharedKey)
					var createMiddleReply *util.TorMessage
					err := json.NewDecoder(bytes.NewReader(createMiddleReplyBytes)).Decode(createMiddleReply)
					util.CheckError(err)

					shaKeyShared := extractAndVerifySharedKeyCreateReply(createMiddleReply, c.ExitNode)
					if shaKeyShared == nil {
						// TODO faire quelque chose ?
						return
					}
					c.NbInitiated = c.NbInitiated + 1

					createTorMessage := gossiper.createCreateRequest(c.ID, c.ExitNode)
					torPayLoad, err := json.Marshal(createTorMessage)
					util.CheckError(err)
					ciphertext, nonce := util.EncryptGCM(torPayLoad, c.MiddleNode.SharedKey)
					// send extend message to middle node
					/*extendTorMessage := &util.TorMessage{
						CircuitID:    c.ID,
						Flag:         util.Extend,
						NextHop:      c.MiddleNode.Identity,
						DHPublic:     nil,
						DHSharedHash: nil,
						Nonce:        nonce,
						Data:         ciphertext,
					}*/
					// TODO décider comment faire

				}


			}
			gossiper.lCircuits.Unlock()
		}
	case util.Payload:
		{

		}
	}
}

func (gossiper *Gossiper) HandleTorCreateReply(torMessage *util.TorMessage, source string) {
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
		// THIS INITIATED THE CIRCUIT AND THE GUARD NODE REPLIED
		// first find corresponding circuit
		circuit := gossiper.findInitiatedCircuit(torMessage, source)
		if circuit == nil {
			fmt.Println("RECEIVED CREATE REPLY WITHOUT INITIATED THE CIRCUIT FROM " + source)
			return
			// TODO vérifier
			// could also happen if the circuit timeout
		}
		// check hash of shared key
		shaKeyShared := extractAndVerifySharedKeyCreateReply(torMessage, circuit.GuardNode)
		if shaKeyShared == nil {
			// TODO faire quelque chose ?
			return
		}
		circuit.NbInitiated = circuit.NbInitiated + 1

		createTorMessage := gossiper.createCreateRequest(circuit.ID, circuit.MiddleNode)
		torPayLoad, err := json.Marshal(createTorMessage)
		util.CheckError(err)
		ciphertext, nonce := util.EncryptGCM(torPayLoad, circuit.GuardNode.SharedKey)
		// send extend message to middle node
		extendTorMessage := &util.TorMessage{
			CircuitID:    circuit.ID,
			Flag:         util.Extend,
			NextHop:      circuit.MiddleNode.Identity,
			DHPublic:     nil,
			DHSharedHash: nil,
			Nonce:        nonce,
			Data:         ciphertext,
		}
		go gossiper.HandleMessageTorSecure(extendTorMessage, circuit.GuardNode.Identity)
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
		publicDHReceived := util.DecryptRSA(torMessage.DHPublic, gossiper.lConsensus.privateKey)
		gossiper.lConsensus.RUnlock()

		// create DH shared key
		privateDH, publicDH := util.CreateDHPartialKey()
		shaKeyShared := util.CreateDHSharedKey(publicDHReceived, privateDH)

		// add circuit
		newCircuit := &Circuit{
			ID:          torMessage.CircuitID,
			PreviousHOP: source,
			NextHOP:     "",
			SharedKey:   shaKeyShared,
		}
		gossiper.lCircuits.circuits[torMessage.CircuitID] = newCircuit

		// CREATE REPLY
		hashSharedKey := sha256.Sum256(shaKeyShared)
		torMessageReply := &util.TorMessage{
			CircuitID:    torMessage.CircuitID,
			Flag:         util.Create,
			NextHop:      "",
			DHPublic:     publicDH,
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
