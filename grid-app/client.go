package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait = 10 * time.Second

	pongWait = 60 * time.Second

	pingPeriod = (pongWait * 9) / 10

	maxMessageSize = math.MaxInt64
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type StringJSON struct {
	Arguments []string `json:"arguments"`
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	actions chan []byte

	commands chan string

	grid *Grid
}

// readPump pumps messages from the websocket connection to the hub (?)
func setFile(path string, dataString string, c *Client) {

	errFile := ioutil.WriteFile(path, []byte(dataString), 0644)
	if errFile != nil {
		fmt.Println("Error calling setFile for path: " + path)
		fmt.Print(errFile)
		return
	}
}

func getFile(path string, c *Client) {

	b, err := ioutil.ReadFile(path) // just pass the file name
	if err != nil {
		fmt.Println("Error calling getFile for path: " + path)
		fmt.Print(err)
		return
	}

	sEnc := base64.StdEncoding.EncodeToString(b)

	jsonData := []string{"GET-FILE", path, sEnc}

	json, err := json.Marshal(jsonData)

	if err != nil {
		fmt.Println(err)
	}

	c.send <- json
}

func getDirectory(path string, c *Client) {

	path = strings.TrimRight(path, "/")

	if len(path) == 0 {
		path = "/"
	}

	jsonData := []string{"GET-DIRECTORY"}

	levelUp := ""

	if len(path) > 0 {
		pathComponents := strings.Split(path, "/")
		pathComponents = pathComponents[:len(pathComponents)-1]
		levelUp = strings.Join(pathComponents, "/")

		if levelUp == "" {
			levelUp = "/"
		}
	}

	jsonData = append(jsonData, "directory", "..", levelUp)

	files, err := ioutil.ReadDir(path)
	if err != nil {
		fmt.Println(err)

		// directory doesn't exist
		jsonData = []string{"GET-DIRECTORY", "INVALIDPATH"}

	}

	filePath := path
	if path == "/" {
		filePath = ""
	}

	for _, f := range files {
		fileType := "file"
		if f.IsDir() {
			fileType = "directory"
		}

		jsonData = append(jsonData, fileType, f.Name(), filePath+"/"+f.Name())
	}

	json, err := json.Marshal(jsonData)

	if err != nil {
		fmt.Println(err)
	}

	c.send <- json
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
		fmt.Println("Closed readPump")
		c.commands <- "CLOSE"
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := c.conn.ReadMessage()

		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				log.Printf("error: %v", err)
			}
			break
		}

		messageString := string(message)

		c.hub.inactiveTime = 0

		// if len(messageString) > 100 {
		// 	fmt.Println("Received WS message: " + messageString[:100] + "... [truncated]")
		// } else {
		// 	fmt.Println("Received WS message: " + messageString)
		// }

		// check if command or code
		if messageString[:7] == "#PARSE#" {
			c.commands <- messageString[7:]
		} else {
			c.actions <- message
		}

		// c.hub.broadcast <- message // send message to hub
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)

	go gridInstance(c)

	defer func() {
		ticker.Stop()
		c.conn.Close()

		fmt.Println("Closed writePump")
	}()

	for {
		select {
		case message, ok := <-c.send:

			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if !ok {
				// The hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)

			if err != nil {
				return
			}

			w.Write(message)

			// add queued chat messages to the current websocket message.
			n := len(c.send)

			for i := 0; i < n; i++ {
				w.Write(newline)
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			c.hub.inactiveTime = 0

			if err := c.conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				fmt.Println("errored on sending pingmessage to client")
				fmt.Println(err)
				return
			}
		}
	}
}

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {

	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256), actions: make(chan []byte, 256), commands: make(chan string, 256)}
	client.hub.register <- client
	fmt.Println("Client connected!")

	// Allow new connection of memory referenced by the caller by doing all the work in new goroutines.
	go client.writePump()
	go client.readPump()
	go client.pythonInterpreter()

}
