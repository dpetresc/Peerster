package main

import (
	"crypto/rsa"
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
var tor bool
var secure bool

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

	// crypto
	// when tor flag is not present
	flag.BoolVar(&secure, "secure", false, "secure private message flag")
	// tor
	flag.BoolVar(&tor, "tor", false, "tor flag")

	flag.Parse()
}

func main() {
	clientAddr = "127.0.0.1:" + uiPort

	util.InitFileFolders()

	var privateKey *rsa.PrivateKey
	var CAKey *rsa.PublicKey
	if tor || secure {
		privateKey = util.GetPrivateKey(util.KeysFolderPath, name)
		CAKey = util.GetCAKey(util.KeysFolderPath)
	}

	mGossiper = gossip.NewGossiper(clientAddr, gossipAddr, name, peers, simple, antiEntropy,
		rtimer, tor, secure, privateKey, CAKey)

	if tor || secure {
		go func() {
			mGossiper.Consensus()
		}()

		go func() {
			mGossiper.HSConsensus()
		}()
	}

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
			http.HandleFunc("/privateTor", PrivateTorMessagesHandler)
			http.HandleFunc("/file", FileHandler)
			http.HandleFunc("/search", SearchHandler)
			http.HandleFunc("/onionAddr", OnionAddrHandler)
			http.HandleFunc("/htmlGetter", HTMLGetterHandler)
			for {
				err := http.ListenAndServe("localhost:8080", nil)
				util.CheckError(err)
			}
		}()
	}

	mGossiper.ListenPeers()
}