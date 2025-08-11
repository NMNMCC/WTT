package server

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"wtt/common"

	"github.com/cornelk/hashmap"
	"github.com/golang/glog"
	"github.com/gorilla/websocket"
)

type ServerConfig struct {
	ListenAddr string
	Tokens     []string
}

func (cfg ServerConfig) Validate() error {
	if cfg.ListenAddr == "" {
		return fmt.Errorf("listen address is required")
	}
	if len(cfg.Tokens) == 0 {
		return fmt.Errorf("at least one authentication token is required")
	}
	return nil
}

func Run(cfg ServerConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid server configuration: %v", err)
	}
	connM := hashmap.New[string, *websocket.Conn]()

	upgrader := websocket.Upgrader{}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized: Missing Authorization header", http.StatusUnauthorized)
			glog.Warningf("Rejecting connection from %s: missing Authorization header", r.RemoteAddr)
			return
		}
		
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Unauthorized: Invalid Authorization header format", http.StatusUnauthorized)
			glog.Warningf("Rejecting connection from %s: invalid Authorization header format", r.RemoteAddr)
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

		var msg common.Message[any]
		if err := conn.ReadJSON(&msg); err != nil {
			glog.Errorf("Failed to read initial message: %v", err)
			return
		}

		connM.Set(msg.SenderID, conn)
		conn.SetCloseHandler(func(code int, text string) error {
			connM.Del(msg.SenderID)
			return nil
		})

		conn, ok := connM.Get(msg.TargetID)
		if !ok {
			glog.Errorf("Connection not found for target ID %s", msg.TargetID)
			return
		}
		if msg.TargetID != "" {
			conn.WriteJSON(msg)
		}

	})

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: mux}

	glog.Infof("Signaling server starting on %s", cfg.ListenAddr)
	return srv.ListenAndServe()
}
