package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Message struct {
	Type     string          `json:"type"`
	Room     string          `json:"room,omitempty"`
	To       string          `json:"to,omitempty"`
	From     string          `json:"from,omitempty"`
	Offer    json.RawMessage `json:"offer,omitempty"`
	Answer   json.RawMessage `json:"answer,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
	MicOn    *bool           `json:"micOn,omitempty"`
	Users    []User          `json:"users,omitempty"`
	ID       string          `json:"id,omitempty"`
}

type User struct {
	ID    string `json:"id"`
	MicOn bool   `json:"micOn"`
}

type Client struct {
	ID     string
	Conn   *websocket.Conn
	Room   string
	MicOn  bool
}

var (
	clients   = make(map[string]*Client)
	roomUsers = make(map[string][]string)
	mutex     sync.Mutex
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	http.HandleFunc("/ws", handleWebSocket)
	log.Println("✅ Signaling server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("❌ Upgrade error:", err)
		return
	}

	id := uuid.New().String()
	client := &Client{ID: id, Conn: conn, MicOn: true}
	clients[id] = client

	sendMessage(conn, Message{Type: "id", ID: id})

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			log.Println("❌ Read error:", err)
			break
		}

		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			log.Println("❌ JSON error:", err)
			continue
		}

		switch msg.Type {
		case "join":
			client.Room = msg.Room
			mutex.Lock()
			roomUsers[msg.Room] = append(roomUsers[msg.Room], client.ID)
			mutex.Unlock()
			log.Printf("🔵 User %s joined room %s", client.ID, msg.Room)
			broadcastUserList(msg.Room)

		case "leave":
			log.Printf("🔴 User %s left room %s", client.ID, client.Room)
			removeClient(client)
			broadcastUserList(msg.Room)

		case "mic":
			if msg.MicOn != nil {
				client.MicOn = *msg.MicOn
				broadcastUserList(client.Room)
			}

		case "offer", "answer", "candidate":
			msg.From = client.ID
			sendTo(msg.To, msg)
		}
	}

	log.Printf("🔴 User %s disconnected from room %s", client.ID, client.Room)
	removeClient(client)
	broadcastUserList(client.Room)
}

func sendTo(id string, msg Message) {
	if target, ok := clients[id]; ok {
		sendMessage(target.Conn, msg)
	}
}

func sendMessage(conn *websocket.Conn, msg Message) {
	conn.WriteJSON(msg)
}

func broadcastUserList(room string) {
	mutex.Lock()
	defer mutex.Unlock()

	users := []User{}
	for _, id := range roomUsers[room] {
		if c, ok := clients[id]; ok {
			users = append(users, User{ID: c.ID, MicOn: c.MicOn})
		}
	}

	for _, id := range roomUsers[room] {
		if c, ok := clients[id]; ok {
			sendMessage(c.Conn, Message{Type: "user_list", Users: users})
		}
	}
}

func removeClient(c *Client) {
	mutex.Lock()
	defer mutex.Unlock()

	delete(clients, c.ID)

	if users, ok := roomUsers[c.Room]; ok {
		updated := []string{}
		for _, id := range users {
			if id != c.ID {
				updated = append(updated, id)
			}
		}
		roomUsers[c.Room] = updated
	}
}
