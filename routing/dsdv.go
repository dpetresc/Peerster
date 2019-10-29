package routing

import (
	"github.com/dpetresc/Peerster/util"
	"sync"
)

var LastPrivateMessages map[string][]*util.PrivateMessage = make(map[string][]*util.PrivateMessage)

type LockDsdv struct {
	Dsdv map[string]string
	LastIds map[string]uint32
	Origins []string
	Mutex sync.RWMutex
}

func NewDsdv() LockDsdv {
	dsdv := make(map[string]string)
	lastIds := make(map[string]uint32)
	return LockDsdv{
		Dsdv: dsdv,
		LastIds: lastIds,
		Origins: make([]string, 0),
		Mutex: sync.RWMutex{},
	}
}

func (l *LockDsdv) GetNextHopOrigin(origin string) string {
	l.Mutex.RLock()
	nextHop, ok := l.Dsdv[origin]
	l.Mutex.RUnlock()
	if ok {
		return nextHop
	}
	return ""
}

func (l *LockDsdv) getLastIDOrigin(origin string) uint32 {
	lastId, ok := l.LastIds[origin]
	if ok {
		return lastId
	}
	return 0
}

func (l *LockDsdv) UpdateOrigin(origin string, peer string, id uint32) {
	l.Mutex.Lock()
	idOrigin := l.getLastIDOrigin(origin)
	if id > idOrigin {
		if idOrigin == 0 {
			l.Origins = append(l.Origins, origin)
		}
		l.Dsdv[origin] = peer
		l.LastIds[origin] = id
	}
	l.Mutex.Unlock()
}

func AddNewPrivateMessageForGUI(key string, packet *util.PrivateMessage) {
	_, ok := LastPrivateMessages[key]
	if !ok {
		LastPrivateMessages[key] = make([]*util.PrivateMessage, 0)
	}
	LastPrivateMessages[key] = append(
		LastPrivateMessages[key], packet)
}
