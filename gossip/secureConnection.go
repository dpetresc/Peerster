package gossip

import (
	"github.com/dpetresc/Peerster/util"
	"github.com/monnand/dhkx"
	"sync"
	"time"
)

/*
 *	TunnelIdentifier represents the Tor tunnel between 2 peers.
 *	TimeoutChan chan bool channel used to indicates to the timer if it needs to be reset, i.e., each time a new message
 *	is sent through the secured tunnel.
 *	Nonce       []byte is the unique identifier of the tunnel.
 *	NextPacket	util.MessageType is the type of the next message that should be handled.
 *	Pending 	[]util.Message stores the messages sent by the client during the handshake, data messages during the
 *	handshake should be discarded.
 */
type TunnelIdentifier struct {
	TimeoutChan       chan bool
	Nonce             []byte
	NextPacket        util.MessageType
	Pending           [][]byte
	PrivateDH         *dhkx.DHKey
	SharedKey         []byte
	HandShakeMessages []*util.SecureMessage
	CTR               uint32
	TimeOut           time.Duration
	ACKsHandshake     chan bool
	ACKs              map[uint32]chan bool
	ConsecutiveTO     uint32
	ToDeliver         []*util.SecureMessage
	NextID            uint32
}

/*
 *	Connections keeps in memory all the connections of a node. This structure is not meant to be inherently thread safe.
 *	Conns map[string]TunnelIdentifier is a mapping from the nodes's name to the tunnel identifier.
 */
type Connections struct {
	sync.RWMutex
	Conns map[string]*TunnelIdentifier
}

/*
 *	NewConnections is a factory to create a *Connections.
 */
func NewConnections() *Connections {
	return &Connections{
		RWMutex: sync.RWMutex{},
		Conns:   make(map[string]*TunnelIdentifier),
	}
}
