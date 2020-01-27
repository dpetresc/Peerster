package gossip

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"github.com/monnand/dhkx"
	"time"
)

const TimeoutDuration = time.Duration(1 * time.Minute)
const HopLimit = 10

/*
 * 	HandleClientSecureMessage handles the messages coming from the client.
 *	message *util.Message is the message sent by the client.
 */
func (gossiper *Gossiper) HandleClientSecureMessage(message *util.Message) {
	dest := *message.Destination
	gossiper.connections.Lock()
	defer gossiper.connections.Unlock()

	if tunnelId, ok := gossiper.connections.Conns[dest]; ok {
		tunnelId.TimeoutChan <- true

		if tunnelId.NextPacket != util.Data {
			tunnelId.Pending = append(tunnelId.Pending, message)
			fmt.Println("PENDING message received from client")
		} else {
			gossiper.sendSecureMessage(message, tunnelId)
		}
	} else {

		//Create a new connection
		nonce := make([]byte, 32)
		_, err := rand.Read(nonce)
		util.CheckError(err)
		newTunnelId := &TunnelIdentifier{
			TimeoutChan: make(chan bool),
			Nonce:       nonce,
			NextPacket:  util.ServerHello,
			Pending:     make([]*util.Message, 0, 1),
			CTRSet:      make(map[uint32]bool),
			CTR:         0,
		}
		newTunnelId.Pending = append(newTunnelId.Pending, message)
		gossiper.connections.Conns[dest] = newTunnelId

		secureMessage := &util.SecureMessage{
			MessageType: util.ClientHello,
			Nonce:       nonce,
			Origin:      gossiper.Name,
			Destination: dest,
			HopLimit:    HopLimit,
		}

		newTunnelId.HandShakeMessages = append(newTunnelId.HandShakeMessages, secureMessage)

		go gossiper.setTimeout(dest, newTunnelId)
		fmt.Printf("CONNECTION with %s opened\n", dest)
		gossiper.HandleSecureMessage(secureMessage)

	}
}

/*
 *	sendSecureMessage sends a tor message of type Data created from a Message.
 *	The payload has the following format: bytes(Nonce)||bytes(CTR)||bytes(text)
 *	where Nonce is 32 bytes long, CTR is 4 bytes long and text has a variable length.
 */
func (gossiper *Gossiper) sendSecureMessage(message *util.Message, tunnelId *TunnelIdentifier) {

	toEncrypt := make([]byte,0)
	toEncrypt = append(toEncrypt, tunnelId.Nonce...)
	ctrBytes := make([]byte,4)
	binary.LittleEndian.PutUint32(ctrBytes, tunnelId.CTR)
	toEncrypt = append(toEncrypt, ctrBytes...)
	toEncrypt = append(toEncrypt, []byte(message.Text)...)

	ciphertext, nonceGCM := util.EncryptGCM(toEncrypt, tunnelId.SharedKey)
	secMsg := &util.SecureMessage{
		MessageType:   util.Data,
		Nonce:         tunnelId.Nonce,
		EncryptedData: ciphertext,
		GCMNonce:      nonceGCM,
		Origin:        gossiper.Name,
		Destination:   *message.Destination,
		HopLimit:      HopLimit,
	}
	tunnelId.CTR += 1
	gossiper.HandleSecureMessage(secMsg)
}

/*
 *	HandleSecureMessage handles the tor messages coming from other peer.
 *	secureMessage *util.SecureMessage is the message sent by the other peer.
 */
func (gossiper *Gossiper) HandleSecureMessage(secureMessage *util.SecureMessage) {
	if secureMessage.Destination != gossiper.Name {
		nextHop := gossiper.LDsdv.GetNextHopOrigin(secureMessage.Destination)
		// we have the next hop of this origin
		if nextHop != "" {
			hopValue := secureMessage.HopLimit
			if hopValue > 0 {
				secureMessage.HopLimit -= 1
				packetToForward := &util.GossipPacket{
					SecureMessage: secureMessage,
				}

				gossiper.sendPacketToPeer(nextHop, packetToForward)

			}
		}
	} else {
		//Discard all out of order packet!
		gossiper.connections.Lock()
		defer gossiper.connections.Unlock()
		if tunnelId, ok := gossiper.connections.Conns[secureMessage.Origin]; ok {
			if tunnelId.NextPacket == secureMessage.MessageType && Equals(secureMessage.Nonce, tunnelId.Nonce) {

				tunnelId.TimeoutChan <- true
				tunnelId.HandShakeMessages = append(tunnelId.HandShakeMessages, secureMessage)

				switch secureMessage.MessageType {
				case util.ServerHello:
					fmt.Println("HANDSHAKE ServerHello")
					gossiper.handleServerHello(secureMessage)
				case util.ChangeCipherSec:
					fmt.Println("HANDSHAKE ChangeCipherSec")
					gossiper.handleChangeCipherSec(secureMessage)
				case util.ServerFinished:
					fmt.Println("HANDSHAKE ServerFinished")
					gossiper.handleServerFinished(secureMessage)
				case util.ClientFinished:
					fmt.Println("HANDSHAKE ClientFinished")
					gossiper.handleClientFinished(secureMessage)
				case util.ACK:
					gossiper.handleACK(secureMessage)
				case util.Data:
					gossiper.handleData(secureMessage)

				}
			}

		} else if secureMessage.MessageType == util.ClientHello {
			fmt.Println("HANDSHAKE ClientHello")
			gossiper.handleClientHello(secureMessage)
		}
	}
}

func Equals(nonce1, nonce2 []byte) bool{
	if len(nonce1) != len(nonce2){
		return false
	}

	for i := range nonce1{
		if nonce1[i] != nonce2[i]{
			return false
		}
	}
	return true
}

/*
 *	handleClientHello handles the received ClientHello messages. Notice that gossiper.connections
 *	must be locked at this point. Sends a ServerHello message.
 */
func (gossiper *Gossiper) handleClientHello(message *util.SecureMessage) {

	if message.Nonce != nil && len(message.Nonce) == 32 {
		//create new connection

		tunnelId := &TunnelIdentifier{
			TimeoutChan:       make(chan bool),
			Nonce:             message.Nonce,
			NextPacket:        util.ChangeCipherSec,
			Pending:           make([]*util.Message, 0),
			HandShakeMessages: make([]*util.SecureMessage, 0),
			CTRSet:            make(map[uint32]bool),
			CTR:               0,
		}
		tunnelId.HandShakeMessages = append(tunnelId.HandShakeMessages, message)

		gossiper.connections.Conns[message.Origin] = tunnelId
		go gossiper.setTimeout(message.Origin, tunnelId)

		fmt.Printf("CONNECTION with %s opened\n", message.Origin)

		g, err := dhkx.GetGroup(0)
		util.CheckError(err)

		privateDH, err := g.GeneratePrivateKey(nil)
		util.CheckError(err)

		tunnelId.PrivateDH = privateDH

		publicDH := privateDH.Bytes()

		gossiper.lConsensus.RLock()
		DHSignature := util.SignByteMessage(publicDH, gossiper.lConsensus.privateKey)
		gossiper.lConsensus.RUnlock()

		response := &util.SecureMessage{
			MessageType: util.ServerHello,
			Nonce:       message.Nonce,
			DHPublic:    publicDH,
			DHSignature: DHSignature,
			Origin:      gossiper.Name,
			Destination: message.Origin,
			HopLimit:    HopLimit,
		}

		tunnelId.HandShakeMessages = append(tunnelId.HandShakeMessages, response)

		gossiper.HandleSecureMessage(response)

	}
}

/*
 *	handleServerHello handles the messages of the handshake. Notice that gossiper.connections
 *	must be locked at this point. Sends a ChangeCipherSec message.
 */
func (gossiper *Gossiper) handleServerHello(message *util.SecureMessage) {
	gossiper.lConsensus.RLock()
	if publicKeyOrigin, ok := gossiper.lConsensus.nodesPublicKeys[message.Origin]; ok {
		if util.VerifySignature(message.DHPublic, message.DHSignature, publicKeyOrigin) {
			tunnelId := gossiper.connections.Conns[message.Origin]
			tunnelId.NextPacket = util.ServerFinished

			g, err := dhkx.GetGroup(0)
			util.CheckError(err)

			privateDH, err := g.GeneratePrivateKey(nil)
			util.CheckError(err)
			tunnelId.PrivateDH = privateDH

			sharedKey, err := g.ComputeKey(dhkx.NewPublicKey(message.DHPublic), privateDH)
			util.CheckError(err)
			shaKeyShared := sha256.Sum256(sharedKey.Bytes())
			tunnelId.SharedKey = shaKeyShared[:]

			publicDH := privateDH.Bytes()
			DHSignature := util.SignByteMessage(publicDH, gossiper.lConsensus.privateKey)

			response := &util.SecureMessage{
				MessageType: util.ChangeCipherSec,
				Nonce:       message.Nonce,
				DHPublic:    publicDH,
				DHSignature: DHSignature,
				Origin:      gossiper.Name,
				Destination: message.Origin,
				HopLimit:    HopLimit,
			}

			tunnelId.HandShakeMessages = append(tunnelId.HandShakeMessages, response)

			gossiper.HandleSecureMessage(response)
		}
	}
	gossiper.lConsensus.RUnlock()

}

/*
 *	handleChangeCipherSec handles the messages of the handshake. Notice that gossiper.connections
 *	must be locked at this point. Sends a ServerFinished message.
 */
func (gossiper *Gossiper) handleChangeCipherSec(message *util.SecureMessage) {
	gossiper.lConsensus.RLock()
	defer gossiper.lConsensus.RUnlock()

	if publicKeyOrigin, ok := gossiper.lConsensus.nodesPublicKeys[message.Origin]; ok {
		if util.VerifySignature(message.DHPublic, message.DHSignature, publicKeyOrigin) {
			tunnelId := gossiper.connections.Conns[message.Origin]
			tunnelId.NextPacket = util.ClientFinished

			g, err := dhkx.GetGroup(0)
			util.CheckError(err)
			sharedKey, err := g.ComputeKey(dhkx.NewPublicKey(message.DHPublic), tunnelId.PrivateDH)
			util.CheckError(err)
			shaKeyShared := sha256.Sum256(sharedKey.Bytes())
			tunnelId.SharedKey = shaKeyShared[:]

			encryptedHandshake, nonceGCM := gossiper.encryptHandshake(tunnelId)

			response := &util.SecureMessage{
				MessageType:   util.ServerFinished,
				Nonce:         message.Nonce,
				EncryptedData: encryptedHandshake,
				GCMNonce:      nonceGCM,
				Origin:        gossiper.Name,
				Destination:   message.Origin,
				HopLimit:      HopLimit,
			}
			tunnelId.HandShakeMessages = append(tunnelId.HandShakeMessages, response)
			gossiper.HandleSecureMessage(response)
		}
	}

}

/*
 *	handleServerFinished handles the messages of the handshake. Notice that gossiper.connections
 *	must be locked at this point. Sends a ClientFinished message.
 */
func (gossiper *Gossiper) handleServerFinished(message *util.SecureMessage) {
	tunnelId := gossiper.connections.Conns[message.Origin]
	if gossiper.checkFinishedMessages(message.EncryptedData, message.GCMNonce, tunnelId) {
		tunnelId.NextPacket = util.ACK

		encryptedHandshake, nonceGCM := gossiper.encryptHandshake(tunnelId)

		response := &util.SecureMessage{
			MessageType:   util.ClientFinished,
			Nonce:         message.Nonce,
			EncryptedData: encryptedHandshake,
			GCMNonce:      nonceGCM,
			Origin:        gossiper.Name,
			Destination:   message.Origin,
			HopLimit:      HopLimit,
		}
		tunnelId.HandShakeMessages = append(tunnelId.HandShakeMessages, response)
		gossiper.HandleSecureMessage(response)
	}
}

func (gossiper *Gossiper) handleClientFinished(message *util.SecureMessage) {
	tunnelId := gossiper.connections.Conns[message.Origin]
	if gossiper.checkFinishedMessages(message.EncryptedData, message.GCMNonce, tunnelId) {
		tunnelId.NextPacket = util.Data

		ack := &util.SecureMessage{
			MessageType: util.ACK,
			Nonce:       message.Nonce,
			Origin:      message.Destination,
			Destination: message.Origin,
			HopLimit:    HopLimit,
		}
		gossiper.HandleSecureMessage(ack)

	}
}

func (gossiper *Gossiper) handleACK(message *util.SecureMessage) {
	tunnelId := gossiper.connections.Conns[message.Origin]
	tunnelId.NextPacket = util.Data
	for _, msg := range tunnelId.Pending {
		gossiper.sendSecureMessage(msg, tunnelId)
	}
}

func (gossiper *Gossiper) handleData(message *util.SecureMessage) {


	tunnelId := gossiper.connections.Conns[message.Origin]
	plaintext := util.DecryptGCM(message.EncryptedData, message.GCMNonce, tunnelId.SharedKey)

	receivedNonce := plaintext[:32]
	receivedCTRBytes := plaintext[32:36]
	receivedCTR := binary.LittleEndian.Uint32(receivedCTRBytes)

	if _,ok := tunnelId.CTRSet[receivedCTR]; ok || !Equals(receivedNonce, message.Nonce){
		return
	}else{
		tunnelId.CTRSet[receivedCTR] = true
	}

	receivedText := plaintext[36:]

	privMessage := &util.PrivateMessage{
		Origin:      message.Origin,
		ID:          0,
		Text:        string(receivedText),
		Destination: message.Destination,
		HopLimit:    message.HopLimit,
	}

	gossiper.handlePrivatePacket(&util.GossipPacket{
		Private: privMessage,
	})
}

/*
 *	encryptHandshake encrypts the messages from the handshake.
 *	ServerFinished: Enc(ClientHello||ServerHello||ChangeCipherSec)
 *	ClientFinished:	Enc(ClientHello||ServerHello||ChangeCipherSec||ServerFinished)
 */
func (gossiper *Gossiper) encryptHandshake(tunnelId *TunnelIdentifier) ([]byte, []byte) {
	toEncrypt := make([]byte, 0)
	for _, msg := range tunnelId.HandShakeMessages {
		//fmt.Println(msg)
		toEncrypt = append(toEncrypt, msg.Bytes()...)
	}
	encryptedHandshake, nonceGCM := util.EncryptGCM(toEncrypt, tunnelId.SharedKey)
	return encryptedHandshake, nonceGCM
}

/*
 *	checkFinishedMessages verifies that the received encrypted data in a finished message,
 *	i.e., either Client- or ServeFinished was correctly encrypted with the given nonce and the computed shared key.
 */
func (gossiper *Gossiper) checkFinishedMessages(ciphertext, nonce []byte, tunnelId *TunnelIdentifier) bool {
	toEncrypt := make([]byte, 0)
	for _, msg := range tunnelId.HandShakeMessages[:len(tunnelId.HandShakeMessages)-1] {
		//fmt.Println(msg)
		toEncrypt = append(toEncrypt, msg.Bytes()...)
	}

	plaintext := util.DecryptGCM(ciphertext, nonce, tunnelId.SharedKey)

	for i := range toEncrypt {
		if toEncrypt[i] != plaintext[i] {
			fmt.Println(i, toEncrypt[i], plaintext[i])
			return false
		}
	}
	return true

}

/*
 *	setTimeout starts a new timer for the connection that was previously opened.
 *	When the timer expires the connection is closed and when a new message arrives the timer is reset.
 *
 * dest string is the other party of the connection.
 */
func (gossiper *Gossiper) setTimeout(dest string, id *TunnelIdentifier) {
	ticker := time.NewTicker(TimeoutDuration)
	for {
		select {
		case <-ticker.C:
			//time out, the connection expires.
			ticker.Stop()
			gossiper.connections.Lock()
			close(id.TimeoutChan)
			delete(gossiper.connections.Conns, dest)
			gossiper.connections.Unlock()
			fmt.Printf("EXPIRED connection with %s\n", dest)
			return
		case <-id.TimeoutChan:
			//connection tunnel was used, reset the timer.
			ticker = time.NewTicker(TimeoutDuration)

		}

	}
}
