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
	case "POST" :
		err := r.ParseForm()
		util.CheckError(err)
		messageText := r.Form.Get("value")
		dest := r.Form.Get("identifier")
		var message util.Message
		if dest == "public" {
			// public message
			message = util.Message{
				Text:      messageText,
				Destination: nil,
			}
		} else {
			// private message
			message = util.Message{
				Text:      messageText,
				Destination: &dest,
			}
		}
		sendMessagetoClient(&message)
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
	case "POST" :
		err := r.ParseForm()
		util.CheckError(err)
		value := r.Form.Get("value")
		mGossiper.Peers.Lock()
		// can't add my address to the peers
		if value != util.UDPAddrToString(mGossiper.Address){
			mGossiper.Peers.AddPeer(value)
		} else {
			http.Error(w, "Can't add own address as Peer !", http.StatusUnauthorized)
		}
		mGossiper.Peers.Unlock()
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
	case "POST" :
		err := r.ParseForm()
		util.CheckError(err)
		fileName := r.Form.Get("value")
		dest := r.Form.Get("identifier")
		var message util.Message
		if dest == "public" {
			message = util.Message{
				Destination: nil,
				File: &fileName,
			}
			sendMessagetoClient(&message)
		} else {
			request := r.Form.Get("request")
			requestBytes, err := hex.DecodeString(request)
			goodFormat := true
			if err != nil {
				goodFormat = false
			} else if len(requestBytes) != 32{
				goodFormat = false
			}
			if !goodFormat {
				http.Error(w, "Invalid metahash !", http.StatusUnauthorized)
			} else {
				message = util.Message{
					Destination: &dest,
					File: &fileName,
					Request: &requestBytes,
				}
				sendMessagetoClient(&message)
			}
		}
	}
}

func SearchHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "POST" :
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
			budget, err := strconv.ParseUint(budgetStr,10, 64)
			if err != nil {
				http.Error(w, "Please enter an uint64 budget !", http.StatusUnauthorized)
				return
			}
			message.Budget = &budget
		}
		sendMessagetoClient(&message)
	}
}

// GUI
func matchesHandler(w http.ResponseWriter, request *http.Request) {
	enableCors(&w)
	switch request.Method{
	case "GET":
		mGossiper.Matches.RLock()
		matchesAsJSON, err := json.Marshal(mGossiper.Matches.Queue)
		mGossiper.Matches.RUnlock()
		if err == nil {
			w.WriteHeader(http.StatusOK)
			w.Write(matchesAsJSON)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	default:
		w.WriteHeader(http.StatusNotFound)

	}
}

func confirmationHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	switch r.Method {
	case "GET":
		mGossiper.LCurrentPublish.Lock()
		confirmation := mGossiper.LCurrentPublish.Confirmed
		if len(confirmation) > 0 {
			msgListJson, err := json.Marshal(confirmation)
			util.CheckError(err)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(msgListJson)
			mGossiper.LCurrentPublish.Confirmed = make([]string, 0)
		}
		mGossiper.LCurrentPublish.Unlock()
	}
}
// END GUI

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
}
