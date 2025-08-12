package rtc

import (
	"encoding/json"
	"log/slog"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

func CreatePeerConnection(cfg webrtc.Configuration) (*webrtc.PeerConnection, error) {
	pc, err := webrtc.NewPeerConnection(cfg)
	if err != nil {
		return nil, err
	}
	return pc, nil
}

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

func SetLocalDescription(pc *webrtc.PeerConnection, desc webrtc.SessionDescription) error {
	if err := pc.SetLocalDescription(desc); err != nil {
		return err
	}
	return nil
}

func SetRemoteDescription(pc *webrtc.PeerConnection, desc webrtc.SessionDescription) error {
	if err := pc.SetRemoteDescription(desc); err != nil {
		return err
	}
	return nil
}

func SendSignal(wsc *websocket.Conn, signal json.RawMessage) error {
	if err := wsc.WriteJSON(signal); err != nil {
		return err
	}
	return nil
}
