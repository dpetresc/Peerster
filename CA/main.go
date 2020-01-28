package main

import (
	"crypto/rsa"
	"encoding/json"
	"github.com/dpetresc/Peerster/util"
	"net/http"
	"sync"
	"time"
)

// TODO ajouter handle des noeuds qui crash
// TODO gérer le timer du consensus dans le CA
type consensus struct {
	NodesIDPublicKeys map[string]*rsa.PublicKey
	Signature         []byte
	sync.RWMutex
}

type gossiperDescriptor struct {
	PublicKey []byte
	Identity  []byte
	Signature []byte
}

var privateKey *rsa.PrivateKey

var mConsensus consensus

func setConsensus(nodesPublicKeys map[string]*rsa.PublicKey) {
	b, err := json.Marshal(nodesPublicKeys)
	util.CheckError(err)
	signature := util.SignRSA(b, privateKey)
	mConsensus = consensus{
		NodesIDPublicKeys: nodesPublicKeys,
		Signature:         signature,
	}
}

func updateConsensus() {
	ticker := time.NewTicker(time.Duration(util.ConsensusTimerMin) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			mConsensus.Lock()
			// TODO change setConsensus(mConsensus.NodesIDPublicKeys) to tempConsensus ???
			setConsensus(mConsensus.NodesIDPublicKeys)
			mConsensus.Unlock()
		}
	}
}

func main() {
	privateKey = util.GetPrivateKey(util.KeysFolderPath, "")
	util.SavePublicKey(util.KeysFolderPath, &privateKey.PublicKey)
	nodesPublicKeys := make(map[string]*rsa.PublicKey)
	setConsensus(nodesPublicKeys)

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
