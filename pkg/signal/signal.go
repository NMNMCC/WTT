package signal

// Message is the generic wrapper for all WebSocket messages.
type Message struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

// --- Client -> Server Payloads ---

// PublishServicesPayload is the payload for the "publish_services" message.
type PublishServicesPayload struct {
	Services []string `json:"services"`
}

// RequestConnectionPayload is the payload for the "request_connection" message.
type RequestConnectionPayload struct {
	TargetPeerID string `json:"target_peer_id"`
	ServiceName  string `json:"service_name"`
}

// SignalPayload is the payload for the "signal" message.
type SignalPayload struct {
	ToPeerID string      `json:"to_peer_id"`
	Data     interface{} `json:"data"` // Can contain SDP or ICE candidate
}

// --- Server -> Client Payloads ---

// RegisteredPayload is the payload for the "registered" message.
type RegisteredPayload struct {
	PeerID     string      `json:"peer_id"`
	IceServers interface{} `json:"ice_servers"` // Using interface{} to be flexible
}

// ConnectionRequestPayload is the payload for the "connection_request" message.
type ConnectionRequestPayload struct {
	FromPeerID  string `json:"from_peer_id"`
	ServiceName string `json:"service_name"`
}

// SignalData is the inner data for the "signal" message from the server.
type SignalData struct {
	FromPeerID string      `json:"from_peer_id"`
	Data       interface{} `json:"data"`
}
