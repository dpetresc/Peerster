package main

import (
	"crypto/rsa"
	"encoding/json"
	"github.com/dpetresc/Peerster/util"
	"net/http"
	"sync"
)

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

func main() {
	privateKey = util.GetPrivateKey(util.KeysFolderPath, "")
	util.SavePublicKey(util.KeysFolderPath, &privateKey.PublicKey)
	nodesPublicKeys := make(map[string]*rsa.PublicKey)
	b, err := json.Marshal(nodesPublicKeys)
	util.CheckError(err)
	signature := util.SignByteMessage(b, privateKey)
	mConsensus = consensus{
		NodesIDPublicKeys: nodesPublicKeys,
		Signature:         signature,
		RWMutex:           sync.RWMutex{},
	}

	http.HandleFunc("/consensus", ConsensusHandler)
	http.HandleFunc("/subscription", SubscriptionHandler)

	for {
		err := http.ListenAndServe(util.CAAddress, nil)
		util.CheckError(err)
	}
}
