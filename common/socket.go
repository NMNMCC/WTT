package common

import (
	"context"
	"net/http"

	"github.com/gorilla/websocket"
)

type MessageType string

const (
	MessageTypeForward MessageType = "forward"
	MessageTypeReverse MessageType = "reverse"
)

type WebSocketForwardMessageContainer[C any] struct {
	// when sending a message to the server, leave it empty
	MessageID  string      `json:"message_id,omitempty"`
	Type       MessageType `json:"type"`
	ReceiverID string      `json:"target_id"`
	Content    C           `json:"payload"`
}

type WebSocketReverseMessageContainer[C any] struct {
	Type             MessageType `json:"type"`
	ForwardMessageID string      `json:"forward_message_id"`
	Content          C           `json:"payload"`
}

type WebSocketMessageContainer[C any] interface {
	WebSocketForwardMessageContainer[C] | WebSocketReverseMessageContainer[C]
}

func ConnectToWebSocketServer(ctx context.Context, addr string) (*websocket.Conn, error) {
	d := websocket.Dialer{}
	conn, _, err := d.DialContext(ctx, addr, http.Header{})
	if err != nil {
		return nil, err
	}
	return conn, nil
}
