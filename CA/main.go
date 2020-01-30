package main

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"net/http"
	"sync"
	"time"
)

/*
 * AllNodesIDPublicKeys the nodes and their publick key that are taken into account in this consensus timelapse
 * Signature	the signature of the consensus list
 */
type consensus struct {
	NodesIDPublicKeys map[string]*rsa.PublicKey
	Signature         []byte
	sync.RWMutex
}

/*
 * AllNodesIDPublicKeys all the nodes with their public key that were in the consensus in the past and
 * the new ones subscribing during this consensus timelapse
 * NodesRunning the nodes still runing that pinged the consensus during this consensus timelapse
 */
type consensusTracking struct {
	AllNodesIDPublicKeys map[string]*rsa.PublicKey
	NodesRunning         map[string]bool
	sync.RWMutex
}

type gossiperDescriptor struct {
	PublicKey []byte
	Identity  []byte
	Signature []byte
}

var privateKey *rsa.PrivateKey

var mConsensus consensus

var mConsensusTracking consensusTracking

func setConsensus(nodesPublicKeys map[string]*rsa.PublicKey) {
	b, err := json.Marshal(nodesPublicKeys)
	util.CheckError(err)
	signature := util.SignRSA(b, privateKey)
	mConsensus.NodesIDPublicKeys =  nodesPublicKeys
	mConsensus.Signature = signature
}

func (c consensus) String() string {
	if len(c.NodesIDPublicKeys) == 0 {
		return ""
	}
	s := ""
	for node,_ := range c.NodesIDPublicKeys {
		s += node + ", "
	}
	return s[:len(s)-2]
}

func updateConsensus() {
	ticker := time.NewTicker(time.Duration(util.ConsensusTimerMin) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			mConsensus.Lock()
			mConsensusTracking.RLock()
			currNodesPublicKeys := make(map[string]*rsa.PublicKey)
			for node, _ := range mConsensusTracking.NodesRunning {
				if key, ok := mConsensusTracking.AllNodesIDPublicKeys[node]; ok {
					currNodesPublicKeys[node] = key
				} else {
					// should never happen
					fmt.Println("PROBLEM")
				}
			}
			setConsensus(currNodesPublicKeys)
			mConsensusTracking.RUnlock()
			mConsensus.Unlock()
		}
	}
}

func main() {
	privateKey = util.GetPrivateKey(util.KeysFolderPath, "")
	util.SavePublicKey(util.KeysFolderPath, &privateKey.PublicKey)
	nodesPublicKeys := make(map[string]*rsa.PublicKey)
	setConsensus(nodesPublicKeys)
	mConsensusTracking = consensusTracking{
		AllNodesIDPublicKeys: make(map[string]*rsa.PublicKey),
		NodesRunning:         make(map[string]bool),
		RWMutex:              sync.RWMutex{},
	}

	go func() {
		http.HandleFunc("/consensus", ConsensusHandler)
		http.HandleFunc("/descriptor", DescriptorHandler)
		for {
			err := http.ListenAndServe(util.CAAddress, nil)
			util.CheckError(err)
		}
	}()

	updateConsensus()
}
