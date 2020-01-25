package gossip

import (
	"crypto/rand"
	"encoding/json"
	"github.com/dpetresc/Peerster/util"
	"github.com/monnand/dhkx"
	"time"
)

const TimeoutDuration  = time.Duration(10*time.Second)
const HopLimit = 10

/*
 * 	HandleClientSecureMessage handles the messages coming from the client.
 *	message *util.Message is the message sent by the client.
 */
func (gossiper *Gossiper) HandleClientSecureMessage(message *util.Message) {
	//TODO verifier les locks
	dest := *message.Destination

	gossiper.connections.RLock()
	if tunnelId, ok := gossiper.connections.Conns[dest]; ok {
		tunnelId.TimeoutChan <- true
		gossiper.connections.RUnlock()

		if tunnelId.NextPacket != util.Data{
			gossiper.connections.Lock()
			tunnelId.Pending = append(tunnelId.Pending, message)
			gossiper.connections.Unlock()
		}else{
			gossiper.sendSecureMessage(message, tunnelId)
		}
	} else {
		gossiper.connections.RUnlock()

		//Create a new connection
		nonce := make([]byte, 32)
		_, err := rand.Read(nonce)
		util.CheckError(err)
		gossiper.connections.Lock()
		newTunnelId := TunnelIdentifier{
			TimeoutChan: make(chan bool),
			Nonce:       nonce,
			NextPacket:  util.ServerHello,
			Pending:     make([]*util.Message, 0, 1),
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

		gossiper.connections.Unlock()
		go gossiper.setTimeout(dest, &newTunnelId)

		gossiper.HandleSecureMessage(secureMessage)

	}
}

func (gossiper *Gossiper) sendSecureMessage(message *util.Message, tunnelId TunnelIdentifier) {
	ciphertext, nonceGCM := util.EncryptGCM([]byte(message.Text), tunnelId.SharedKey.Bytes())
	secMsg := &util.SecureMessage{
		MessageType:   util.Data,
		Nonce:         tunnelId.Nonce,
		EncryptedData: ciphertext,
		GCMNonce:      nonceGCM,
		Origin:        gossiper.Name,
		Destination:   *message.Destination,
		HopLimit:      HopLimit,
	}
	gossiper.HandleSecureMessage(secMsg)
}

/*
 *	HandleSecureMessage handles the secure messages coming from other peer.
 *	secureMessage *util.SecureMessage is the message sent by the other peer.
 */
func (gossiper *Gossiper) HandleSecureMessage(secureMessage *util.SecureMessage){
	if secureMessage.Destination != gossiper.Name{
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
	}else{
		//Discard all out of order packet!
		gossiper.connections.Lock()
		defer gossiper.connections.Unlock()
		if conn, ok := gossiper.connections.Conns[secureMessage.Origin]; ok{
			if conn.NextPacket == secureMessage.MessageType{

				conn.TimeoutChan <- true
				conn.HandShakeMessages = append(conn.HandShakeMessages, secureMessage)

				switch secureMessage.MessageType {
				case util.ServerHello:
					gossiper.handleServerHello(secureMessage)
				case util.ChangeCipherSec:
					gossiper.handleChangeCipherSec(secureMessage)
				case util.ServerFinished:
					gossiper.handleServerFinished(secureMessage)
				case util.ClientFinished:
					gossiper.handleClientFinished(secureMessage)
				case util.Data:
					gossiper.handleData(secureMessage)

				}
			}

		}else if secureMessage.MessageType == util.ClientHello{
			gossiper.handleClientHello(secureMessage)
		}
	}
}

/*
 *	handleClientHello handles the received ClientHello messages. Notice that gossiper.connections
 *	must be locked at this point. Sends a ServerHello message.
 */
func (gossiper *Gossiper) handleClientHello(message *util.SecureMessage) {

	if message.Nonce != nil && len(message.Nonce) == 32{
		//create new connection
		tunnelId := TunnelIdentifier{
			TimeoutChan: make(chan bool),
			Nonce:       message.Nonce,
			NextPacket:  util.ServerHello,
			Pending:     make([]*util.Message,0),
		}

		gossiper.connections.Conns[message.Origin] = tunnelId
		go gossiper.setTimeout(message.Origin, &tunnelId)

		g, err := dhkx.GetGroup(0)
		util.CheckError(err)

		privateDH , err := g.GeneratePrivateKey(nil)
		util.CheckError(err)

		tunnelId.PrivateDH = privateDH

		publicDH:= privateDH.Bytes()

		DHSignature := util.Sign(publicDH, util.GetPrivateKey(gossiper.Name))

		response := &util.SecureMessage{
			MessageType: util.ChangeCipherSec,
			Nonce:       message.Nonce,
			DHPublic:    publicDH,
			DHSignature: DHSignature,
			Origin:      gossiper.Name,
			Destination: message.Origin,
			HopLimit:    HopLimit,
		}

		gossiper.HandleSecureMessage(response)

	}
}

/*
 *	handleServerHello handles the messages of the handshake. Notice that gossiper.connections
 *	must be locked at this point. Sends a ChangeCipherSec message.
 */
func (gossiper *Gossiper) handleServerHello(message *util.SecureMessage) {
	if util.Verify(message.DHPublic, message.DHSignature, util.GetPublicKey(message.Origin)){
		tunnelId := gossiper.connections.Conns[message.Origin]
		tunnelId.NextPacket = util.ServerFinished

		g, err := dhkx.GetGroup(0)
		util.CheckError(err)

		privateDH , err := g.GeneratePrivateKey(nil)
		util.CheckError(err)
		tunnelId.PrivateDH = privateDH

		sharedKey, err := g.ComputeKey(dhkx.NewPublicKey(message.DHPublic), privateDH)
		util.CheckError(err)
		tunnelId.SharedKey = sharedKey

		publicDH:= privateDH.Bytes()
		DHSignature := util.Sign(publicDH, util.GetPrivateKey(gossiper.Name))

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

/*
 *	handleChangeCipherSec handles the messages of the handshake. Notice that gossiper.connections
 *	must be locked at this point. Sends a ServerFinished message.
 */
func (gossiper *Gossiper) handleChangeCipherSec(message *util.SecureMessage) {
	if util.Verify(message.DHPublic, message.DHSignature, util.GetPublicKey(message.Origin)) {
		tunnelId := gossiper.connections.Conns[message.Origin]
		tunnelId.NextPacket = util.ClientFinished

		g, err := dhkx.GetGroup(0)
		util.CheckError(err)
		sharedKey, err := g.ComputeKey(dhkx.NewPublicKey(message.DHPublic), tunnelId.PrivateDH)
		util.CheckError(err)
		tunnelId.SharedKey = sharedKey

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


/*
 *	handleServerFinished handles the messages of the handshake. Notice that gossiper.connections
 *	must be locked at this point. Sends a ClientFinished message.
 */
func (gossiper *Gossiper) handleServerFinished(message *util.SecureMessage) {
	tunnelId := gossiper.connections.Conns[message.Origin]
	if gossiper.checkFinishedMessages(message.EncryptedData, message.GCMNonce, tunnelId){
		tunnelId.NextPacket = util.Data

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
		for _, msg := range tunnelId.Pending{
			gossiper.sendSecureMessage(msg, tunnelId)
		}
	}
}

func (gossiper *Gossiper) handleData(message *util.SecureMessage) {
	tunnelId := gossiper.connections.Conns[message.Origin]
	plaintext := util.DecryptGCM(message.EncryptedData, message.GCMNonce, tunnelId.SharedKey.Bytes())

	privMessage := &util.PrivateMessage{
		Origin:      message.Origin,
		ID:          0,
		Text:        string(plaintext),
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
func (gossiper *Gossiper) encryptHandshake(tunnelId TunnelIdentifier) ([]byte, []byte) {
	toEncrypt := make([]byte, 0)
	for _, msg := range tunnelId.HandShakeMessages {
		bytes, err := json.Marshal(msg)
		util.CheckError(err)

		toEncrypt = append(toEncrypt, bytes...)
	}
	encryptedHandshake, nonceGCM := util.EncryptGCM(toEncrypt, tunnelId.SharedKey.Bytes())
	return encryptedHandshake, nonceGCM
}

/*
 *	checkFinishedMessages verifies that the received encrypted data in a finished message,
 *	i.e., either Client- or ServeFinished was correctly encrypted with the given nonce and the computed shared key.
 */
func (gossiper *Gossiper) checkFinishedMessages(ciphertext, nonce []byte, tunnelId TunnelIdentifier) bool{
	toEncrypt := make([]byte,0)
	for _, msg := range tunnelId.HandShakeMessages[:len(tunnelId.HandShakeMessages)-1]{
		bytes, err := json.Marshal(msg)
		util.CheckError(err)
		toEncrypt = append(toEncrypt, bytes...)
	}
	
	plaintext := util.DecryptGCM(ciphertext, nonce, tunnelId.SharedKey.Bytes())
	for i := range toEncrypt{
		if toEncrypt[i] != plaintext[i]{
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
	for{
		select {
			case <- ticker.C:
				//time out, the connection expires.
				ticker.Stop()
				gossiper.connections.Lock()
				close(id.TimeoutChan)
				delete(gossiper.connections.Conns, dest)
				gossiper.connections.Unlock()
				return
			case <- id.TimeoutChan:
				//connection tunnel was used, reset the timer.
				ticker = time.NewTicker(TimeoutDuration)

		}

	}
}


