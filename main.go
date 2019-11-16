package main

import (
	"flag"
	"fmt"
	"github.com/dpetresc/Peerster/gossip"
	"github.com/dpetresc/Peerster/util"
	"net/http"
)

var uiPort string
var gossipAddr string
var name string
var peers string
var simple bool
var antiEntropy int
var gui bool
var rtimer int

var clientAddr string

var mGossiper *gossip.Gossiper

func init() {
	flag.StringVar(&uiPort, "UIPort", "8080", "port for the UI client")
	flag.StringVar(&gossipAddr, "gossipAddr", "127.0.0.1:5000", "ip:port for the gossip")
	flag.StringVar(&name, "name", "", "name of the gossip")
	flag.StringVar(&peers, "peers", "", "comma separated list of peers of the form ip:port")
	flag.BoolVar(&simple, "simple", false, "run gossip in simple broadcast mode")
	flag.IntVar(&antiEntropy, "antiEntropy", 10, "timeout in seconds for anti-entropy")
	flag.IntVar(&rtimer, "rtimer", 0, "Timeout in seconds to send route rumors")
	flag.BoolVar(&gui, "gui", false, "run gossip with gui")

	flag.Parse()
}

func main() {
	//rand.Seed(time.Now().UnixNano())

	clientAddr = "127.0.0.1:" + uiPort

	util.InitFileFolders()

	mGossiper = gossip.NewGossiper(clientAddr, gossipAddr, name, peers, simple, antiEntropy, rtimer)

	go func() {
		mGossiper.ListenClient()
	}()

	if !simple && antiEntropy != 0 {
		// Anti - Entropy
		// in simple mode you can't receive status packets
		// antiEntropy = 0 deactivates the entropy
		go func() {
			mGossiper.AntiEntropy()
		}()
	}

	fmt.Println(rtimer)
	if !simple && rtimer != 0 {
		// Send route rumor
		// 0 means disabling this feature
		go func() {
			mGossiper.RouteRumors()
		}()
	}

	if gui {
		go func() {
			http.Handle("/", http.FileServer(http.Dir("./frontend")))
			http.HandleFunc("/message", RumorMessagesHandler)
			http.HandleFunc("/id", GetIdHandler)
			http.HandleFunc("/node", NodesHandler)
			http.HandleFunc("/identifier", IdentifiersHandler)
			http.HandleFunc("/private", PrivateMessagesHandler)
			http.HandleFunc("/file", FileHandler)
			for {
				err := http.ListenAndServe("localhost:8080", nil)
				util.CheckError(err)
			}
		}()
	}

	mGossiper.ListenPeers()
}
