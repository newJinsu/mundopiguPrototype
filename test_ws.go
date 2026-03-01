//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

type ClientInput struct {
	Type   string `json:"type,omitempty"`
	RoomID string `json:"roomId,omitempty"`
}

func main() {
	u := url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/ws"}

	// Connect client 1
	c1, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial c1:", err)
	}
	defer c1.Close()

	// C1 creates room
	err = c1.WriteJSON(ClientInput{Type: "CREATE_ROOM"})
	if err != nil {
		log.Fatal("c1 write:", err)
	}

	var msg map[string]interface{}
	// Read until we get the init or room list for joining
	var roomID string
	for {
		err = c1.ReadJSON(&msg)
		if err != nil {
			log.Fatal("c1 read:", err)
		}
		if msg["type"] == "room_list" {
			rooms := msg["rooms"].([]interface{})
			if len(rooms) > 0 {
				room := rooms[0].(map[string]interface{})
				roomID = room["id"].(string)
				fmt.Println("Created room:", roomID)
				break
			}
		}
	}

	// Dump out any pending init messages for c1
	go func() {
		for {
			var m map[string]interface{}
			if err := c1.ReadJSON(&m); err != nil {
				return
			}
			if m["type"] == "state" {
				state := m["state"].(map[string]interface{})
				b, _ := json.MarshalIndent(state, "", "  ")
				fmt.Println("C1 State:", string(b))
			}
		}
	}()

	// Connect client 2
	c2, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial c2:", err)
	}
	defer c2.Close()

	err = c2.WriteJSON(ClientInput{Type: "JOIN_ROOM", RoomID: roomID})
	if err != nil {
		log.Fatal("c2 write:", err)
	}

	// Wait a bit to ensure C2 joined
	time.Sleep(500 * time.Millisecond)

	// C1 starts game
	err = c1.WriteJSON(ClientInput{Type: "START_GAME"})
	if err != nil {
		log.Fatal("c1 start game:", err)
	}

	time.Sleep(2 * time.Second)
}
