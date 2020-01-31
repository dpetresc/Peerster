package gossip

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"io/ioutil"
	"strings"
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

				if privateMessage.HsFlag == util.KeepAlive {
					return
				}
				if privateMessage.HsFlag == util.IPRequest {
					//fmt.Println("IPRequest")
					gossiper.LHS.RLock()
					gossiper.LHS.OnionAddrToCircuit[privateMessage.OnionAddr] = c.ID
					gossiper.LHS.RUnlock()
					//fmt.Println("IPRequest")
				} else if privateMessage.HsFlag == util.Bridge {
					//fmt.Println("Bridge")
					// RDV point receives the message containing the cookie and the identity of the introduction point (IP)
					// from the client. It forwards the cookie and its name to the IP. Finally, it keeps a state linking
					// the cookie and the circuit used by the client.
					gossiper.bridges.Lock()
					gossiper.bridges.ClientServerPairs[privateMessage.Cookie] = &ClientServerPair{Client: c.ID,}
					gossiper.bridges.Unlock()

					fmt.Println(privateMessage.Cookie)
					newPrivMsg := &util.PrivateMessage{
						HsFlag:    util.Introduce,
						RDVPoint:  gossiper.Name,
						Cookie:    privateMessage.Cookie,
						OnionAddr: privateMessage.OnionAddr,
					}

					gossiper.HandlePrivateMessageToSend(privateMessage.IPIdentity, newPrivMsg)
					//fmt.Println("Bridge")
				} else if privateMessage.HsFlag == util.Introduce {
					//fmt.Println("Introduce")
					// IP receives the message of the RDV point. It forwards it to the server using the connection previously
					// established with the server.
					privateMessage.HsFlag = util.NewCo
					gossiper.LHS.RLock()
					if cID, ok := gossiper.LHS.OnionAddrToCircuit[privateMessage.OnionAddr]; ok {
						gossiper.HandlePrivateMessageToReply(cID, privateMessage)
					}
					gossiper.LHS.RUnlock()
					//fmt.Println("Introduce")
				} else if privateMessage.HsFlag == util.Server {
					//fmt.Println("Server")
					// RDV point receives the message of the server. It notifies the client and keeps a state linking, the
					// cookie, the client's circuit ID and the server's circuit ID.
					fmt.Println(privateMessage.Cookie)
					gossiper.bridges.Lock()
					if pair, ok := gossiper.bridges.ClientServerPairs[privateMessage.Cookie]; ok {
						fmt.Println("ICI")
						pair.Server = c.ID
						newPrivMsg := &util.PrivateMessage{
							HsFlag:    util.Ready,
							Cookie:    privateMessage.Cookie,
							OnionAddr: privateMessage.OnionAddr,
						}
						gossiper.HandlePrivateMessageToReply(pair.Client, newPrivMsg)
					}
					gossiper.bridges.Unlock()
					//fmt.Println("Server")
				} else if privateMessage.HsFlag == util.ClientDHFwd || privateMessage.HsFlag == util.ServerDHFwd || privateMessage.HsFlag == util.HTTPFwd || privateMessage.HsFlag == util.HTTPRepFwd {
					//fmt.Println("FORWARD")
					// RDV points receives a message that it must forward.
					gossiper.bridges.Lock()
					if pair, ok := gossiper.bridges.ClientServerPairs[privateMessage.Cookie]; ok {
						if privateMessage.HsFlag == util.ClientDHFwd {
							privateMessage.HsFlag = util.ClientDH
						} else if privateMessage.HsFlag == util.ServerDHFwd {
							privateMessage.HsFlag = util.ServerDH
						} else if privateMessage.HsFlag == util.HTTPFwd {
							privateMessage.HsFlag = util.HTTP
						} else {
							privateMessage.HsFlag = util.HTTPRep
						}
						gossiper.HandlePrivateMessageToReply(pair.Other(c.ID), privateMessage)
					}
					gossiper.bridges.Unlock()
					//fmt.Println("FORWARD")
				} else {
					//fmt.Println("ELSE")
					gossiper.handlePrivatePacketTor(privateMessage, privateMessage.Origin, c.ID)
				}
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

			if privateMessage.HsFlag == util.NewCo {
				//fmt.Println("NewCo")
				// Server receives the connection request with the cookie from the IP. It opens a connection to the RDV
				// point and sends the cookie.
				newPrivMsg := &util.PrivateMessage{
					HsFlag:    util.Server,
					Cookie:    privateMessage.Cookie,
					OnionAddr: privateMessage.OnionAddr,
				}
				gossiper.hsCo.Lock()
				gossiper.hsCo.hsCos[privateMessage.Cookie] = &HSConnection{
					SharedKey: nil,
					RDVPoint:  privateMessage.RDVPoint,
					OnionAddr: privateMessage.OnionAddr,
				}
				gossiper.hsCo.Unlock()
				gossiper.HandlePrivateMessageToSend(privateMessage.RDVPoint, newPrivMsg)
				//fmt.Println("NewCo")
			} else if privateMessage.HsFlag == util.Ready {
				//fmt.Println("Ready")
				// Client receives notification from RDV point and starts the DH key exchange.
				gossiper.connectionsToHS.Lock()
				defer gossiper.connectionsToHS.Unlock()

				if onionAddr, ok := gossiper.connectionsToHS.CookiesToAddr[privateMessage.Cookie]; ok {
					co := gossiper.connectionsToHS.Connections[onionAddr]
					privateDH, publicDH := util.CreateDHPartialKey()
					co.PrivateDH = privateDH
					newPrivMsg := &util.PrivateMessage{
						HsFlag:    util.ClientDHFwd,
						PublicDH:  publicDH,
						Cookie:    privateMessage.Cookie,
						OnionAddr: privateMessage.OnionAddr,
					}

					gossiper.HandlePrivateMessageToSend(co.RDVPoint, newPrivMsg)
				}
				//fmt.Println("Ready")
			} else if privateMessage.HsFlag == util.ClientDH {
				//fmt.Println("ClientDH")
				// Server receives the DH part of the client, computes its part and the shared key. It keeps a state
				// of the connection.
				gossiper.hsCo.Lock()
				defer gossiper.hsCo.Unlock()
				if co, ok := gossiper.hsCo.hsCos[privateMessage.Cookie]; ok {
					clientPublicDH := privateMessage.PublicDH
					privateDH, publicDH := util.CreateDHPartialKey()
					sharedKey := util.CreateDHSharedKey(clientPublicDH, privateDH)
					co.SharedKey = sharedKey

					gossiper.LHS.RLock()
					if privateKey, ok := gossiper.LHS.MPrivateKeys[privateMessage.OnionAddr]; ok {
						signatureDH := util.SignRSA(publicDH, privateKey)
						newPrivMsg := &util.PrivateMessage{
							HsFlag:      util.ServerDHFwd,
							Cookie:      privateMessage.Cookie,
							PublicDH:    publicDH,
							SignatureDH: signatureDH,
							OnionAddr:   privateMessage.OnionAddr,
						}
						gossiper.HandlePrivateMessageToSend(co.RDVPoint, newPrivMsg)
					}
					gossiper.LHS.RUnlock()
				}
				//fmt.Println("ClientDH")
			} else if privateMessage.HsFlag == util.ServerDH {
				//fmt.Println("ServerDH")
				gossiper.connectionsToHS.Lock()
				defer gossiper.connectionsToHS.Unlock()

				if onionAddr, ok := gossiper.connectionsToHS.CookiesToAddr[privateMessage.Cookie]; ok {
					gossiper.LHS.RLock()
					if descriptor, ok := gossiper.LHS.HashMap[privateMessage.OnionAddr]; ok {
						pK := descriptor.PublicKey
						pKRSA, err := x509.ParsePKCS1PublicKey(pK)
						util.CheckError(err)
						util.VerifyRSASignature(privateMessage.PublicDH, privateMessage.SignatureDH, pKRSA)
						co := gossiper.connectionsToHS.Connections[onionAddr]
						co.SharedKey = util.CreateDHSharedKey(privateMessage.PublicDH, co.PrivateDH)
						fmt.Printf("CONNECTION to %s established\n", onionAddr)

						encryption, nonce := util.EncryptGCM([]byte("GET "+privateMessage.OnionAddr), co.SharedKey)
						newPrivMsg := &util.PrivateMessage{
							HsFlag:        util.HTTPFwd,
							Cookie:        privateMessage.Cookie,
							OnionAddr:     privateMessage.OnionAddr,
							GCMEncryption: encryption,
							GCMNonce:      nonce,
						}
						gossiper.HandlePrivateMessageToSend(co.RDVPoint, newPrivMsg)
					}
					gossiper.LHS.RUnlock()
				}
				//fmt.Println("ServerDH")
			} else if privateMessage.HsFlag == util.HTTP {
				gossiper.hsCo.Lock()
				if conn, ok := gossiper.hsCo.hsCos[privateMessage.Cookie]; ok {
					plaintext := util.DecryptGCM(privateMessage.GCMEncryption, privateMessage.GCMNonce, conn.SharedKey)
					getRequest := "GET " + conn.OnionAddr
					if strings.Compare(string(plaintext), getRequest) == 0 {
						file, err := ioutil.ReadFile(util.HSHTML)
						util.CheckError(err)
						encryption, nonce := util.EncryptGCM(file, conn.SharedKey)
						newPrivMsg := &util.PrivateMessage{
							HsFlag:        util.HTTPRepFwd,
							Cookie:        privateMessage.Cookie,
							OnionAddr:     privateMessage.OnionAddr,
							GCMEncryption: encryption,
							GCMNonce:      nonce,
						}
						gossiper.HandlePrivateMessageToSend(conn.RDVPoint, newPrivMsg)
					}
				}
				gossiper.hsCo.Unlock()
			} else if privateMessage.HsFlag == util.HTTPRep {
				gossiper.connectionsToHS.Lock()
				defer gossiper.connectionsToHS.Unlock()

				if onionAddr, ok := gossiper.connectionsToHS.CookiesToAddr[privateMessage.Cookie]; ok {
					co := gossiper.connectionsToHS.Connections[onionAddr]
					plaintext := util.DecryptGCM(privateMessage.GCMEncryption, privateMessage.GCMNonce, co.SharedKey)
					html := string(plaintext)
					fmt.Println(html)
					gossiper.LHS.Lock()
					gossiper.LHS.HTMLForGui[onionAddr] = html
					gossiper.LHS.Unlock()
				}
			} else {
				// "Normal" private message
				//fmt.Println("ELSE")
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
