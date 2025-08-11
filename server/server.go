package server

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
	"wtt/common"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
)

type ServerConfig struct {
	ListenAddr string
	Tokens     []string
}

func Run(ctx context.Context, cfg ServerConfig) error {
	connM := &sync.Map{}
	upgrader := websocket.Upgrader{}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			glog.Warningf("Rejecting connection from %s: missing authorization header", r.RemoteAddr)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			glog.Warningf("Rejecting connection from %s: invalid authorization header format", r.RemoteAddr)
			return
		}
		token := parts[1]

		if !slices.Contains(cfg.Tokens, token) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			glog.Warningf("Rejecting unauthorized connection from %s", r.RemoteAddr)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			glog.Errorf("Failed to upgrade connection: %v", err)
			return
		}

		go HandleConnection(conn, connM)
	})

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: mux}

	// Goroutine for graceful shutdown
	go func() {
		<-ctx.Done()
		glog.Info("Shutting down signaling server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	glog.Infof("Signaling server starting on %s", cfg.ListenAddr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}

	return nil
}

func HandleConnection(conn *websocket.Conn, connM *sync.Map) {
	var initialMsg common.Message[any]
	if err := conn.ReadJSON(&initialMsg); err != nil {
		glog.Errorf("Failed to read initial message: %v", err)
		conn.Close()
		return
	}

	senderID := initialMsg.SenderID
	if senderID == "" {
		glog.Errorf("Initial message is missing sender ID")
		conn.Close()
		return
	}

	connM.Store(senderID, conn)
	glog.Infof("Client '%s' connected", senderID)

	defer func() {
		connM.Delete(senderID)
		conn.Close()
		glog.Infof("Client '%s' disconnected", senderID)
	}()

	if initialMsg.TargetID != "" {
		forwardMessage(initialMsg, connM)
	}

	for {
		_, p, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				glog.Errorf("Error reading message from %s: %v", senderID, err)
			}
			break
		}

		var msg common.Message[any]
		if err := json.Unmarshal(p, &msg); err != nil {
			glog.Errorf("Failed to unmarshal message from %s: %v", senderID, err)
			continue
		}

		forwardMessage(msg, connM)
	}
}

func forwardMessage(msg common.Message[any], connM *sync.Map) {
	targetID := msg.TargetID
	if targetID == "" {
		glog.Warningf("Message from %s has no target ID, dropping", msg.SenderID)
		return
	}

	targetConn, ok := connM.Load(targetID)
	if !ok {
		glog.Errorf("Connection not found for target ID %s (from sender %s)", targetID, msg.SenderID)
		return
	}

	wsConn, ok := targetConn.(*websocket.Conn)
	if !ok {
		glog.Errorf("Invalid connection type in map for target ID %s", targetID)
		return
	}

	if err := wsConn.WriteJSON(msg); err != nil {
		glog.Errorf("Failed to forward message to %s: %v", targetID, err)
	}
}
