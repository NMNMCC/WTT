package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
	"webrtc-tunnel/signal"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 10240 // Increased to allow for SDP
)

// safeConnection is a thread-safe wrapper around a websocket.Conn.
type safeConnection struct {
	conn *websocket.Conn
	// Buffered channel of outbound messages.
	send chan []byte
}

// writePump pumps messages from the send channel to the websocket connection.
func (sc *safeConnection) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		sc.conn.Close()
	}()
	for {
		select {
		case message, ok := <-sc.send:
			sc.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The peerManager closed the channel.
				sc.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := sc.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				glog.Errorf("Error writing message: %v", err)
				return
			}

		case <-ticker.C:
			sc.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := sc.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				glog.Warningf("Error writing ping: %v", err)
				return // Connection might be dead.
			}
		}
	}
}

// peerManager holds all registered host peers.
type peerManager struct {
	peers map[string]*safeConnection
	mu    sync.RWMutex
}

func newPeerManager() *peerManager {
	return &peerManager{
		peers: make(map[string]*safeConnection),
	}
}

func (pm *peerManager) addHost(id string, conn *websocket.Conn) (*safeConnection, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if _, ok := pm.peers[id]; ok {
		return nil, fmt.Errorf("host with ID '%s' already registered", id)
	}

	safeConn := &safeConnection{conn: conn, send: make(chan []byte, 256)}
	pm.peers[id] = safeConn
	go safeConn.writePump()

	glog.Infof("Host registered: %s", id)
	return safeConn, nil
}

func (pm *peerManager) removeHost(id string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if safeConn, ok := pm.peers[id]; ok {
		close(safeConn.send)
		delete(pm.peers, id)
		glog.Infof("Host removed: %s", id)
	}
}

func (pm *peerManager) getHost(id string) *safeConnection {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.peers[id]
}

// handleConnection manages the entire lifecycle of a websocket connection.
func (pm *peerManager) handleConnection(conn *websocket.Conn) {
	// The first message determines the connection's role.
	var msg signal.Message
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	err := conn.ReadJSON(&msg)
	conn.SetReadDeadline(time.Time{}) // Clear deadline after initial read
	if err != nil {
		if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
			glog.Errorf("Error reading initial message: %v", err)
		}
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
		if _, err := pm.addHost(hostID, conn); err != nil {
			glog.Errorf("Host registration failed: %v", err)
			// Cannot easily write JSON back, as write pump is not started.
			conn.Close()
			return
		}
		// The readPump for the host will handle reading messages and cleanup.
		pm.readPump(conn, hostID, true)
		return
	}

	// For clients, route the first message, then enter a read loop for subsequent messages (candidates).
	if err := pm.routeMessage(conn, msg); err != nil {
		glog.Errorf("Failed to route initial client message: %v", err)
	}
	pm.readPump(conn, msg.SenderID, false) // Identify connection as client for logging
}

// readPump pumps messages from the websocket connection to the router.
func (pm *peerManager) readPump(conn *websocket.Conn, connID string, isHost bool) {
	if isHost {
		defer pm.removeHost(connID)
	} else {
		defer conn.Close()
	}

	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error { conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		var msg signal.Message
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				glog.Errorf("Error reading message from %s: %v", connID, err)
			} else {
				glog.Infof("Connection from %s closed.", connID)
			}
			break // Exit loop on error or close
		}

		if err := pm.routeMessage(conn, msg); err != nil {
			glog.Errorf("Routing error from %s: %v", connID, err)
		}
	}
}

// routeMessage forwards a message to a target host's send channel.
func (pm *peerManager) routeMessage(senderConn *websocket.Conn, msg signal.Message) error {
	targetID := msg.TargetID
	if targetID == "" {
		return fmt.Errorf("message from %s has no target ID", msg.SenderID)
	}

	target := pm.getHost(targetID)
	if target == nil {
		errText := fmt.Sprintf("target host '%s' not found", targetID)
		glog.Warning(errText)
		// Try to send error back to the sender
		errRsp := signal.Message{Type: signal.MessageTypeError, Payload: errText, TargetID: msg.SenderID}
		senderConn.SetWriteDeadline(time.Now().Add(writeWait))
		_ = senderConn.WriteJSON(errRsp)
		return fmt.Errorf(errText)
	}

	glog.Infof("Routing message from '%s' to '%s' (type: %s)", msg.SenderID, msg.TargetID, msg.Type)

	b, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message for target '%s': %v", targetID, err)
	}

	select {
	case target.send <- b:
	default:
		glog.Warningf("Dropping message for host %s, send channel full", targetID)
	}

	return nil
}

func isAuthorized(r *http.Request, validTokens []string) bool {
	// If no tokens are configured on the server, allow all connections.
	if len(validTokens) == 0 {
		return true
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return false
	}
	token := parts[1]

	for _, validToken := range validTokens {
		if token == validToken {
			return true
		}
	}

	return false
}

// Run starts the signaling server.
func Run(addr, certFile, keyFile string, allowedOrigins, validTokens []string) error {
	peerManager := newPeerManager()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			// Check if all origins are allowed
			if len(allowedOrigins) == 1 && allowedOrigins[0] == "*" {
				return true
			}
			// Otherwise, check against the whitelist
			origin := r.Header.Get("Origin")
			for _, allowed := range allowedOrigins {
				if origin == allowed {
					return true
				}
			}
			glog.Warningf("Rejecting connection from disallowed origin: %s", origin)
			return false
		},
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthorized(r, validTokens) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			glog.Warningf("Rejecting unauthorized connection from %s", r.RemoteAddr)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			glog.Errorf("Failed to upgrade connection: %v", err)
			return
		}
		go peerManager.handleConnection(conn)
	})

	useTLS := certFile != "" && keyFile != ""
	if useTLS {
		glog.Infof("Signaling server starting with TLS on %s", addr)
		return http.ListenAndServeTLS(addr, certFile, keyFile, nil)
	}

	glog.Infof("Signaling server starting without TLS on %s", addr)
	return http.ListenAndServe(addr, nil)
}
