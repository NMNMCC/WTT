package common

import (
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
)

func WebSocketConn(addr, token string) (*websocket.Conn, error) {
	header := http.Header{}
	if token != "" {
		header.Add("Authorization", "Bearer "+token)
	}
	wsc, _, err := websocket.DefaultDialer.Dial(addr, header)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to signaling server: %v", err)
	}

	return wsc, nil
}
