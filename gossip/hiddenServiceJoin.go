package gossip

import (
	"errors"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"github.com/monnand/dhkx"
	"math/rand"
	"sync"
)

type ConnectionsToHS struct {
	sync.RWMutex
	// onion address => connection with the hidden service
	Connections   map[string]*ConnectionToHS
	CookiesToAddr map[uint64]string
}

type ConnectionToHS struct {
	RDVPoint  string
	Cookie    uint64
	PrivateDH *dhkx.DHKey
	SharedKey []byte
}

type ClientServerPair struct {
	Client uint32
	Server uint32
}

func (cl *ClientServerPair) Other(id uint32) uint32 {
	if cl.Client == id {
		return cl.Server
	} else if cl.Server == id {
		return cl.Client
	} else if cl.Client == cl.Server && cl.Client == id {
		return cl.Client
	} else {
		panic(errors.New("id is not in the pair"))
	}
}

type Bridges struct {
	sync.RWMutex
	ClientServerPairs map[uint64]*ClientServerPair //cookie to circuit id pairs
}

func NewBridges() *Bridges {
	return &Bridges{
		RWMutex:           sync.RWMutex{},
		ClientServerPairs: make(map[uint64]*ClientServerPair),
	}
}

func NewConnectionsToHS() *ConnectionsToHS {
	return &ConnectionsToHS{
		RWMutex:       sync.RWMutex{},
		Connections:   make(map[string]*ConnectionToHS),
		CookiesToAddr: make(map[uint64]string),
	}
}

/*
 *	JoinHS takes an onion address of a hidden service and perform a GET onionAddr command.
 */
func (gossiper *Gossiper) JoinHS(onionAddr string) {
	var descriptor *HSDescriptor
	gossiper.LHS.RLock()
	if d, ok := gossiper.LHS.HashMap[onionAddr]; ok {
		descriptor = d
	} else {
		gossiper.LHS.RUnlock()
		return
	}
	gossiper.LHS.RUnlock()

	gossiper.LConsensus.Lock()
	gossiper.lCircuits.Lock()
	gossiper.connectionsToHS.Lock()
	if _, ok := gossiper.connectionsToHS.Connections[onionAddr]; !ok {
		newConn := &ConnectionToHS{
			Cookie: rand.Uint64(),
		}
		gossiper.connectionsToHS.CookiesToAddr[newConn.Cookie] = onionAddr

		rdvPoint := gossiper.selectRandomNodeFromConsensus(descriptor.IPIdentity)
		if rdvPoint != "" {
			newConn.RDVPoint = rdvPoint

			gossiper.connectionsToHS.Connections[onionAddr] = newConn

			privateMsg := &util.PrivateMessage{
				HsFlag:     util.Bridge,
				IPIdentity: descriptor.IPIdentity,
				Cookie:     newConn.Cookie,
				OnionAddr:  onionAddr,
			}
			gossiper.HandlePrivateMessageToSend(rdvPoint, privateMsg)
		} else {
			fmt.Println("Consensus does not have enough nodes, retry later")
		}

	} else {
		fmt.Printf("CONNECTION to %s already exists\n", onionAddr)
	}
	gossiper.connectionsToHS.Unlock()
	gossiper.lCircuits.Unlock()
	gossiper.LConsensus.Unlock()
}
