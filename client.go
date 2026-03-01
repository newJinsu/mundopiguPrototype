package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait = 10 * time.Second
	pongWait = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	maxMessageSize = 1024
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Client struct {
	lobby *Lobby
	room *Room
	conn *websocket.Conn
	send chan []byte
	role string
}

func (c *Client) readPump() {
	defer func() {
		if c.room != nil {
			c.room.unregister <- c
		} else if c.lobby != nil {
			c.lobby.unregister <- c
		}
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	
	for {
		var input ClientInput
		err := c.conn.ReadJSON(&input)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		
		if input.Type == "LEAVE_ROOM" && c.lobby != nil {
			c.lobby.inputs <- struct{
				client *Client
				input  ClientInput
			}{client: c, input: input}
			continue
		}

		if c.room != nil {
			c.room.inputs <- struct{
				client *Client
				input  ClientInput
			}{client: c, input: input}
		} else if c.lobby != nil {
			c.lobby.inputs <- struct{
				client *Client
				input  ClientInput
			}{client: c, input: input}
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func serveWs(lobby *Lobby, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{lobby: lobby, conn: conn, send: make(chan []byte, 256)}
	client.lobby.register <- client

	go client.writePump()
	go client.readPump()
}
