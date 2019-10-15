package main

import (
	"encoding/json"
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/util"
	"net/http"
)

func RumorMessagesHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "GET":
		msgList := util.AllMessagesInOrder
		if len(msgList) > 0 {
			msgListJson, err := json.Marshal(msgList)
			util.CheckError(err)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(msgListJson)
		}
	case "POST" :
		err := r.ParseForm()
		util.CheckError(err)
		messageText := r.Form.Get("value")
		message := util.Message{
			Text:      messageText,
		}
		packetBytes, err := protobuf.Encode(&message)
		util.CheckError(err)
		mGossiper.ClientConn.WriteToUDP(packetBytes, mGossiper.ClientAddr)
	}
}

func GetIdHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		enableCors(&w)
		jsonValue, err := json.Marshal(mGossiper.Name)
		util.CheckError(err)
		w.WriteHeader(http.StatusOK)
		w.Write(jsonValue)
	}
}

func NodesHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "GET":
		mGossiper.LPeers.Mutex.Lock()
		peersMap := mGossiper.LPeers.Peers.PeersMap
		mGossiper.LPeers.Mutex.Unlock()
		if len(*peersMap) > 0 {
			peersList := make([]string, 0)
			for k := range *peersMap {
				peersList = append(peersList, k)
			}
			peerListJson, err := json.Marshal(peersList)
			util.CheckError(err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(peerListJson)
		}
	case "POST" :
		err := r.ParseForm()
		util.CheckError(err)
		value := r.Form.Get("value")
		mGossiper.LPeers.Mutex.Lock()
		mGossiper.LPeers.Peers.AddPeer(value)
		mGossiper.LPeers.Mutex.Unlock()
	}
}

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
}
