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

			if privateMessage.HsFlag == util.Bridge {
				//RDV point receives the message containing the cookie and the identity of the introduction point (IP)
				// from the client. It forwards the cookie and its name to the IP. Finally, it keeps a state linking
				//the cookie and the circuit used by the client.
				gossiper.bridges.Lock()
				gossiper.bridges.ClientServerPairs[privateMessage.Cookie] = &ClientServerPair{Client: circuit.ID,}
				gossiper.bridges.Unlock()

				newPrivMsg := &util.PrivateMessage{
					HsFlag:   util.Introduce,
					RDVPoint: privateMessage.RDVPoint,
					Cookie:   privateMessage.Cookie,
				}

				gossiper.HandlePrivateMessageToSend(privateMessage.IPIdentity, newPrivMsg)
			} else if privateMessage.HsFlag == util.Introduce {
				//IP receives the message of the RDV point. It forwards it to the server using the connection previously
				//established with the server.
				privateMessage.HsFlag = util.NewCo
				gossiper.HandlePrivateMessageToReply(circuit.ID, privateMessage)
			} else if privateMessage.HsFlag == util.NewCo {
				//Server receives the connection request with the cookie from the IP. It opens a connection to the RDV
				//point and sends the cookie.
				newPrivMsg := &util.PrivateMessage{
					HsFlag: util.Server,
					Cookie: privateMessage.Cookie,
				}
				gossiper.hsCo.Lock()
				gossiper.hsCo.hsCos[privateMessage.Cookie].RDVPoint = privateMessage.RDVPoint
				gossiper.hsCo.Unlock()
				gossiper.HandlePrivateMessageToSend(privateMessage.RDVPoint, newPrivMsg)

			} else if privateMessage.HsFlag == util.Server {
				//RDV point receives the message of the server. It notifies the client and keeps a state linking, the
				//cookie, the client's circuit ID and the server's circuit ID.
				gossiper.bridges.Lock()
				if pair, ok := gossiper.bridges.ClientServerPairs[privateMessage.Cookie]; ok {
					pair.Server = circuit.ID
					newPrivMsg := &util.PrivateMessage{
						HsFlag: util.Ready,
						Cookie: privateMessage.Cookie,
					}
					gossiper.HandlePrivateMessageToReply(pair.Client, newPrivMsg)
				}
				gossiper.bridges.Unlock()

			} else if privateMessage.HsFlag == util.Ready {
				//Client receives notification from RDV point and starts the DH key exchange.
				gossiper.connectionsToHS.Lock()
				defer gossiper.connectionsToHS.Unlock()

				if onionAddr, ok := gossiper.connectionsToHS.CookiesToAddr[privateMessage.Cookie]; ok {
					co := gossiper.connectionsToHS.Connections[onionAddr]
					privateDH, publicDH := util.CreateDHPartialKey()
					co.PrivateDH = privateDH
					newPrivMsg := &util.PrivateMessage{
						HsFlag:   util.ClientDHFwd,
						PublicDH: publicDH,
						Cookie:   privateMessage.Cookie,
					}

					gossiper.HandlePrivateMessageToSend(co.RDVPoint, newPrivMsg)
				}
			} else if privateMessage.HsFlag == util.ClientDHFwd || privateMessage.HsFlag == util.ServerDHFwd || privateMessage.HsFlag == util.HTTPFwd {
				//RDV points receives a message that it must forward.
				gossiper.bridges.Lock()
				if pair, ok := gossiper.bridges.ClientServerPairs[privateMessage.Cookie]; ok {
					if privateMessage.HsFlag == util.ClientDHFwd {
						privateMessage.HsFlag = util.ClientDH
					} else if privateMessage.HsFlag == util.ServerDHFwd {
						privateMessage.HsFlag = util.ServerDH
					} else {
						privateMessage.HsFlag = util.HTTP
					}
					gossiper.HandlePrivateMessageToReply(pair.Other(circuit.ID), privateMessage)
				}
				gossiper.bridges.Unlock()

			}else if privateMessage.HsFlag == util.ClientDH {
				//Server receives the DH part of the client, computes its part and the shared key. It keeps a state
				// of the connection.
				gossiper.hsCo.Lock()
				defer gossiper.hsCo.Unlock()
				if co, ok := gossiper.hsCo.hsCos[privateMessage.Cookie]; ok {
					clientPublicDH := privateMessage.PublicDH
					privateDH, publicDH := util.CreateDHPartialKey()
					sharedKey := util.CreateDHSharedKey(clientPublicDH, privateDH)
					co.SharedKey = sharedKey

					//TODO need private key of HS
					signatureDH := util.SignRSA(publicDH, nil)
					newPrivMsg := &util.PrivateMessage{
						HsFlag:      util.ServerDHFwd,
						Cookie:      privateMessage.Cookie,
						PublicDH:    publicDH,
						SignatureDH: signatureDH,
					}
					gossiper.HandlePrivateMessageToSend(co.RDVPoint, newPrivMsg)
				}
			}else if privateMessage.HsFlag == util.ServerDH{
				gossiper.connectionsToHS.Lock()
				defer gossiper.connectionsToHS.Unlock()

				if onionAddr, ok := gossiper.connectionsToHS.CookiesToAddr[privateMessage.Cookie]; ok {
					co := gossiper.connectionsToHS.Connections[onionAddr]
					co.SharedKey = util.CreateDHSharedKey(privateMessage.PublicDH, co.PrivateDH)
					fmt.Printf("CONNECTION to %s established\n", onionAddr)
				}
			} else {
				gossiper.handlePrivatePacketTor(privateMessage, circuit.ExitNode.Identity, circuit.ID)
			}

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
