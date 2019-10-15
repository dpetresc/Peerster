package main

import (
	"flag"
	"github.com/dpetresc/Peerster/gossip"
	"sync"
	"time"
)

var uiPort string
var gossipAddr string
var name string
var peers string
var simple bool
var antiEntropy uint

var clientAddr string

var mGossiper *gossip.Gossiper

func main() {
	flag.StringVar(&uiPort, "UIPort", "8080", "port for the UI client")
	flag.StringVar(&gossipAddr, "gossipAddr", "127.0.0.1:5000", "ip:port for the gossip")
	flag.StringVar(&name, "name", "", "name of the gossip")
	flag.StringVar(&peers, "peers", "", "comma separated list of peers of the form ip:port")
	flag.BoolVar(&simple, "simple", false, "run gossip in simple broadcast mode")
	flag.UintVar(&antiEntropy, "antiEntropy", 10, "timeout in seconds for anti-entropy")

	flag.Parse()

	var group sync.WaitGroup
	group.Add(2)

	clientAddr = "127.0.0.1:" + uiPort

	mGossiper = gossip.NewGossiper(clientAddr, gossipAddr, name, peers, simple)

	go func() {
		defer group.Done()
		mGossiper.ListenClient()
	}()

	go func() {
		defer group.Done()
		mGossiper.ListenPeers()
	}()

	// in simple mode you can't receive status packets
	// antiEntropy = 0 deactivates the entropy
	if !simple && antiEntropy != 0 {
		group.Add(1)
		go func() {
			defer group.Done()
			ticker := time.NewTicker(time.Duration(antiEntropy) * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					mGossiper.SendStatusPacket("")
				}
			}
		}()
	}
	group.Wait()

	//fmt.Println("Yes")

}
