package client

import (
	"context"
	"fmt"
	"net"
	"time"

	"wtt/common"

	"github.com/golang/glog"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

type ClientConfig struct {
	ID        string
	HostID    string
	SigAddr   string
	LocalAddr string
	Protocol  common.Protocol
	STUNAddrs []string
	Token     string
	Timeout   int // seconds
}

func (cfg ClientConfig) Validate() error {
	if cfg.HostID == "" {
		return fmt.Errorf("client mode requires a target host ID")
	}
	if cfg.SigAddr == "" {
		return fmt.Errorf("client mode requires a signaling server address")
	}
	if cfg.LocalAddr == "" {
		return fmt.Errorf("client mode requires a local address to listen on")
	}
	if cfg.Protocol != common.TCP && cfg.Protocol != common.UDP {
		return fmt.Errorf("unsupported protocol: %s", cfg.Protocol)
	}
	if cfg.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive, got: %d", cfg.Timeout)
	}
	return nil
}

func Run(cfg ClientConfig) {
	err := cfg.Validate()
	if err != nil {
		glog.Errorf("invalid client configuration: %v", err)
		return
	}

	wsc, err := common.WebSocketConn(cfg.SigAddr, cfg.Token)
	if err != nil {
		glog.Fatalf("failed to connect to signaling server: %v", err)
		return
	}

	pc, err := emit(wsc, cfg.STUNAddrs, cfg.ID, cfg.HostID, time.Duration(cfg.Timeout)*time.Second)
	if err != nil {
		glog.Fatalf("failed to establish PeerConnection: %v", err)
		return
	}

	err = tunnel(pc, cfg.Protocol, cfg.LocalAddr)
	if err != nil {
		glog.Fatalf("failed to set up tunnel: %v", err)
		return
	}

	glog.Infof("Client '%s' connected to host '%s' and listening on %s", cfg.ID, cfg.HostID, cfg.LocalAddr)
}

func emit(wsc *websocket.Conn, st []string, clientID, hostID string, timeout time.Duration) (*webrtc.PeerConnection, error) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: st}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create PeerConnection: %v", err)
	}

	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}

		cand := common.Message[common.CandidatePayload]{
			Type:     common.Candidate,
			Payload:  common.CandidatePayload{Candidate: i.ToJSON()},
			TargetID: hostID,
			SenderID: clientID,
		}

		_ = wsc.WriteJSON(cand)
	})

	// Create and set local description before accessing it
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create offer: %v", err)
	}
	
	if err := pc.SetLocalDescription(offer); err != nil {
		return nil, fmt.Errorf("failed to set local description: %v", err)
	}

	offerMsg := common.Message[common.OfferPayload]{
		Type: common.Offer,
		Payload: common.OfferPayload{
			SDP: offer,
		},
		TargetID: hostID,
		SenderID: clientID,
	}
	_ = wsc.WriteJSON(offerMsg)

	connected := make(chan struct{})
	pc.OnConnectionStateChange(func(pcs webrtc.PeerConnectionState) {
		if pcs == webrtc.PeerConnectionStateConnected {
			close(connected)
		}
	})

	timer, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case <-connected:

		return pc, nil
	case <-timer.Done():

		return nil, fmt.Errorf("connection timed out after %d seconds", timeout)
	}
}

func tunnel(pc *webrtc.PeerConnection, protocol common.Protocol, localAddr string) error {
	id, _ := uuid.NewRandom()
	switch protocol {
	case common.TCP:
		localConn, err := net.Dial(string(common.TCP), localAddr)
		if err != nil {
			return fmt.Errorf("failed to connect to local address %s: %v", localAddr, err)
		}
		dc, err := pc.CreateDataChannel(id.String(), nil)
		if err != nil {
			return fmt.Errorf("failed to create DataChannel: %v", err)
		}
		if err := common.BridgeStream(dc, localConn); err != nil {
			return fmt.Errorf("failed to bridge DataChannel: %v", err)
		}
	case common.UDP:
		localConn, err := net.ListenPacket(string(common.UDP), localAddr)
		if err != nil {
			return fmt.Errorf("failed to listen on local address %s: %v", localAddr, err)
		}
		dc, err := pc.CreateDataChannel(id.String(), nil)
		if err != nil {
			return fmt.Errorf("failed to create DataChannel: %v", err)
		}
		if err := common.BridgePacket(dc, localConn); err != nil {
			return fmt.Errorf("failed to bridge DataChannel: %v", err)
		}
	default:
		return fmt.Errorf("unsupported protocol: %s", protocol)
	}

	return nil
}
