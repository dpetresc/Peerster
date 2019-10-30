package main

import (
	"encoding/json"
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/routing"
	"github.com/dpetresc/Peerster/util"
	"net/http"
)

func RumorMessagesHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "GET":
		msgList := util.LastMessagesInOrder
		if len(msgList) > 0 {
			msgListJson, err := json.Marshal(msgList)
			util.CheckError(err)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(msgListJson)
			util.LastMessagesInOrder = make([]*util.RumorMessage, 0)
		}
	case "POST" :
		err := r.ParseForm()
		util.CheckError(err)
		messageText := r.Form.Get("value")
		dest := r.Form.Get("identifier")
		if dest == "public" {
			dest = ""
		}
		message := util.Message{
			Text:      messageText,
			Destination: &dest,
		}
		packetBytes, err := protobuf.Encode(&message)
		util.CheckError(err)
		mGossiper.ClientConn.WriteToUDP(packetBytes, mGossiper.ClientAddr)
	}
}

func IdentifiersHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "GET":
		originList := mGossiper.LDsdv.Origins
		if len(originList) > 0 {
			msgListJson, err := json.Marshal(originList)
			util.CheckError(err)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(msgListJson)
		}
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
		mGossiper.Peers.Mutex.Lock()
		peersMap := mGossiper.Peers.PeersMap
		if len(peersMap) > 0 {
			peersList := make([]string, 0)
			for k := range peersMap {
				peersList = append(peersList, k)
			}
			peerListJson, err := json.Marshal(peersList)
			util.CheckError(err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(peerListJson)
		}
		mGossiper.Peers.Mutex.Unlock()
	case "POST" :
		err := r.ParseForm()
		util.CheckError(err)
		value := r.Form.Get("value")
		mGossiper.Peers.Mutex.Lock()
		// can't add my address to the peers
		if value != util.UDPAddrToString(mGossiper.Address){
			mGossiper.Peers.AddPeer(value)
		} else {
			http.Error(w, "Can't add own address as Peer !", http.StatusUnauthorized)
		}
		mGossiper.Peers.Mutex.Unlock()
	}
}

func PrivateMessagesHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "GET":
		privateMsgs := routing.LastPrivateMessages
		if len(privateMsgs) > 0 {
			msgListJson, err := json.Marshal(privateMsgs)
			util.CheckError(err)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(msgListJson)
			routing.LastPrivateMessages = make(map[string][]*util.PrivateMessage)
		}
	}
}

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
}
