package client

import (
	"context"
	"encoding/json"
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

func Run(ctx context.Context, cfg ClientConfig) error {
	if cfg.ID == "" {
		cfg.ID = uuid.NewString()
		glog.Infof("No client ID provided, generated one: %s", cfg.ID)
	}

	wsc, err := common.WebSocketConn(cfg.SigAddr, cfg.Token)
	if err != nil {
		return fmt.Errorf("failed to connect to signaling server: %w", err)
	}
	defer wsc.Close()

	pc, dc, err := negotiate(ctx, wsc, cfg)
	if err != nil {
		return fmt.Errorf("failed to establish PeerConnection: %w", err)
	}
	defer pc.Close()

	if err := tunnel(ctx, pc, dc, cfg.Protocol, cfg.LocalAddr); err != nil {
		return fmt.Errorf("failed to set up tunnel: %w", err)
	}

	glog.Infof("Client '%s' connected to host '%s'", cfg.ID, cfg.HostID)

	// Wait for context cancellation
	<-ctx.Done()
	glog.Info("Client shutting down...")
	return nil
}

func negotiate(ctx context.Context, wsc *websocket.Conn, cfg ClientConfig) (*webrtc.PeerConnection, *webrtc.DataChannel, error) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: cfg.STUNAddrs}},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create PeerConnection: %w", err)
	}

	connected := make(chan struct{})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		glog.Infof("PeerConnection state changed: %s", state.String())
		if state == webrtc.PeerConnectionStateConnected {
			close(connected)
		}
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			select {
			case <-connected:
			default:
				close(connected)
			}
		}
	})

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		payload := common.CandidatePayload{Candidate: c.ToJSON()}
		msg := common.Message[common.CandidatePayload]{
			Type:     common.Candidate,
			Payload:  payload,
			TargetID: cfg.HostID,
			SenderID: cfg.ID,
		}
		if err := wsc.WriteJSON(msg); err != nil {
			glog.Errorf("Failed to send ICE candidate: %v", err)
		}
	})

	dc, err := pc.CreateDataChannel("wtt", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create DataChannel: %w", err)
	}

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create offer: %w", err)
	}

	if err := pc.SetLocalDescription(offer); err != nil {
		return nil, nil, fmt.Errorf("failed to set local description: %w", err)
	}

	offerMsg := common.Message[common.OfferPayload]{
		Type:     common.Offer,
		Payload:  common.OfferPayload{SDP: offer},
		TargetID: cfg.HostID,
		SenderID: cfg.ID,
	}
	if err := wsc.WriteJSON(offerMsg); err != nil {
		return nil, nil, fmt.Errorf("failed to send offer: %w", err)
	}

	go func() {
		for {
			_, p, err := wsc.ReadMessage()
			if err != nil {
				glog.Warningf("Error reading from websocket: %v", err)
				pc.Close()
				return
			}
			var msg common.Message[any]
			if err := json.Unmarshal(p, &msg); err != nil {
				glog.Errorf("Failed to unmarshal message: %v", err)
				continue
			}

			switch msg.Type {
			case common.Answer:
				answer, err := common.ReUnmarshal[common.Message[common.AnswerPayload]](msg)
				if err != nil {
					glog.Errorf("Failed to unmarshal answer payload: %v", err)
					continue
				}
				if err := pc.SetRemoteDescription(answer.Payload.SDP); err != nil {
					glog.Errorf("Failed to set remote description: %v", err)
				}
			case common.Candidate:
				candidate, err := common.ReUnmarshal[common.Message[common.CandidatePayload]](msg)
				if err != nil {
					glog.Errorf("Failed to unmarshal candidate payload: %v", err)
					continue
				}
				if err := pc.AddICECandidate(candidate.Payload.Candidate); err != nil {
					glog.Errorf("Failed to add ICE candidate: %v", err)
				}
			}
		}
	}()

	timeout, cancel := context.WithTimeout(ctx, time.Duration(cfg.Timeout)*time.Second)
	defer cancel()

	select {
	case <-connected:
		if pc.ConnectionState() == webrtc.PeerConnectionStateConnected {
			glog.Info("PeerConnection established")
			return pc, dc, nil
		}
		return nil, nil, fmt.Errorf("peer connection failed to connect, state is %s", pc.ConnectionState().String())
	case <-timeout.Done():
		pc.Close()
		return nil, nil, fmt.Errorf("connection timed out after %d seconds", cfg.Timeout)
	}
}

func tunnel(ctx context.Context, pc *webrtc.PeerConnection, dc *webrtc.DataChannel, protocol common.Protocol, localAddr string) error {
	listener, err := net.Listen(string(protocol), localAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on local address %s: %w", localAddr, err)
	}
	glog.Infof("Client listening on %s", localAddr)

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	dc.OnOpen(func() {
		glog.Infof("DataChannel '%s' opened, waiting for local connections.", dc.Label())
		go func() {
			for {
				localConn, err := listener.Accept()
				if err != nil {
					glog.Warningf("Failed to accept local connection: %v", err)
					if ctx.Err() != nil {
						return
					}
					continue
				}
				glog.Infof("Accepted local connection from %s", localConn.RemoteAddr())
				if err := common.BridgeStream(dc, localConn); err != nil {
					glog.Errorf("Failed to bridge connection: %v", err)
				}
				glog.Info("Bridged connection closed.")
			}
		}()
	})

	return nil
}
