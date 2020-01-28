package main

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"github.com/dpetresc/Peerster/util"
	"net/http"
)

func ConsensusHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "GET":
		enableCors(&w)
		jsonConsensus, err := json.Marshal(mConsensus)
		util.CheckError(err)
		w.WriteHeader(http.StatusOK)
		w.Write(jsonConsensus)
	}
}

func SubscriptionHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "POST":
		var descriptor gossiperDescriptor
		err := json.NewDecoder(r.Body).Decode(&descriptor)
		util.CheckError(err)

		newNodeIdentityStr := string(descriptor.Identity)
		newNode := append(descriptor.PublicKey[:], descriptor.Identity[:]...)
		newNodeRSAPublicKey, err := x509.ParsePKCS1PublicKey(descriptor.PublicKey)
		util.CheckError(err)

		if !util.VerifyRSASignature(newNode, descriptor.Signature, newNodeRSAPublicKey) {
			http.Error(w, "Signature isn't correct !", http.StatusUnauthorized)
			return
		}
		mConsensus.Lock()
		if key, ok := mConsensus.NodesIDPublicKeys[newNodeIdentityStr]; ok {
			if !bytes.Equal(x509.MarshalPKCS1PublicKey(key), descriptor.PublicKey) {
				http.Error(w, "Identity already used !", http.StatusUnauthorized)
				mConsensus.Unlock()
				return
			}
		} else {
			for _, key := range mConsensus.NodesIDPublicKeys {
				if bytes.Equal(x509.MarshalPKCS1PublicKey(key), descriptor.PublicKey) {
					http.Error(w, "Public Key already exists in consensus !", http.StatusUnauthorized)
					mConsensus.Unlock()
					return
				}
			}
		}
		mConsensus.NodesIDPublicKeys[newNodeIdentityStr] = newNodeRSAPublicKey
		b, err := json.Marshal(mConsensus.NodesIDPublicKeys)
		util.CheckError(err)
		mConsensus.Signature = util.SignRSA(b, privateKey)
		mConsensus.Unlock()
	}
}

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
}
