package server

import (
	"context"
	"encoding/json"
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
			ec <- err
		}
		defer conn.Close()
		conn.SetReadLimit(maxMsgSize)
		id, err := uuid.NewRandom()
		if err != nil {
			ec <- err
		}
		connM.Set(id.String(), common.NewRWLock(conn))

		var anyM common.WebSocketForwardMessageContainer[json.RawMessage]
		if err := conn.ReadJSON(&anyM); err != nil {
			ec <- err
		}

		if conn, ok := connM.Get(anyM.ReceiverID); ok {
			go conn.Write(func(c *websocket.Conn) {
				if err := c.WriteJSON(anyM.Content); err != nil {
					ec <- err
				}
			})
		}
	})

	err := http.ListenAndServe(listenAddr, srv)
	if err != nil {
		ec <- err
	}

	return ec
}
