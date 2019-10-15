package util

import "fmt"

type Message struct {
	Text string
}

func (clientMessage *Message) PrintClientMessage() {
	fmt.Printf("CLIENT MESSAGE %s\n", clientMessage.Text)
}