package main

import (
	"flag"
	"github.com/dpetresc/Peerster/gossip"
	"github.com/dpetresc/Peerster/util"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

var uiPort string
var gossipAddr string
var name string
var peers string
var simple bool
var antiEntropy uint
var gui bool

var clientAddr string

var mGossiper *gossip.Gossiper

func init() {
	flag.StringVar(&uiPort, "UIPort", "8080", "port for the UI client")
	flag.StringVar(&gossipAddr, "gossipAddr", "127.0.0.1:5000", "ip:port for the gossip")
	flag.StringVar(&name, "name", "", "name of the gossip")
	flag.StringVar(&peers, "peers", "", "comma separated list of peers of the form ip:port")
	flag.BoolVar(&simple, "simple", false, "run gossip in simple broadcast mode")
	flag.UintVar(&antiEntropy, "antiEntropy", 10, "timeout in seconds for anti-entropy")
	flag.BoolVar(&gui, "gui", false, "run gossip with gui")

	flag.Parse()
}

func main() {
	var group sync.WaitGroup
	group.Add(3)

	rand.Seed(time.Now().UnixNano())

	clientAddr = "127.0.0.1:" + uiPort

	mGossiper = gossip.NewGossiper(clientAddr, gossipAddr, name, peers, simple, antiEntropy)

	go func() {
		defer group.Done()
		mGossiper.ListenClient()
	}()

	go func() {
		defer group.Done()
		mGossiper.ListenPeers()
	}()

	if !simple && antiEntropy != 0 {
		// Anti - Entropy
		// in simple mode you can't receive status packets
		// antiEntropy = 0 deactivates the entropy
		defer group.Done()
		go func() {
			mGossiper.AntiEntropy()
		}()
	}

	if gui {
		go func() {
			http.Handle("/", http.FileServer(http.Dir("./frontend")))
			http.HandleFunc("/message", RumorMessagesHandler)
			http.HandleFunc("/id", GetIdHandler)
			http.HandleFunc("/node", NodesHandler)
			for {
				err := http.ListenAndServe("localhost:8080", nil)
				util.CheckError(err)
			}
		}()
	}
	group.Wait()

}
