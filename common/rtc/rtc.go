package rtc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"wtt/common"

	"github.com/go-resty/resty/v2"
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
	return pc.SetLocalDescription(desc)
}

func SetRemoteDescription(pc *webrtc.PeerConnection, desc webrtc.SessionDescription) error {
	return pc.SetRemoteDescription(desc)
}

func RegisterHost(c *resty.Client, hostID string) error {
	res, err := c.R().Head("/" + string(common.RTCRegisterType) + "/" + hostID)
	if err != nil {
		return err
	}
	slog.Debug("registered host", "id", hostID, "status", res.Status())

	return nil
}

func SendRTCEvent[T common.RTCEventType](c *resty.Client, typ T, hostID string, signal webrtc.SessionDescription) error {
	slog.Debug("sending signal", "server", c.BaseURL, "type", typ, "hostID", hostID)

	res, err := c.R().SetBody(signal).Post("/" + string(typ) + "/" + hostID)
	if err != nil {
		return err
	}
	if res.StatusCode() != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", res.StatusCode())
	}
	slog.Debug("signal sent", "type", typ, "status", res.Status())

	return nil
}

func ReceiveRTCEvent[T common.RTCEventType](c *resty.Client, typ T, hostID string) (*webrtc.SessionDescription, error) {
	slog.Debug("receiving signal", "server", c.BaseURL, "type", typ, "hostID", hostID)

	res, err := c.R().Get("/" + string(typ) + "/" + hostID)
	if err != nil {
		return nil, err
	}
	if res.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode())
	}
	slog.Debug("signal received", "type", typ)

	var signal webrtc.SessionDescription
	if err := json.Unmarshal(res.Body(), &signal); err != nil {
		return nil, err
	}

	return &signal, nil
}
