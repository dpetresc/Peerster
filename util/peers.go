package util

import (
	"fmt"
	"strings"
)


type Peers struct {
	PeersMap 	*map[string]bool
}

func NewPeers(peers string) *Peers{
	var peersArray []string
	if peers != "" {
		peersArray = strings.Split(peers, ",")
	}else {
		peersArray = []string{}
	}
	peersMap := make(map[string]bool)
	for i := 0; i < len(peersArray); i+=1 {
		peersMap[peersArray[i]] = true
	}
	return &Peers{
		PeersMap: &peersMap,
	}
}

func (peers *Peers) AddPeer(addr string) {
	_, ok := (*peers.PeersMap)[addr]
	if !ok {
		(*peers.PeersMap)[addr] = false
	}
}

func (peers *Peers) PrintPeers() {
	fmt.Print("PEERS ")
	keys := make([]string, 0, len(*peers.PeersMap))
	for k := range *peers.PeersMap {
		keys = append(keys, k)
	}
	for _, peer := range keys[:len(keys)-1] {
		fmt.Print(peer + ",")
	}
	fmt.Println(keys[len(keys)-1])
}