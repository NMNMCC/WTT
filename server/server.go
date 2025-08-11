package server

import (
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

func Run(cfg ServerConfig) error {
	connM := hashmap.New[string, *websocket.Conn]()

	upgrader := websocket.Upgrader{}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		token := strings.Split(r.Header.Get("Authorization"), " ")[1]
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
