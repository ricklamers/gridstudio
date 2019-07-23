package main

import (
	"fmt"
	"time"
)

type Hub struct {
	clients map[*Client]bool

	broadcast chan []byte

	register chan *Client

	unregister chan *Client

	mainThreadChannel chan string

	inactiveTime time.Duration

	rootDirectory string
}

func newHub(mainThreadChannel chan string, rootDirectory string) *Hub {

	return &Hub{
		broadcast:         make(chan []byte),
		register:          make(chan *Client),
		unregister:        make(chan *Client),
		clients:           make(map[*Client]bool),
		mainThreadChannel: mainThreadChannel,
		inactiveTime:      0,
		rootDirectory:     rootDirectory,
	}

}

func (h *Hub) run() {

	// timer for inactiveTime
	ticker := time.NewTicker(time.Second)

	for {
		select {
		case client := <-h.register:
			fmt.Println("WS Client registered")
			h.clients[client] = true
		case client := <-h.unregister:
			fmt.Println("WS Client unregistered")
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:

			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		case <-ticker.C:

			// update inactiveTime
			h.inactiveTime += time.Second
		}

	}
}
