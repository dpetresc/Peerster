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
	var torMessage util.TorMessage
	err := json.NewDecoder(bytes.NewReader(torMessageBytes)).Decode(&torMessage)
	util.CheckError(err)
	return &torMessage
}

func (gossiper *Gossiper) HandleTorRelayRequest(torMessage *util.TorMessage, source string) {
	if c, ok := gossiper.lCircuits.circuits[torMessage.CircuitID]; ok {
		c.TimeoutChan <- true
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
				c.NextHOP = torMessage.NextHop
				go gossiper.HandleTorToSecure(createTorMessage, c.NextHOP)
			}
		case util.TorData:
			{
				privateMessage := privateMessageFromTorData(torMessage)
				gossiper.handlePrivatePacketTor(privateMessage, privateMessage.Origin, c.ID)
			}
		}
	}
}

func (gossiper *Gossiper) HandleTorIntermediateRelayReply(torMessage *util.TorMessage, source string) {
	if c, ok := gossiper.lCircuits.circuits[torMessage.CircuitID]; ok {
		c.TimeoutChan <- true
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
		circuit.TimeoutChan <- true
		torMessageFirst := gossiper.decrpytTorMessageFromRelay(torMessage, circuit.GuardNode.SharedKey)

		if circuit.NbCreated == 1 {
			// The MIDDLE NODE exchanged key
			if torMessageFirst.Flag != util.Extend {
				// TODO remove should not happen
				fmt.Println("PROBLEM !")
				return
			}
			shaKeyShared := extractAndVerifySharedKeyCreateReply(torMessageFirst, circuit.MiddleNode)
			if shaKeyShared != nil {
				circuit.NbCreated = circuit.NbCreated + 1

				extendMessage := gossiper.createExtendRequest(circuit.ID, circuit.ExitNode)

				extendMessageBytes, err := json.Marshal(extendMessage)
				util.CheckError(err)
				relayMessage := gossiper.encryptDataInRelay(extendMessageBytes, circuit.MiddleNode.SharedKey, util.Request, circuit.ID)
				relayMessageFinalBytes, err := json.Marshal(relayMessage)
				util.CheckError(err)
				relayMessageFinal := gossiper.encryptDataInRelay(relayMessageFinalBytes, circuit.GuardNode.SharedKey, util.Request, circuit.ID)

				go gossiper.HandleTorToSecure(relayMessageFinal, circuit.GuardNode.Identity)
			}
			return
		}
		torMessageSecond := gossiper.decrpytTorMessageFromRelay(torMessageFirst, circuit.MiddleNode.SharedKey)

		if circuit.NbCreated == 2 {
			// The EXIT NODE exchanged key
			if torMessageSecond.Flag != util.Extend {
				// TODO remove should not happen
				fmt.Println("PROBLEM !")
				return
			}
			shaKeyShared := extractAndVerifySharedKeyCreateReply(torMessageSecond, circuit.ExitNode)
			if shaKeyShared != nil {
				circuit.NbCreated = circuit.NbCreated + 1

				// send pendings messages on connection
				for _, privateMessage := range circuit.Pending {
					gossiper.sendTorToSecure(privateMessage, circuit)
					gossiper.handlePrivatePacketTor(privateMessage, gossiper.Name, circuit.ID)
				}
				circuit.Pending = make([]*util.PrivateMessage, 0, 0)
			}
		} else if circuit.NbCreated >= 3 {
			// we receive data replies from the exit node
			torMessagePayload := gossiper.decrpytTorMessageFromRelay(torMessageSecond, circuit.ExitNode.SharedKey)
			if torMessagePayload.Flag != util.TorData {
				// TODO remove should not happen
				fmt.Println("PROBLEM !")
				return
			}
			privateMessage := privateMessageFromTorData(torMessagePayload)
			gossiper.handlePrivatePacketTor(privateMessage, circuit.ExitNode.Identity, circuit.ID)
		}
	} else {
		// TODO remove
		fmt.Println("RECEIVED INITIATE REPLY FROM " + source)
	}
}

func (gossiper *Gossiper) handlePrivatePacketTor(privateMessage *util.PrivateMessage, origin string, cID uint32) {
	privateMessage.Origin = origin
	fmt.Println(cID)
	privateMessage.PrintPrivateMessage()
	gossiper.LLastPrivateMsg.Lock()
	gossiper.LLastPrivateMsg.LastPrivateMsgTor[cID] = append(gossiper.LLastPrivateMsg.LastPrivateMsgTor[cID], privateMessage)
	gossiper.LLastPrivateMsg.Unlock()
}
