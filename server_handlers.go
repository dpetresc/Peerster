package main

import (
	"encoding/hex"
	"encoding/json"
	"github.com/dedis/protobuf"
	"github.com/dpetresc/Peerster/util"
	"net/http"
	"strconv"
)

func sendMessagetoClient(message *util.Message) {
	packetBytes, err := protobuf.Encode(message)
	util.CheckError(err)
	mGossiper.ClientConn.WriteToUDP(packetBytes, mGossiper.ClientAddr)
}

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
			// TODO lock
			util.LastMessagesInOrder = make([]*util.RumorMessage, 0)
		}
	case "POST":
		err := r.ParseForm()
		util.CheckError(err)
		messageText := r.Form.Get("value")
		dest := r.Form.Get("identifier")
		anonyme := r.Form.Get("messagetype")
		cID := r.Form.Get("cid")
		var message *util.Message
		if dest == "public" {
			// public message
			message = &util.Message{
				Text:        messageText,
				Destination: nil,
			}
		} else {
			// private message
			message = &util.Message{
				Text:        messageText,
				Destination: &dest,
			}
			if mGossiper.Tor {
				if anonyme == "anonyme" {
					message.Anonyme = true
				} else {
					message.Anonyme = false
				}
				if cID != "nil" {
					cIDUint, err := strconv.ParseUint(cID, 10, 64)
					util.CheckError(err)
					cUid := uint32(cIDUint)
					message.CID = &cUid
				}
			}

		}
		sendMessagetoClient(message)
	}
}

func OnionAddrHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "POST":
		err := r.ParseForm()
		util.CheckError(err)
		onionAddr := r.Form.Get("value")
		var message *util.Message
		// private message
		message = &util.Message{
			OnionAddr: &onionAddr,
		}
		sendMessagetoClient(message)
	}
}

func HTMLGetterHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "GET":
		mGossiper.LHS.Lock()
		html := mGossiper.LHS.HTMLForGui
		if len(html) > 0 {
			htmlJson, err := json.Marshal(html)
			util.CheckError(err)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(htmlJson)
			mGossiper.LHS.HTMLForGui = make(map[string]string)
		}
		mGossiper.LHS.Unlock()
	}
}


func IdentifiersHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "GET":
		if mGossiper.Tor {
			mGossiper.LConsensus.Lock()
			nodes := make([]string, 0, len(mGossiper.LConsensus.NodesPublicKeys))
			for k := range mGossiper.LConsensus.NodesPublicKeys {
				if k != mGossiper.Name {
					nodes = append(nodes, k)
				}
			}
			nodesJson, err := json.Marshal(nodes)
			util.CheckError(err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(nodesJson)
			mGossiper.LConsensus.Unlock()
		} else {
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
		mGossiper.Peers.Lock()
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
		mGossiper.Peers.Unlock()

	case "POST":
		err := r.ParseForm()
		util.CheckError(err)
		value := r.Form.Get("value")
		mGossiper.Peers.Lock()
		// can't add my address to the peers
		if value != util.UDPAddrToString(mGossiper.Address) {
			mGossiper.Peers.AddPeer(value)
		} else {
			http.Error(w, "Can't add own address as Peer !", http.StatusUnauthorized)
		}
		mGossiper.Peers.Unlock()
	}
}

func PrivateTorMessagesHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "GET":
		mGossiper.LLastPrivateMsg.Lock()
		privateMsgs := mGossiper.LLastPrivateMsg.LastPrivateMsgTor
		if len(privateMsgs) > 0 {
			msgListJson, err := json.Marshal(privateMsgs)
			util.CheckError(err)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(msgListJson)
		}
		for k, _ := range mGossiper.LLastPrivateMsg.LastPrivateMsgTor {
			mGossiper.LLastPrivateMsg.LastPrivateMsgTor[k] = make([]*util.PrivateMessage, 0)
		}
		//mGossiper.LLastPrivateMsg.LastPrivateMsgTor = make(map[uint32][]*util.PrivateMessage)
		mGossiper.LLastPrivateMsg.Unlock()
	}
}

func PrivateMessagesHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "GET":
		mGossiper.LLastPrivateMsg.Lock()
		privateMsgs := mGossiper.LLastPrivateMsg.LastPrivateMsg
		if len(privateMsgs) > 0 {
			msgListJson, err := json.Marshal(privateMsgs)
			util.CheckError(err)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(msgListJson)
			mGossiper.LLastPrivateMsg.LastPrivateMsg = make(map[string][]*util.PrivateMessage)
		}
		mGossiper.LLastPrivateMsg.Unlock()
	}
}

func FileHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "POST":
		err := r.ParseForm()
		util.CheckError(err)
		fileName := r.Form.Get("value")
		dest := r.Form.Get("identifier")
		var message util.Message
		if dest == "public" {
			message = util.Message{
				Destination: nil,
				File:        &fileName,
			}
			sendMessagetoClient(&message)
		} else {
			request := r.Form.Get("request")
			requestBytes, err := hex.DecodeString(request)
			goodFormat := true
			if err != nil {
				goodFormat = false
			} else if len(requestBytes) != 32 {
				goodFormat = false
			}
			if !goodFormat {
				http.Error(w, "Invalid metahash !", http.StatusUnauthorized)
			} else {
				message = util.Message{
					Destination: &dest,
					File:        &fileName,
					Request:     &requestBytes,
				}
				sendMessagetoClient(&message)
			}
		}
	}
}

func SearchHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "POST":
		err := r.ParseForm()
		util.CheckError(err)
		keywordsStr := r.Form.Get("value")
		budgetStr := r.Form.Get("budget")
		keywords := util.GetNonEmptyElementsFromString(keywordsStr, ",")
		if len(keywords) == 0 {
			http.Error(w, "Please enter at least one non-empty keyword !", http.StatusUnauthorized)
			return
		}
		message := util.Message{
			Keywords: &keywordsStr,
		}
		if budgetStr == "" {
			message.Budget = nil
		} else {
			budget, err := strconv.ParseUint(budgetStr, 10, 64)
			if err != nil {
				http.Error(w, "Please enter an uint64 budget !", http.StatusUnauthorized)
				return
			}
			message.Budget = &budget
		}
		sendMessagetoClient(&message)
	}
}

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
}
