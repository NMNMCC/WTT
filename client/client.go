package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"time"
	"wtt/common"

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
		slog.Info("No client ID provided, generated one", "id", cfg.ID)
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

	slog.Info("Client connected to host", "client_id", cfg.ID, "host_id", cfg.HostID)

	// Wait for context cancellation
	<-ctx.Done()
	slog.Info("Client shutting down...")
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
		slog.Info("PeerConnection state changed", "state", state.String())
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
		msg := common.TypedMessage[common.CandidatePayload]{
			Type:     common.Candidate,
			Payload:  payload,
			TargetID: cfg.HostID,
			SenderID: cfg.ID,
		}
		if err := wsc.WriteJSON(msg); err != nil {
			slog.Error("Failed to send ICE candidate", "error", err)
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

	offerMsg := common.TypedMessage[common.OfferPayload]{
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
				slog.Warn("Error reading from websocket", "error", err)
				pc.Close()
				return
			}
			var msg common.Message
			if err := json.Unmarshal(p, &msg); err != nil {
				slog.Error("Failed to unmarshal message", "error", err)
				continue
			}

			switch msg.Type {
			case common.Answer:
				var payload common.AnswerPayload
				if err := json.Unmarshal(msg.Payload, &payload); err != nil {
					slog.Error("Failed to unmarshal answer payload", "error", err)
					continue
				}
				if err := pc.SetRemoteDescription(payload.SDP); err != nil {
					slog.Error("Failed to set remote description", "error", err)
				}
			case common.Candidate:
				var payload common.CandidatePayload
				if err := json.Unmarshal(msg.Payload, &payload); err != nil {
					slog.Error("Failed to unmarshal candidate payload", "error", err)
					continue
				}
				if err := pc.AddICECandidate(payload.Candidate); err != nil {
					slog.Error("Failed to add ICE candidate", "error", err)
				}
			}
		}
	}()

	timeout, cancel := context.WithTimeout(ctx, time.Duration(cfg.Timeout)*time.Second)
	defer cancel()

	select {
	case <-connected:
		if pc.ConnectionState() == webrtc.PeerConnectionStateConnected {
			slog.Info("PeerConnection established")
			return pc, dc, nil
		}
		return nil, nil, fmt.Errorf("peer connection failed to connect, state is %s", pc.ConnectionState().String())
	case <-timeout.Done():
		pc.Close()
		return nil, nil, fmt.Errorf("connection timed out after %d seconds", cfg.Timeout)
	}
}

func tunnel(ctx context.Context, pc *webrtc.PeerConnection, dc *webrtc.DataChannel, protocol common.Protocol, localAddr string) error {
	switch protocol {
	case common.TCP:
		listener, err := net.Listen("tcp", localAddr)
		if err != nil {
			return fmt.Errorf("failed to listen on local address %s: %w", localAddr, err)
		}
		slog.Info("Client listening", "local_addr", localAddr, "protocol", "tcp")

		go func() {
			<-ctx.Done()
			listener.Close()
		}()

		dc.OnOpen(func() {
			slog.Info("DataChannel opened, waiting for local connections", "label", dc.Label())
			go func() {
				for {
					localConn, err := listener.Accept()
					if err != nil {
						slog.Warn("Failed to accept local connection", "error", err)
						if ctx.Err() != nil {
							return
						}
						continue
					}
					slog.Info("Accepted local connection", "remote_addr", localConn.RemoteAddr())
					if err := common.BridgeStream(dc, localConn); err != nil {
						slog.Error("Failed to bridge connection", "error", err)
					}
					slog.Info("Bridged connection closed.")
				}
			}()
		})
	case common.UDP:
		pconn, err := net.ListenPacket("udp", localAddr)
		if err != nil {
			return fmt.Errorf("failed to listen on local address %s: %w", localAddr, err)
		}
		slog.Info("Client listening", "local_addr", localAddr, "protocol", "udp")

		go func() {
			<-ctx.Done()
			pconn.Close()
		}()

		dc.OnOpen(func() {
			slog.Info("DataChannel opened, bridging UDP packets", "label", dc.Label())
			if err := common.BridgePacket(dc, pconn); err != nil {
				slog.Error("Failed to bridge packet connection", "error", err)
			}
		})
	}
	return nil
}
