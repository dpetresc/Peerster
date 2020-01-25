package gossip

import (
	"crypto/rand"
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
	dest := *message.Destination

	gossiper.connections.RLock()
	if tunnelId, ok := gossiper.connections.Conns[dest]; ok {
		gossiper.connections.RUnlock()

		tunnelId.TimeoutChan <- true
		//TODO handle the fact that the handshake may not be finished
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
			Pending:	make([]*util.Message,0,1),
		}
		newTunnelId.Pending = append(newTunnelId.Pending, message)
		gossiper.connections.Conns[dest] = newTunnelId
		gossiper.connections.Unlock()
		go gossiper.setTimeout(dest, &newTunnelId)
		
		//Send first message of the handshake.
		secureMessage := &util.SecureMessage{
			MessageType: util.ClientHello,
			Nonce:       nonce,
			Origin:      gossiper.Name,
			Text:        dest,
			Destination: "",
			HopLimit:    HopLimit,
		}
		gossiper.HandleSecureMessage(secureMessage)


	}
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
			conn.TimeoutChan <- true
			if conn.NextPacket == secureMessage.MessageType{
				switch secureMessage.MessageType {
				case util.ServerHello:
					gossiper.handleServerHello(secureMessage)
				case util.ChangeCipherSec:
					gossiper.handleChangeCipherSec(secureMessage)
				case util.ServerFinished:
					break
				case util.ClientFinished:
					break
				case util.Data:
					break

				}
			}

		}else if secureMessage.MessageType == util.ClientHello{
			gossiper.handleClientHello(secureMessage)
		}
	}
}

/*
 *	handleClientHello handles the received ClientHello messages. Notice that gossiper.connections
 *	must be locked at this point. Sends a ChangeCipherSec message.
 */
func (gossiper *Gossiper) handleClientHello(message *util.SecureMessage) {

	if message.Nonce != nil && len(message.Nonce) == 32{
		//create new connection
		tunnelId := TunnelIdentifier{
			TimeoutChan: make(chan bool),
			Nonce:       message.Nonce,
			NextPacket:  util.ChangeCipherSec,
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
 *	must be locked at this point. Sends a ServerHello message.
 */
func (gossiper *Gossiper) handleServerHello(message *util.SecureMessage) {
	if util.Verify(message.DHPublic, message.DHSignature, util.GetPublicKey(message.Origin)){
		tunnelId := gossiper.connections.Conns[message.Origin]

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
			MessageType: util.ServerHello,
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

func (gossiper *Gossiper) handleChangeCipherSec(message *util.SecureMessage) {

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


