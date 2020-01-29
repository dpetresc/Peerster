package gossip

import (
	"bytes"
	"encoding/json"
	"github.com/dpetresc/Peerster/util"
)

/*
 *	ID	id of the Tor circuit
 *	PreviousHOP previous node in Tor
 *	NextHOP 	next node in Tor, nil if you are the destination
 *	SharedKey 	shared SharedKey exchanged with the source
 */
type Circuit struct {
	ID          uint32
	PreviousHOP string
	NextHOP     string
	SharedKey   []byte

	// add timer
}

func (gossiper *Gossiper) HandleTorExtendRequest(torMessage *util.TorMessage, source string) {
	if c, ok := gossiper.lCircuits.circuits[torMessage.CircuitID]; ok {
		// decrpyt payload
		extendPayloadBytes := util.DecryptGCM(torMessage.Payload, torMessage.Nonce, c.SharedKey)
		var extendPayload *util.TorMessage
		err := json.NewDecoder(bytes.NewReader(extendPayloadBytes)).Decode(extendPayload)
		util.CheckError(err)


		// Guard Node

		//  Middle Node
	}
}
