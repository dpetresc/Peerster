package main

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"net/http"
)

func ConsensusHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "GET":
		enableCors(&w)
		nodes, ok := r.URL.Query()["node"]
		if !ok || len(nodes[0]) < 1 {
			http.Error(w, "Please specify your node name to get the consensus !", http.StatusUnauthorized)
			return
		}
		mConsensusTracking.Lock()
		mConsensusTracking.NodesRunning[string(nodes[0])] = true
		mConsensusTracking.Unlock()
		mConsensus.RLock()
		jsonConsensus, err := json.Marshal(mConsensus)
		fmt.Println(mConsensus)
		mConsensus.RUnlock()
		util.CheckError(err)
		w.WriteHeader(http.StatusOK)
		w.Write(jsonConsensus)
	}
}

func DescriptorHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "POST":
		var descriptor gossiperDescriptor
		err := json.NewDecoder(r.Body).Decode(&descriptor)
		util.CheckError(err)

		newNodeIdentityStr := string(descriptor.Identity)
		fmt.Println("Receive Descriptor for " + newNodeIdentityStr)
		newNode := append(descriptor.PublicKey[:], descriptor.Identity[:]...)
		newNodeRSAPublicKey, err := x509.ParsePKCS1PublicKey(descriptor.PublicKey)
		util.CheckError(err)

		if !util.VerifyRSASignature(newNode, descriptor.Signature, newNodeRSAPublicKey) {
			http.Error(w, "Signature isn't correct !", http.StatusUnauthorized)
			return
		}

		mConsensusTracking.Lock()
		if key, ok := mConsensusTracking.AllNodesIDPublicKeys[newNodeIdentityStr]; ok {
			if !bytes.Equal(x509.MarshalPKCS1PublicKey(key), descriptor.PublicKey) {
				http.Error(w, "Identity has already been used !", http.StatusUnauthorized)
				mConsensusTracking.Unlock()
				return
			} else {
				mConsensusTracking.NodesRunning[newNodeIdentityStr] = true
			}
		} else {
			for _, key := range mConsensusTracking.AllNodesIDPublicKeys {
				if bytes.Equal(x509.MarshalPKCS1PublicKey(key), descriptor.PublicKey) {
					http.Error(w, "Public SharedKey has already exist in consensus !", http.StatusUnauthorized)
					mConsensusTracking.Unlock()
					return
				}
			}
			mConsensusTracking.NodesRunning[newNodeIdentityStr] = true
			// new node that have not been previously registered
			mConsensusTracking.AllNodesIDPublicKeys[newNodeIdentityStr] = newNodeRSAPublicKey
		}
		mConsensusTracking.Unlock()
	}
}

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
}
