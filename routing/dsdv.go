package routing

import (
	"fmt"
	"sync"
)

type LockDsdv struct {
	Dsdv map[string]string
	LastIds map[string]uint32
	Origins []string
	sync.RWMutex
}

func NewDsdv() LockDsdv {
	dsdv := make(map[string]string)
	lastIds := make(map[string]uint32)
	return LockDsdv{
		Dsdv: dsdv,
		LastIds: lastIds,
		Origins: make([]string, 0),
	}
}

func (l *LockDsdv) GetNextHopOrigin(origin string) string {
	l.RLock()
	nextHop, ok := l.Dsdv[origin]
	l.RUnlock()
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

func (l *LockDsdv) UpdateOrigin(origin string, peer string, id uint32, routeRumor bool) {
	l.Lock()
	idOrigin := l.getLastIDOrigin(origin)
	if id > idOrigin {
		if idOrigin == 0 {
			l.Origins = append(l.Origins, origin)
		}
		if !routeRumor {
			fmt.Printf("DSDV %s %s\n", origin, peer)
		}
		l.Dsdv[origin] = peer
		l.LastIds[origin] = id
	}
	l.Unlock()
}