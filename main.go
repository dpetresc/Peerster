package main

import (
	"flag"
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
var rtimer int
var gui bool

// hw3
var hw3ex2 bool
var N uint64
var hopLimitTLC uint64
var stubbornTimeout int
var hw3ex3 bool
var ackAll bool

var clientAddr string

var mGossiper *gossip.Gossiper

func init() {
	flag.StringVar(&uiPort, "UIPort", "8080", "port for the UI client")
	flag.StringVar(&gossipAddr, "gossipAddr", "127.0.0.1:5000", "ip:port for the gossip")
	flag.StringVar(&name, "name", "", "name of the gossip")
	flag.StringVar(&peers, "peers", "", "comma separated list of peers of the form ip:port")
	flag.BoolVar(&simple, "simple", false, "run gossip in simple broadcast mode")
	flag.IntVar(&antiEntropy, "antiEntropy", 10, "timeout in seconds for anti-entropy")
	flag.IntVar(&rtimer, "rtimer", 0, "timeout in seconds to send route rumors")
	flag.BoolVar(&gui, "gui", false, "run gossip with gui")
	// hw3
	flag.BoolVar(&hw3ex2, "hw3ex2", false, "hw3ex2 flag")
	flag.Uint64Var(&N, "N", 1, "number of nodes in peerster")
	flag.Uint64Var(&hopLimitTLC, "hopLimit", 10, "hop limit value for TLCAck")
	flag.IntVar(&stubbornTimeout, "stubbornTimeout", 5, "timeout in seconds for TLC")
	flag.BoolVar(&hw3ex3, "hw3ex3", false, "hw3ex3 flag")
	flag.BoolVar(&ackAll, "ackAll", false, "hw3ex3 run as in hmw3ex2")

	flag.Parse()
}

func main() {
	//rand.Seed(time.Now().UnixNano())

	clientAddr = "127.0.0.1:" + uiPort

	util.InitFileFolders()

	mGossiper = gossip.NewGossiper(clientAddr, gossipAddr, name, peers, simple, antiEntropy, rtimer,
		N, uint32(hopLimitTLC), stubbornTimeout, hw3ex2, hw3ex3, ackAll)

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
			http.HandleFunc("/search", SearchHandler)
			// GUI
			http.HandleFunc("/matches", matchesHandler)
			http.HandleFunc("/confirmation", confirmationHandler)
			// END GUI
			for {
				err := http.ListenAndServe("localhost:8080", nil)
				util.CheckError(err)
			}
		}()
	}

	mGossiper.ListenPeers()
}
