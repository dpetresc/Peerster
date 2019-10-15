package util

import "fmt"

type Message struct {
	Text string
}

func (clientMessage *Message) PrintClientMessage() {
	fmt.Println("CLIENT MESSAGE " + clientMessage.Text)
}