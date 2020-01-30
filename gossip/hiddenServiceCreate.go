package gossip

import (
	"github.com/dpetresc/Peerster/util"
	"sync"
)

type HSConnections struct {
	sync.RWMutex
	hsCos map[uint64]*HSConnetion
}

type HSConnetion struct{
	SharedKey []byte
	RDVPoint string
}

func NewHSConnections() *HSConnections{
	return &HSConnections{
		RWMutex:           sync.RWMutex{},
		CookieToSharedKey: make(map[uint64]*HSConnetion),
	}
}

func (gossiper *Gossiper) createHS(packet *util.Message) {
	//util.GetPrivateKey()
}