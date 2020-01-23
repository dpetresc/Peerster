package gossip

import (
	"github.com/dpetresc/Peerster/util"
	"sync"
)

/*
 *	TunnelIdentifier represents the secure tunnel between 2 peers.
 *	TimeoutChan chan bool channel used to indicates to the timer if it needs to be reset, i.e., each time a new message
 *	is sent through the secured tunnel.
 *	Nonce       []byte is the unique identifier of the tunnel.
 *	NextPacket	util.MessageType is the type of the next message that should be handled.
 *	Pending 	[]util.Message stores the messages sent by the client during the handshake, data messages during the
 *	handshake should be discarded.
 */
type TunnelIdentifier struct {
	TimeoutChan chan bool
	Nonce       []byte
	NextPacket	util.MessageType
	Pending 	[]*util.Message
}

/*
 *	Connections keeps in memory all the connections of a node. This structure is not meant to be inherently thread safe.
 *	Conns map[string]TunnelIdentifier is a mapping from the nodes's name to the tunnel identifier.
 */
type Connections struct {
	sync.RWMutex
	Conns map[string]TunnelIdentifier
}

/*
 *	NewConnections is a factory to create a *Connections.
 */
func NewConnections() *Connections{
	return &Connections{
		RWMutex: sync.RWMutex{},
		Conns:   make(map[string]TunnelIdentifier),
	}
}
