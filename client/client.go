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

func check(cfg ClientConfig) error {
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
	return nil
}

func Run(ctx context.Context, cfg ClientConfig) {
	if err := check(cfg); err != nil {
		glog.Fatalf("Invalid client configuration: %v", err)
	}

	if cfg.ID == "" {
		cfg.ID = uuid.NewString()
		glog.Infof("No client ID provided, generated one: %s", cfg.ID)
	}

	wsc, err := common.WebSocketConn(cfg.SigAddr, cfg.Token)
	if err != nil {
		glog.Fatalf("Failed to connect to signaling server: %v", err)
	}
	defer wsc.Close()

	pc, dc, err := negotiate(ctx, wsc, cfg)
	if err != nil {
		glog.Fatalf("Failed to establish PeerConnection: %v", err)
	}
	defer pc.Close()

	if err := tunnel(ctx, pc, dc, cfg.Protocol, cfg.LocalAddr); err != nil {
		glog.Fatalf("Failed to set up tunnel: %v", err)
	}

	glog.Infof("Client '%s' connected to host '%s' and listening on %s", cfg.ID, cfg.HostID, cfg.LocalAddr)

	// Wait for context cancellation
	<-ctx.Done()
	glog.Info("Client shutting down...")
}

func negotiate(ctx context.Context, wsc *websocket.Conn, cfg ClientConfig) (*webrtc.PeerConnection, *webrtc.DataChannel, error) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: cfg.STUNAddrs}},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create PeerConnection: %v", err)
	}

	connected := make(chan struct{})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		glog.Infof("PeerConnection state changed: %s", state.String())
		if state == webrtc.PeerConnectionStateConnected {
			close(connected)
		}
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			// If connection fails or closes, we should stop trying to negotiate
			select {
			case <-connected:
				// already connected and closed, do nothing
			default:
				// if not connected yet, close the channel to signal failure
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
		return nil, nil, fmt.Errorf("failed to create DataChannel: %v", err)
	}

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create offer: %v", err)
	}

	if err := pc.SetLocalDescription(offer); err != nil {
		return nil, nil, fmt.Errorf("failed to set local description: %v", err)
	}

	offerMsg := common.Message[common.OfferPayload]{
		Type:     common.Offer,
		Payload:  common.OfferPayload{SDP: offer},
		TargetID: cfg.HostID,
		SenderID: cfg.ID,
	}
	if err := wsc.WriteJSON(offerMsg); err != nil {
		return nil, nil, fmt.Errorf("failed to send offer: %v", err)
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
						// context was cancelled, so this is expected
						return
					}
					continue
				}
				glog.Infof("Accepted local connection from %s", localConn.RemoteAddr())
				// Since we have one data channel, we can only bridge one connection at a time.
				// The bridge function is blocking.
				if err := common.BridgeStream(dc, localConn); err != nil {
					glog.Errorf("Failed to bridge connection: %v", err)
				}
				glog.Info("Bridged connection closed.")
				// After the bridge is closed, we can accept a new connection.
			}
		}()
	})

	return nil
}
