package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"wtt/pkg/signal"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Client represents a connected peer.
type Client struct {
	peerID   string
	conn     *websocket.Conn
	send     chan []byte
	services []string
}

// Hub maintains the set of active clients and service registrations.
type Hub struct {
	clients         map[string]*Client
	serviceRegistry map[string]string // a map from service_name to peer_id
	register        chan *Client
	unregister      chan *Client
	mu              sync.Mutex
}

func newHub() *Hub {
	return &Hub{
		clients:         make(map[string]*Client),
		serviceRegistry: make(map[string]string),
		register:        make(chan *Client),
		unregister:      make(chan *Client),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.peerID] = client
			h.mu.Unlock()
			log.Printf("Client registered: %s", client.peerID)
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.peerID]; ok {
				for _, service := range client.services {
					if h.serviceRegistry[service] == client.peerID {
						delete(h.serviceRegistry, service)
						log.Printf("Service unregistered: %s", service)
					}
				}
				delete(h.clients, client.peerID)
				close(client.send)
				log.Printf("Client unregistered: %s", client.peerID)
			}
			h.mu.Unlock()
		}
	}
}

func authHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Received request on /api/v1/auth")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jwt":        "dummy-session-jwt-for-testing",
		"expires_in": 3600,
	})
}

func (c *Client) readPump(h *Hub) {
	defer func() {
		h.unregister <- c
		c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}

		var msg signal.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("error unmarshalling message: %v", err)
			continue
		}

		log.Printf("Received message type '%s' from peer %s", msg.Type, c.peerID)
		h.handleMessage(c, msg)
	}
}

func (h *Hub) handleMessage(from *Client, msg signal.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()

	switch msg.Type {
	case "publish_services":
		payloadBytes, _ := json.Marshal(msg.Payload)
		var payload signal.PublishServicesPayload
		json.Unmarshal(payloadBytes, &payload)

		from.services = payload.Services
		for _, service := range from.services {
			h.serviceRegistry[service] = from.peerID
			log.Printf("Peer %s registered service: %s", from.peerID, service)
		}

	case "request_connection":
		payloadBytes, _ := json.Marshal(msg.Payload)
		var payload signal.RequestConnectionPayload
		json.Unmarshal(payloadBytes, &payload)

		providerPeerID, ok := h.serviceRegistry[payload.ServiceName]
		if !ok {
			log.Printf("Service %s not found", payload.ServiceName)
			// TODO: Send an error message back to the consumer
			return
		}

		provider, ok := h.clients[providerPeerID]
		if !ok {
			log.Printf("Provider peer %s not found", providerPeerID)
			return
		}

		// Forward request to provider
		reqMsg := signal.Message{
			Type: "connection_request",
			Payload: signal.ConnectionRequestPayload{
				FromPeerID:  from.peerID,
				ServiceName: payload.ServiceName,
			},
		}
		reqBytes, _ := json.Marshal(reqMsg)
		provider.send <- reqBytes
		log.Printf("Forwarded connection request from %s to %s for service %s", from.peerID, provider.peerID, payload.ServiceName)

	case "signal":
		payloadBytes, _ := json.Marshal(msg.Payload)
		var payload signal.SignalPayload
		json.Unmarshal(payloadBytes, &payload)

		targetClient, ok := h.clients[payload.ToPeerID]
		if !ok {
			log.Printf("Signal target peer %s not found", payload.ToPeerID)
			return
		}

		// Forward signal to target peer
		signalMsg := signal.Message{
			Type: "signal",
			Payload: signal.SignalData{
				FromPeerID: from.peerID,
				Data:       payload.Data,
			},
		}
		signalBytes, _ := json.Marshal(signalMsg)
		targetClient.send <- signalBytes
		log.Printf("Relayed signal from %s to %s", from.peerID, payload.ToPeerID)
	}
}

func (c *Client) writePump() {
	defer func() {
		c.conn.Close()
	}()
	for message := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			log.Printf("error writing message: %v", err)
			return
		}
	}
}

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	_, regMsgBytes, err := conn.ReadMessage()
	if err != nil {
		log.Printf("error reading registration message: %v", err)
		conn.Close()
		return
	}

	var regMsg signal.Message
	if err := json.Unmarshal(regMsgBytes, &regMsg); err != nil || regMsg.Type != "register" {
		log.Printf("error: expected 'register' message, got: %s", regMsgBytes)
		conn.Close()
		return
	}

	peerID := "peer_" + time.Now().Format("20060102150405")
	client := &Client{peerID: peerID, conn: conn, send: make(chan []byte, 256)}
	hub.register <- client

	registeredPayload := signal.RegisteredPayload{
		PeerID: peerID,
		// TODO: Pass actual ICE servers from config
		IceServers: []map[string]string{{"urls": "stun:stun.l.google.com:19302"}},
	}
	respBytes, _ := json.Marshal(signal.Message{Type: "registered", Payload: registeredPayload})
	client.send <- respBytes

	go client.writePump()
	go client.readPump(hub)
}

func main() {
	hub := newHub()
	go hub.run()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth", authHandler)
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})

	log.Println("Starting signaling server on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
