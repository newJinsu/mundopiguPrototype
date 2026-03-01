package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
)

// Lobby manages active rooms and clients waiting to join games.
type Lobby struct {
	// Registered clients not yet in a room.
	clients map[*Client]bool

	// Active game rooms.
	rooms map[string]*Room

	// Inbound messages from clients in the lobby.
	inputs chan struct {
		client *Client
		input  ClientInput
	}

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	// Channel to signal a room is closing
	closeRoom chan string
}

func newLobby() *Lobby {
	// Initialize random seed for room IDs
	rand.Seed(time.Now().UnixNano())

	return &Lobby{
		inputs: make(chan struct {
			client *Client
			input  ClientInput
		}),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		closeRoom:  make(chan string),
		clients:    make(map[*Client]bool),
		rooms:      make(map[string]*Room),
	}
}

func (l *Lobby) Run() {
	// Broadcast room list every 2 seconds
	ticker := time.NewTicker(time.Second * 2)
	defer ticker.Stop()

	for {
		select {
		case client := <-l.register:
			l.clients[client] = true
			l.sendRoomListTo(client)

		case client := <-l.unregister:
			if _, ok := l.clients[client]; ok {
				delete(l.clients, client)
				close(client.send)
			}

		case roomID := <-l.closeRoom:
			if room, ok := l.rooms[roomID]; ok {
				// Room has ended, move all its connected clients back to the lobby
				for client := range room.clients {
					client.room = nil
					l.clients[client] = true
				}
				delete(l.rooms, roomID)
				l.broadcastRoomList()
			}

		case msg := <-l.inputs:
			client := msg.client
			input := msg.input

			if input.Type == "CREATE_ROOM" {
				roomID := fmt.Sprintf("room-%d", rand.Intn(10000))
				newRoom := newRoom(roomID, l)
				l.rooms[roomID] = newRoom
				go newRoom.Run()
				
				// Automatically join the room they created
				l.joinRoom(client, roomID)
				l.broadcastRoomList()
			} else if input.Type == "JOIN_ROOM" {
				l.joinRoom(client, input.RoomID)
			} else if input.Type == "LEAVE_ROOM" {
				if client.room != nil {
					client.room.unregister <- client
					client.room = nil
					l.clients[client] = true
					l.sendRoomListTo(client)
					l.broadcastRoomList()
				}
			}

		case <-ticker.C:
			l.broadcastRoomList()
		}
	}
}

func (l *Lobby) joinRoom(client *Client, roomID string) {
	if room, ok := l.rooms[roomID]; ok {
		// Remove from lobby
		delete(l.clients, client)
		
		// Join room
		client.room = room
		room.register <- client
		
		l.broadcastRoomList()
	} else {
		// send error back to client
		errorMsg := map[string]string{"type": "error", "message": "Room not found."}
		msgBytes, _ := json.Marshal(errorMsg)
		client.send <- msgBytes
	}
}

type RoomInfo struct {
	ID          string `json:"id"`
	PlayerCount int    `json:"playerCount"`
	MaxPlayers  int    `json:"maxPlayers"`
	Status      string `json:"status"`
}

func (l *Lobby) getRoomListMessage() []byte {
	var roomList []RoomInfo
	for id, room := range l.rooms {
		status := "Waiting"
		if len(room.clients) >= room.config.MaxPlayersPerTeam*2 {
			status = "Playing"
		}
		roomList = append(roomList, RoomInfo{
			ID:          id,
			PlayerCount: len(room.clients),
			MaxPlayers:  room.config.MaxPlayersPerTeam * 2,
			Status:      status,
		})
	}
	
	msg := map[string]interface{}{
		"type":  "room_list",
		"rooms": roomList,
	}
	msgBytes, _ := json.Marshal(msg)
	return msgBytes
}

func (l *Lobby) broadcastRoomList() {
	if len(l.clients) == 0 {
		return
	}
	msgBytes := l.getRoomListMessage()
	for client := range l.clients {
		select {
		case client.send <- msgBytes:
		default:
			close(client.send)
			delete(l.clients, client)
		}
	}
}

func (l *Lobby) sendRoomListTo(client *Client) {
	msgBytes := l.getRoomListMessage()
	client.send <- msgBytes
}
