package realtime

import (
	"encoding/json"
	"log"
	"sync"
)

type EventMessage struct {
	Event   string      `json:"event"`
	Room    string      `json:"room,omitempty"`
	Payload interface{} `json:"payload"`
}

type Hub struct {
	// Registered clients mapped by pointer
	clients map[*Client]bool

	// Rooms for targeted broadcasts: roomName -> map[client]bool
	rooms map[string]map[*Client]bool

	// Inbound messages from clients
	broadcast chan EventMessage

	// Register requests
	register chan *Client

	// Unregister requests
	unregister chan *Client

	mutex sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan EventMessage, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		rooms:      make(map[string]map[*Client]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			h.clients[client] = true
			for room := range client.rooms {
				if _, ok := h.rooms[room]; !ok {
					h.rooms[room] = make(map[*Client]bool)
				}
				h.rooms[room][client] = true
			}
			h.mutex.Unlock()
			log.Printf("[WebSocket Hub] Client connected. Total: %d", len(h.clients))

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				for room := range client.rooms {
					if clientsInRoom, ok := h.rooms[room]; ok {
						delete(clientsInRoom, client)
						if len(clientsInRoom) == 0 {
							delete(h.rooms, room)
						}
					}
				}
			}
			h.mutex.Unlock()
			log.Printf("[WebSocket Hub] Client disconnected. Total: %d", len(h.clients))

		case message := <-h.broadcast:
			h.mutex.RLock()
			msgBytes, err := json.Marshal(message)
			if err != nil {
				h.mutex.RUnlock()
				continue
			}

			if message.Room != "" {
				// Targeted room broadcast
				if clientsInRoom, ok := h.rooms[message.Room]; ok {
					for client := range clientsInRoom {
						select {
						case client.send <- msgBytes:
						default:
							close(client.send)
							delete(h.clients, client)
						}
					}
				}
			} else {
				// Global broadcast
				for client := range h.clients {
					select {
					case client.send <- msgBytes:
					default:
						close(client.send)
						delete(h.clients, client)
					}
				}
			}
			h.mutex.RUnlock()
		}
	}
}

func (h *Hub) BroadcastToRoom(room string, event string, payload interface{}) {
	h.broadcast <- EventMessage{
		Event:   event,
		Room:    room,
		Payload: payload,
	}
}
