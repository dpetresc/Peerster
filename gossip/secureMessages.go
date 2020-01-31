package gossip

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"time"
)

const ExpirationDuration = time.Duration(3 * time.Minute)
const TimeoutDuration = time.Duration(10 * time.Second)
const HopLimit = 10
const maxRetry = 4

/*
 *	SecureBytesConsumer sends bytes through a secure channel. The channel is created if it does not exist yet.
 *
 *	bytes	[]byte is the data that must be sent.
 *	destination string is the destination of the message.
 */
func (gossiper *Gossiper) SecureBytesConsumer(bytes []byte, destination string) {

	gossiper.LConsensus.RLock()
	if _, ok := gossiper.LConsensus.NodesPublicKeys[destination]; !ok{
		fmt.Printf("NO PUBLIC KEY for %s was found\n", destination)
		gossiper.LConsensus.RUnlock()
		return
	}
	gossiper.LConsensus.RUnlock()

	gossiper.connections.Lock()
	defer gossiper.connections.Unlock()

	if tunnelId, ok := gossiper.connections.Conns[destination]; ok {
		tunnelId.TimeoutChan <- true

		if tunnelId.NextPacket != util.Data {
			tunnelId.Pending = append(tunnelId.Pending, bytes)
			fmt.Println("PENDING message received from client")
		} else {
			gossiper.sendSecureMessage(bytes, tunnelId, destination)

		}
	} else {

		//Create a new connection
		nonce := make([]byte, 32)
		_, err := rand.Read(nonce)
		util.CheckError(err)
		newTunnelId := &TunnelIdentifier{
			TimeoutChan:   make(chan bool),
			Nonce:         nonce,
			NextPacket:    util.ServerHello,
			Pending:       make([][]byte, 0, 1),
			CTR:           0,
			TimeOut:       TimeoutDuration,
			ACKs:          make(map[uint32]chan bool),
			ConsecutiveTO: 0,
			NextID:        0,
			ToDeliver:     make([]*util.SecureMessage, 0),
		}
		newTunnelId.Pending = append(newTunnelId.Pending, bytes)
		gossiper.connections.Conns[destination] = newTunnelId

		secureMessage := &util.SecureMessage{
			MessageType: util.ClientHello,
			Nonce:       nonce,
			Origin:      gossiper.Name,
			Destination: destination,
			HopLimit:    HopLimit,
		}

		newTunnelId.HandShakeMessages = append(newTunnelId.HandShakeMessages, secureMessage)

		go gossiper.setExpirationTimeout(destination, newTunnelId)
		//fmt.Printf("CONNECTION with %s opened\n", destination)
		gossiper.HandleSecureMessage(secureMessage)
		go gossiper.startTimer(secureMessage)

	}
}

/*
*	sendSecureMessage sends a secure message of type Data created from a Message.
*	The payload has the following format: bytes(Nonce)||bytes(CTR)||bytes(text)
*	where Nonce is 32 bytes long, CTR is 4 bytes long and text has a variable length.
 */
func (gossiper *Gossiper) sendSecureMessage(bytesData []byte, tunnelId *TunnelIdentifier, destination string) {

	toEncrypt := make([]byte, 0)
	toEncrypt = append(toEncrypt, tunnelId.Nonce...)
	ctrBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(ctrBytes, tunnelId.CTR)
	toEncrypt = append(toEncrypt, ctrBytes...)
	toEncrypt = append(toEncrypt, bytesData...)

	ciphertext, nonceGCM := util.EncryptGCM(toEncrypt, tunnelId.SharedKey)
	secMsg := &util.SecureMessage{
		MessageType:   util.Data,
		Nonce:         tunnelId.Nonce,
		EncryptedData: ciphertext,
		GCMNonce:      nonceGCM,
		Origin:        gossiper.Name,
		Destination:   destination,
		HopLimit:      HopLimit,
		CTR:           tunnelId.CTR,
	}
	tunnelId.CTR += 1
	gossiper.HandleSecureMessage(secMsg)
	go gossiper.startTimer(secMsg)
}

/*
 *	HandleSecureMessage handles the Tor messages coming from other peer.
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
			tunnelId.TimeoutChan <- true
			if tunnelId.NextPacket == secureMessage.MessageType && Equals(secureMessage.Nonce, tunnelId.Nonce) {
				switch secureMessage.MessageType {
				case util.ServerHello:
					//fmt.Println("HANDSHAKE ServerHello")
					tunnelId.HandShakeMessages = append(tunnelId.HandShakeMessages, secureMessage)
					gossiper.handleServerHello(secureMessage)
				case util.ChangeCipherSec:
					//fmt.Println("HANDSHAKE ChangeCipherSec")
					tunnelId.HandShakeMessages = append(tunnelId.HandShakeMessages, secureMessage)
					gossiper.handleChangeCipherSec(secureMessage)
				case util.ServerFinished:
					//fmt.Println("HANDSHAKE ServerFinished")
					tunnelId.HandShakeMessages = append(tunnelId.HandShakeMessages, secureMessage)
					gossiper.handleServerFinished(secureMessage)
				case util.ClientFinished:
					//fmt.Println("HANDSHAKE ClientFinished")
					tunnelId.HandShakeMessages = append(tunnelId.HandShakeMessages, secureMessage)
					gossiper.handleClientFinished(secureMessage)
				case util.ACKClientFinished:
					//fmt.Println("HANDSHAKE ACKClientFinished")
					gossiper.handleACKClientFinished(secureMessage)
				case util.Data:
					gossiper.handleData(secureMessage)

				}
			} else if secureMessage.MessageType == util.ACK {
				tunnelId.ACKs[secureMessage.CTR] <- true
			}

		} else if secureMessage.MessageType == util.ClientHello {
			//fmt.Println("HANDSHAKE ClientHello")
			gossiper.handleClientHello(secureMessage)
		}
	}
}

/*
 *	Equals check that two byte slices are equal, i.e., each byte at each position is equal in the two slices.
 */
func Equals(bytes1, bytes2 []byte) bool {
	if len(bytes1) != len(bytes2) {
		return false
	}

	for i := range bytes1 {
		if bytes1[i] != bytes2[i] {
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

	gossiper.LConsensus.RLock()
	if _, ok := gossiper.LConsensus.NodesPublicKeys[message.Origin]; !ok{
		fmt.Printf("NO PUBLIC KEY for %s was found\n", message.Origin)
		gossiper.LConsensus.RUnlock()
		return
	}
	gossiper.LConsensus.RUnlock()

	if message.Nonce != nil && len(message.Nonce) == 32 {
		//create new connection
		tunnelId := &TunnelIdentifier{
			TimeoutChan:       make(chan bool),
			Nonce:             message.Nonce,
			NextPacket:        util.ChangeCipherSec,
			Pending:           make([][]byte, 0),
			HandShakeMessages: make([]*util.SecureMessage, 0),
			CTR:               0,
			ACKs:              make(map[uint32]chan bool),
			ConsecutiveTO:     0,
			TimeOut:           TimeoutDuration,
			NextID:            0,
			ToDeliver:         make([]*util.SecureMessage, 0),
		}
		tunnelId.HandShakeMessages = append(tunnelId.HandShakeMessages, message)

		gossiper.connections.Conns[message.Origin] = tunnelId
		go gossiper.setExpirationTimeout(message.Origin, tunnelId)

		//fmt.Printf("CONNECTION with %s opened\n", message.Origin)

		privateDH, publicDH := util.CreateDHPartialKey()
		tunnelId.PrivateDH = privateDH

		gossiper.LConsensus.RLock()
		DHSignature := util.SignRSA(publicDH, gossiper.LConsensus.privateKey)
		gossiper.LConsensus.RUnlock()

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
		go gossiper.startTimer(response)

	}
}

/*
 *	handleServerHello handles the messages of the handshake. Notice that gossiper.connections
 *	must be locked at this point. Sends a ChangeCipherSec message.
 */
func (gossiper *Gossiper) handleServerHello(message *util.SecureMessage) {
	gossiper.LConsensus.RLock()
	defer gossiper.LConsensus.RUnlock()
	if publicKeyOrigin, ok := gossiper.LConsensus.NodesPublicKeys[message.Origin]; ok {
		if util.VerifyRSASignature(message.DHPublic, message.DHSignature, publicKeyOrigin) {
			tunnelId := gossiper.connections.Conns[message.Origin]
			tunnelId.ACKsHandshake <- true
			tunnelId.NextPacket = util.ServerFinished

			privateDH, publicDH := util.CreateDHPartialKey()
			tunnelId.PrivateDH = privateDH

			shaKeyShared := util.CreateDHSharedKey(message.DHPublic, privateDH)
			tunnelId.SharedKey = shaKeyShared

			DHSignature := util.SignRSA(publicDH, gossiper.LConsensus.privateKey)

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
			go gossiper.startTimer(response)
		}
	} else {
		fmt.Printf("No KEY for %s\n", message.Origin)
	}
}

/*
 *	handleChangeCipherSec handles the messages of the handshake. Notice that gossiper.connections
 *	must be locked at this point. Sends a ServerFinished message.
 */
func (gossiper *Gossiper) handleChangeCipherSec(message *util.SecureMessage) {
	gossiper.LConsensus.RLock()
	defer gossiper.LConsensus.RUnlock()

	if publicKeyOrigin, ok := gossiper.LConsensus.NodesPublicKeys[message.Origin]; ok {
		if util.VerifyRSASignature(message.DHPublic, message.DHSignature, publicKeyOrigin) {
			tunnelId := gossiper.connections.Conns[message.Origin]
			tunnelId.ACKsHandshake <- true
			tunnelId.NextPacket = util.ClientFinished

			shaKeyShared := util.CreateDHSharedKey(message.DHPublic, tunnelId.PrivateDH)
			tunnelId.SharedKey = shaKeyShared

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
			go gossiper.startTimer(response)
		}
	} else {
		fmt.Printf("No KEY for %s\n", message.Origin)
	}

}

/*
 *	handleServerFinished handles the messages of the handshake. Notice that gossiper.connections
 *	must be locked at this point. Sends a ClientFinished message.
 */
func (gossiper *Gossiper) handleServerFinished(message *util.SecureMessage) {
	tunnelId := gossiper.connections.Conns[message.Origin]
	if gossiper.checkFinishedMessages(message.EncryptedData, message.GCMNonce, tunnelId) {
		tunnelId.ACKsHandshake <- true
		tunnelId.NextPacket = util.ACKClientFinished
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
		go gossiper.startTimer(response)
	}
}

/*
 *	handleClientFinished handles the messages of the handshake. Notice that gossiper.connections
 *	must be locked at this point. Sends a ACKClientFinished message.
 */
func (gossiper *Gossiper) handleClientFinished(message *util.SecureMessage) {
	tunnelId := gossiper.connections.Conns[message.Origin]
	if gossiper.checkFinishedMessages(message.EncryptedData, message.GCMNonce, tunnelId) {
		tunnelId.NextPacket = util.Data

		ack := &util.SecureMessage{
			MessageType: util.ACKClientFinished,
			Nonce:       message.Nonce,
			Origin:      message.Destination,
			Destination: message.Origin,
			HopLimit:    HopLimit,
		}
		gossiper.HandleSecureMessage(ack)
		tunnelId.ACKsHandshake <- true
		close(tunnelId.ACKsHandshake)
		fmt.Printf("CONNECTION with %s opened\n", message.Origin)
	}
}

/*
 *	handleACKClientFinished handles the messages of the handshake. Notice that gossiper.connections
 *	must be locked at this point. Sends the pending messages.
 */
func (gossiper *Gossiper) handleACKClientFinished(message *util.SecureMessage) {
	tunnelId := gossiper.connections.Conns[message.Origin]
	tunnelId.ACKsHandshake <- true
	close(tunnelId.ACKsHandshake)
	fmt.Printf("CONNECTION with %s opened\n", message.Origin)
	tunnelId.NextPacket = util.Data
	for _, bytes := range tunnelId.Pending {
		gossiper.sendSecureMessage(bytes, tunnelId, message.Origin)
	}
}

/*
 *	handleData handles the Data secure message and forwards it to the upper layer (either PrivateMessage or TorMessage).
 *	It sends an acknowledgment to the source.
 */
func (gossiper *Gossiper) handleData(message *util.SecureMessage) {

	tunnelId := gossiper.connections.Conns[message.Origin]
	plaintext := util.DecryptGCM(message.EncryptedData, message.GCMNonce, tunnelId.SharedKey)

	receivedNonce := plaintext[:32]
	receivedCTRBytes := plaintext[32:36]
	receivedCTR := binary.LittleEndian.Uint32(receivedCTRBytes)

	if Equals(receivedNonce, message.Nonce) && message.CTR == receivedCTR {

		ack := &util.SecureMessage{
			MessageType: util.ACK,
			Origin:      gossiper.Name,
			Destination: message.Origin,
			HopLimit:    util.HopLimit,
			CTR:         message.CTR,
		}
		gossiper.HandleSecureMessage(ack)

		if message.CTR == tunnelId.NextID {
			gossiper.deliver(message, tunnelId)

			if len(tunnelId.ToDeliver) > 0 {
				for i, secMsg := range tunnelId.ToDeliver[1:] {
					if secMsg == nil {
						tunnelId.ToDeliver = tunnelId.ToDeliver[i+1:]
						return
					}
					gossiper.deliver(secMsg, tunnelId)
				}
			}

		} else if message.CTR > tunnelId.NextID {
			lastIndex := tunnelId.NextID + uint32(len(tunnelId.ToDeliver)) - 1

			if message.CTR < lastIndex {
				if el := tunnelId.ToDeliver[message.CTR]; el == nil {
					tunnelId.ToDeliver[message.CTR] = message
				}
			} else {
				toAdd := message.CTR - lastIndex

				for i := uint32(0); i < toAdd; i++ {
					tunnelId.ToDeliver = append(tunnelId.ToDeliver, nil)
				}
				tunnelId.ToDeliver[message.CTR-tunnelId.NextID] = message
			}
		}
	}
}

/*
 *	deliver delivers the message to the upper layer. No further check is done, hence the message must have been already
 * 	processed by handleData before the use of this function.
 */
func (gossiper *Gossiper) deliver(message *util.SecureMessage, tunnelId *TunnelIdentifier) {
	tunnelId.NextID += 1
	plaintext := util.DecryptGCM(message.EncryptedData, message.GCMNonce, tunnelId.SharedKey)
	if gossiper.Tor {
		gossiper.secureToTor(plaintext[36:], message.Origin)
	} else {
		gossiper.secureToPrivate(plaintext[36:], message.Origin)
	}
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
 *	i.e., either Client- or ServeFinished was correctly encrypted with the given nonce and the computed shared SharedKey.
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
			//fmt.Println(i, toEncrypt[i], plaintext[i])
			return false
		}
	}
	return true

}

/*
 *	setExpirationTimeout starts a new timer for the connection that was previously opened.
 *	When the timer expires the connection is closed and when a new message arrives the timer is reset.
 *
 * dest string is the other party of the connection.
 */
func (gossiper *Gossiper) setExpirationTimeout(dest string, id *TunnelIdentifier) {
	ticker := time.NewTicker(ExpirationDuration)
	for {
		select {
		case <-ticker.C:
			//time out, the connection expires.
			fmt.Printf("EXPIRED connection with %s\n", dest)
			ticker.Stop()
			gossiper.killConnection(id, dest)
			return
		case reset := <-id.TimeoutChan:
			//connection tunnel was used, reset the timer.
			if reset {
				ticker = time.NewTicker(ExpirationDuration)
			} else {
				ticker.Stop()
				fmt.Printf("TOO MANY TIMEOUT connection with %s aborted\n", dest)
				gossiper.killConnection(id, dest)
				return
			}
		}

	}
}

/*
 *	killConnection shut downs a secure tunnel.
 */
func (gossiper *Gossiper) killConnection(id *TunnelIdentifier, dest string) {
	gossiper.connections.Lock()
	close(id.TimeoutChan)
	delete(gossiper.connections.Conns, dest)
	gossiper.connections.Unlock()

}

/*
 *	startTimer starts a timer for each message. It starts with a value of TimeoutDuration seconds and each time it timeouts
 *	the timer doubles. This can be done only 4 times then the connection is aborted. A channel is opened that is used when
 *	an acknowledgment arrives to stop the timer.
 */
func (gossiper *Gossiper) startTimer(message *util.SecureMessage) {

	gossiper.connections.Lock()
	tunnelId := gossiper.connections.Conns[message.Destination]
	ticker := time.NewTicker(tunnelId.TimeOut)
	var ackChan chan bool
	if message.MessageType < util.ChangeCipherSec {
		tunnelId.ACKsHandshake = make(chan bool)
		ackChan = tunnelId.ACKsHandshake
	} else if message.MessageType < util.ACKClientFinished {
		close(tunnelId.ACKsHandshake)
		tunnelId.ACKsHandshake = make(chan bool)
		ackChan = tunnelId.ACKsHandshake
	} else {
		tunnelId.ACKs[message.CTR] = make(chan bool)
		ackChan = tunnelId.ACKs[message.CTR]
	}
	gossiper.connections.Unlock()

	for {
		select {
		case <-ticker.C:

			gossiper.connections.Lock()
			tunnelId, ok := gossiper.connections.Conns[message.Destination]
			if ok {
				if message.MessageType == util.Data {
					fmt.Printf("TIMEOUT %d\n", message.CTR)

				} else {
					fmt.Printf("TIMEOUT for %s\n", message.MessageType.String())
				}

				tunnelId.ConsecutiveTO += 1
				if tunnelId.ConsecutiveTO > maxRetry {
					tunnelId.TimeoutChan <- false
					gossiper.connections.Unlock()
					ticker.Stop()
					return
				}
				tunnelId.TimeOut *= 2
				gossiper.HandleSecureMessage(message)
			} else {
				//Connection was already aborted previously
				ticker.Stop()
				gossiper.connections.Unlock()
				return
			}
			gossiper.connections.Unlock()
		case <-ackChan:

			ticker.Stop()
			gossiper.connections.Lock()
			tunnelId := gossiper.connections.Conns[message.Destination]
			tunnelId.ConsecutiveTO = 0

			if message.MessageType == util.Data {
				//fmt.Printf("ACK %d received\n", message.CTR)
				delete(tunnelId.ACKs, message.CTR)
				close(ackChan)
				gossiper.connections.Unlock()
				return

			} else {
				//fmt.Printf("ACK for %s\n", message.MessageType.String())
			}
			gossiper.connections.Unlock()
			return
		}
	}
}
