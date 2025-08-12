package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"wtt/common"

	"github.com/cornelk/hashmap"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func Run(ctx context.Context, listenAddr string, tokens []string, maxMsgSize int64) <-chan error {
	connM := hashmap.New[string, *common.RWLock[*websocket.Conn]]()

	ec := make(chan error)

	srv := http.NewServeMux()

	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("upgrade error", "err", err)
			ec <- err
			return
		}
		defer conn.Close()
		slog.Info("new connection", "remote", conn.RemoteAddr())
		conn.SetReadLimit(maxMsgSize)
		id, err := uuid.NewRandom()
		if err != nil {
			slog.Error("generate uuid error", "err", err)
			ec <- err
			return
		}
		connM.Set(id.String(), common.NewRWLock(conn))
		slog.Info("new client", "id", id)

		var anyM common.WebSocketForwardMessageContainer[json.RawMessage]
		if err := conn.ReadJSON(&anyM); err != nil {
			slog.Error("read json error", "err", err)
			ec <- err
			return
		}
		slog.Debug("forward message", "receiver", anyM.ReceiverID, "bytes", len(anyM.Content))

		if conn, ok := connM.Get(anyM.ReceiverID); ok {
			go conn.Write(func(c *websocket.Conn) {
				if err := c.WriteJSON(anyM.Content); err != nil {
					slog.Error("write json error", "err", err)
					ec <- err
				}
			})
		} else {
			slog.Warn("receiver not found", "id", anyM.ReceiverID)
		}
	})

	slog.Info("server listening", "addr", listenAddr)
	err := http.ListenAndServe(listenAddr, srv)
	if err != nil {
		slog.Error("listen and serve error", "err", err)
		ec <- err
	}

	return ec
}
