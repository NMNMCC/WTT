// Package rtc provides common helper functions for setting up WebRTC peer connections,
// used by both the offerer and the answerer.
package rtc

import (
	"encoding/json"
	"log/slog"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

// CreatePeerConnection creates and returns a new WebRTC PeerConnection.
func CreatePeerConnection(cfg webrtc.Configuration) (*webrtc.PeerConnection, error) {
	pc, err := webrtc.NewPeerConnection(cfg)
	if err != nil {
		return nil, err
	}
	return pc, nil
}

// CreateDataChannel creates a new DataChannel for the given PeerConnection.
func CreateDataChannel(pc *webrtc.PeerConnection, label string) (*webrtc.DataChannel, error) {
	if len(label) == 0 {
		slog.Warn("DataChannel label is empty, using default 'data'")
		label = "data"
	}
	dc, err := pc.CreateDataChannel(label, nil)
	if err != nil {
		return nil, err
	}
	return dc, nil
}

// SetLocalDescription sets the local session description for the PeerConnection.
func SetLocalDescription(pc *webrtc.PeerConnection, desc webrtc.SessionDescription) error {
	if err := pc.SetLocalDescription(desc); err != nil {
		return err
	}
	return nil
}

// SetRemoteDescription sets the remote session description for the PeerConnection.
func SetRemoteDescription(pc *webrtc.PeerConnection, desc webrtc.SessionDescription) error {
	if err := pc.SetRemoteDescription(desc); err != nil {
		return err
	}
	return nil
}

// SendSignal sends a signaling message (as a JSON raw message) over the WebSocket connection.
func SendSignal(wsc *websocket.Conn, signal json.RawMessage) error {
	if err := wsc.WriteJSON(signal); err != nil {
		return err
	}
	return nil
}
