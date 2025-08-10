package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
	"webrtc-tunnel/signal"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 10240
)

type safeConnection struct {
	conn *websocket.Conn
	send chan []byte
}

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
				return
			}
		}
	}
}

type addHostRequest struct {
	id   string
	conn *websocket.Conn
	resp chan *addHostResponse
}

type addHostResponse struct {
	safeConn *safeConnection
	err      error
}

type removeHostRequest struct {
	id string
}

type routeMessageRequest struct {
	msg      signal.Message
	resp     chan error
	sender   *websocket.Conn
}

type peerManager struct {
	peers       map[string]*safeConnection
	addHost     chan *addHostRequest
	removeHost  chan *removeHostRequest
	routeMessage chan *routeMessageRequest
}

func newPeerManager() *peerManager {
	return &peerManager{
		peers:       make(map[string]*safeConnection),
		addHost:     make(chan *addHostRequest),
		removeHost:  make(chan *removeHostRequest),
		routeMessage: make(chan *routeMessageRequest),
	}
}

func (pm *peerManager) run() {
	for {
		select {
		case req := <-pm.addHost:
			if _, ok := pm.peers[req.id]; ok {
				req.resp <- &addHostResponse{err: fmt.Errorf("host with ID '%s' already registered", req.id)}
				continue
			}
			safeConn := &safeConnection{conn: req.conn, send: make(chan []byte, 256)}
			pm.peers[req.id] = safeConn
			go safeConn.writePump()
			glog.Infof("Host registered: %s", req.id)
			req.resp <- &addHostResponse{safeConn: safeConn}
		case req := <-pm.removeHost:
			if safeConn, ok := pm.peers[req.id]; ok {
				close(safeConn.send)
				delete(pm.peers, req.id)
				glog.Infof("Host removed: %s", req.id)
			}
		case req := <-pm.routeMessage:
			targetID := req.msg.TargetID
			if targetID == "" {
				req.resp <- fmt.Errorf("message from %s has no target ID", req.msg.SenderID)
				continue
			}
			target, ok := pm.peers[targetID]
			if !ok {
				errText := fmt.Sprintf("target host '%s' not found", targetID)
				glog.Warning(errText)
				errRsp := signal.Message{Type: signal.MessageTypeError, Payload: errText, TargetID: req.msg.SenderID}
				req.sender.SetWriteDeadline(time.Now().Add(writeWait))
				_ = req.sender.WriteJSON(errRsp)
				req.resp <- fmt.Errorf(errText)
				continue
			}
			glog.Infof("Routing message from '%s' to '%s' (type: %s)", req.msg.SenderID, req.msg.TargetID, req.msg.Type)
			b, err := json.Marshal(req.msg)
			if err != nil {
				req.resp <- fmt.Errorf("failed to marshal message for target '%s': %v", targetID, err)
				continue
			}
			select {
			case target.send <- b:
			default:
				glog.Warningf("Dropping message for host %s, send channel full", targetID)
			}
			req.resp <- nil
		}
	}
}

func (pm *peerManager) handleConnection(conn *websocket.Conn) {
	var msg signal.Message
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	err := conn.ReadJSON(&msg)
	conn.SetReadDeadline(time.Time{})
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
		respChan := make(chan *addHostResponse)
		pm.addHost <- &addHostRequest{id: hostID, conn: conn, resp: respChan}
		resp := <-respChan
		if resp.err != nil {
			glog.Errorf("Host registration failed: %v", resp.err)
			conn.Close()
			return
		}
		pm.readPump(conn, hostID, true)
		return
	}

	respChan := make(chan error)
	pm.routeMessage <- &routeMessageRequest{msg: msg, resp: respChan, sender: conn}
	if err := <-respChan; err != nil {
		glog.Errorf("Failed to route initial client message: %v", err)
	}
	pm.readPump(conn, msg.SenderID, false)
}

func (pm *peerManager) readPump(conn *websocket.Conn, connID string, isHost bool) {
	if isHost {
		defer func() {
			pm.removeHost <- &removeHostRequest{id: connID}
		}()
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
			break
		}

		respChan := make(chan error)
		pm.routeMessage <- &routeMessageRequest{msg: msg, resp: respChan, sender: conn}
		if err := <-respChan; err != nil {
			glog.Errorf("Routing error from %s: %v", connID, err)
		}
	}
}

func isAuthorized(r *http.Request, validTokens []string) bool {
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

func Run(addr, certFile, keyFile string, allowedOrigins, validTokens []string) error {
	pm := newPeerManager()
	go pm.run()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			if len(allowedOrigins) == 1 && allowedOrigins[0] == "*" {
				return true
			}
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
		go pm.handleConnection(conn)
	})

	useTLS := certFile != "" && keyFile != ""
	if useTLS {
		glog.Infof("Signaling server starting with TLS on %s", addr)
		return http.ListenAndServeTLS(addr, certFile, keyFile, nil)
	}
	glog.Infof("Signaling server starting without TLS on %s", addr)
	return http.ListenAndServe(addr, nil)
}
