package server

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
	"webrtc-tunnel/signal"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
}

// peerManager holds all registered host peers.
type peerManager struct {
	peers map[string]*websocket.Conn
	mu    sync.RWMutex
}

func newPeerManager() *peerManager {
	return &peerManager{
		peers: make(map[string]*websocket.Conn),
	}
}

func (pm *peerManager) addHost(id string, conn *websocket.Conn) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if _, ok := pm.peers[id]; ok {
		return fmt.Errorf("host with ID '%s' already registered", id)
	}
	pm.peers[id] = conn
	glog.Infof("Host registered: %s", id)
	return nil
}

func (pm *peerManager) removeHost(id string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if conn, ok := pm.peers[id]; ok {
		conn.Close()
		delete(pm.peers, id)
		glog.Infof("Host removed: %s", id)
	}
}

func (pm *peerManager) getHost(id string) *websocket.Conn {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.peers[id]
}

// handleConnection manages the entire lifecycle of a websocket connection.
func (pm *peerManager) handleConnection(conn *websocket.Conn) {
	// The first message determines the connection's role.
	var msg signal.Message
	err := conn.ReadJSON(&msg)
	if err != nil {
		glog.Errorf("Error reading initial message: %v", err)
		conn.Close()
		return
	}

	if msg.Type == signal.MessageTypeRegisterHost {
		hostID := msg.SenderID
		if hostID == "" {
			glog.Error("Host registration failed: empty ID")
			conn.Close()
			return
		}
		if err := pm.addHost(hostID, conn); err != nil {
			glog.Errorf("Host registration failed: %v", err)
			conn.WriteJSON(signal.Message{Type: signal.MessageTypeError, Payload: err.Error()})
			conn.Close()
			return
		}
		defer pm.removeHost(hostID)
		pm.readLoop(conn, hostID) // Pass hostID for logging
		return
	}

	if err := pm.routeMessage(conn, msg); err != nil {
		glog.Errorf("Failed to route initial client message: %v", err)
		conn.WriteJSON(signal.Message{Type: signal.MessageTypeError, Payload: err.Error()})
	}
	pm.readLoop(conn, "client") // Identify connection as client for logging
}

// readLoop reads messages from a websocket and routes them.
func (pm *peerManager) readLoop(conn *websocket.Conn, connType string) {
	for {
		var msg signal.Message
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				glog.Errorf("Error reading message from %s: %v", connType, err)
			} else {
				glog.Infof("Connection from %s closed gracefully.", connType)
			}
			break // Exit loop on error or close
		}

		if err := pm.routeMessage(conn, msg); err != nil {
			glog.Errorf("Routing error: %v", err)
			conn.WriteJSON(signal.Message{Type: signal.MessageTypeError, Payload: err.Error()})
		}
	}
}

// routeMessage forwards a message to a target host.
func (pm *peerManager) routeMessage(senderConn *websocket.Conn, msg signal.Message) error {
	targetID := msg.TargetID
	if targetID == "" {
		return fmt.Errorf("message has no target ID")
	}

	targetConn := pm.getHost(targetID)
	if targetConn == nil {
		return fmt.Errorf("target host '%s' not found", targetID)
	}

	glog.Infof("Routing message from '%s' to '%s' (type: %s)", msg.SenderID, msg.TargetID, msg.Type)

	if err := targetConn.WriteJSON(msg); err != nil {
		return fmt.Errorf("failed to send message to target '%s': %v", targetID, err)
	}
	return nil
}


// Run starts the signaling server.
func Run(addr string) error {
	peerManager := newPeerManager()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			glog.Errorf("Failed to upgrade connection: %v", err)
			return
		}
		go peerManager.handleConnection(conn)
	})

	glog.Infof("Signaling server starting on %s", addr)
	return http.ListenAndServe(addr, nil)
}
