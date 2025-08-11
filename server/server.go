package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
	"wtt/common"

	cmap "github.com/orcaman/concurrent-map/v2"

	"github.com/gorilla/websocket"
)

type ServerConfig struct {
	ListenAddr string
	Tokens     []string
}

func Run(ctx context.Context, cfg ServerConfig) error {
	connM := cmap.New[*websocket.Conn]()
	upgrader := websocket.Upgrader{}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			slog.Warn("Rejecting connection: missing authorization header", "remote_addr", r.RemoteAddr)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			slog.Warn("Rejecting connection: invalid authorization header format", "remote_addr", r.RemoteAddr)
			return
		}
		token := parts[1]

		if !slices.Contains(cfg.Tokens, token) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			slog.Warn("Rejecting unauthorized connection", "remote_addr", r.RemoteAddr)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("Failed to upgrade connection", "error", err)
			return
		}

		go HandleConnection(conn, connM)
	})

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: mux}

	// Goroutine for graceful shutdown
	go func() {
		<-ctx.Done()
		slog.Info("Shutting down signaling server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	slog.Info("Signaling server starting", "addr", cfg.ListenAddr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}

	return nil
}

func HandleConnection(conn *websocket.Conn, connM cmap.ConcurrentMap[string, *websocket.Conn]) {
	var initialMsg common.Message
	if err := conn.ReadJSON(&initialMsg); err != nil {
		slog.Error("Failed to read initial message", "error", err)
		conn.Close()
		return
	}

	senderID := initialMsg.SenderID
	if senderID == "" {
		slog.Error("Initial message is missing sender ID")
		conn.Close()
		return
	}

	connM.Set(senderID, conn)
	slog.Info("Client connected", "client_id", senderID)

	defer func() {
		connM.Remove(senderID)
		conn.Close()
		slog.Info("Client disconnected", "client_id", senderID)
	}()

	if initialMsg.TargetID != "" {
		forwardMessage(initialMsg, connM)
	}

	for {
		_, p, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Error("Error reading message", "sender_id", senderID, "error", err)
			}
			break
		}

		var msg common.Message
		if err := json.Unmarshal(p, &msg); err != nil {
			slog.Error("Failed to unmarshal message", "sender_id", senderID, "error", err)
			continue
		}

		forwardMessage(msg, connM)
	}
}

func forwardMessage(msg common.Message, connM cmap.ConcurrentMap[string, *websocket.Conn]) {
	targetID := msg.TargetID
	if targetID == "" {
		slog.Warn("Message has no target ID, dropping", "sender_id", msg.SenderID)
		return
	}

	wsConn, ok := connM.Get(targetID)
	if !ok {
		slog.Error("Connection not found for target", "target_id", targetID, "sender_id", msg.SenderID)
		return
	}

	if err := wsConn.WriteJSON(msg); err != nil {
		slog.Error("Failed to forward message", "target_id", targetID, "error", err)
	}
}
