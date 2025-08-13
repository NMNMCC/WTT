package rtc

import (
	"log/slog"
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
	if err := pc.SetLocalDescription(desc); err != nil {
		return err
	}
	return nil
}

func SetRemoteDescription(pc *webrtc.PeerConnection, desc webrtc.SessionDescription) error {
	return pc.SetRemoteDescription(desc)
}

func RegisterHost(c *resty.Client, hostID string) error {
	reg := common.RTCRegister{
		HostID: hostID,
	}

	res, err := c.R().SetBody(reg).Post(string(common.RTCRegisterType))
	if err != nil {
		return err
	}
	slog.Info("registered host", "id", hostID, "status", res.Status())

	return nil
}

func SendSignal[T common.RTCEventType, S common.RTCEventPayload](c *resty.Client, typ T, signal S) error {
	res, err := c.R().SetBody(signal).Post(string(typ))
	if err != nil {
		return err
	}
	slog.Info("signal sent", "type", typ, "status", res.Status())

	return nil
}

func ReceiveSignal[T common.RTCEventType](c *resty.Client, typ T) (webrtc.SessionDescription, error) {
	var signal webrtc.SessionDescription
	_, err := c.R().SetResult(&signal).Get(string(typ))
	if err != nil {
		return signal, err
	}
	slog.Info("signal received", "type", typ)

	return signal, nil
}
