package common

import (
	"context"
	"net/http"

	"github.com/gorilla/websocket"
)

// MessageType defines the type of a WebSocket message.
type MessageType string

// Constants for the different types of WebSocket messages.
const (
	MessageTypeForward MessageType = "forward" // A message to be forwarded to another peer.
	MessageTypeReverse MessageType = "reverse" // A message sent in response to a forwarded message.
)

// WebSocketForwardMessageContainer is a generic container for a message that should be forwarded by the server.
// It wraps the actual content and adds metadata for routing.
type WebSocketForwardMessageContainer[C any] struct {
	MessageID  string      `json:"message_id,omitempty"` // An optional ID for the message, used for tracking responses.
	Type       MessageType `json:"type"`                 // The type of the message, should be MessageTypeForward.
	ReceiverID string      `json:"target_id"`            // The ID of the recipient peer.
	Content    C           `json:"payload"`              // The actual content of the message.
}

// WebSocketReverseMessageContainer is a generic container for a response to a forwarded message.
type WebSocketReverseMessageContainer[C any] struct {
	Type             MessageType `json:"type"`               // The type of the message, should be MessageTypeReverse.
	ForwardMessageID string      `json:"forward_message_id"` // The ID of the original message that this is a response to.
	Content          C           `json:"payload"`            // The actual content of the response.
}

// WebSocketMessageContainer is a constraint that defines the possible types for a WebSocket message container.
type WebSocketMessageContainer[C any] interface {
	WebSocketForwardMessageContainer[C] | WebSocketReverseMessageContainer[C]
}

// ConnectToWebSocketServer establishes a WebSocket connection to the given address.
func ConnectToWebSocketServer(ctx context.Context, addr string) (*websocket.Conn, error) {
	d := websocket.Dialer{}
	conn, _, err := d.DialContext(ctx, addr, http.Header{})
	if err != nil {
		return nil, err
	}
	return conn, nil
}
