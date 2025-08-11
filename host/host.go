package host

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"wtt/common"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

// threadSafeWriter protects the websocket connection from concurrent writes.
type threadSafeWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (t *threadSafeWriter) WriteJSON(v interface{}) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.conn.WriteJSON(v)
}

func newThreadSafeWriter(conn *websocket.Conn) *threadSafeWriter {
	return &threadSafeWriter{conn: conn}
}

type HostConfig struct {
	ID        string
	SigAddr   string
	LocalAddr string
	Protocol  common.Protocol
	STUNAddrs []string
	Token     string
}

func Run(ctx context.Context, config HostConfig) error {
	if config.ID == "" {
		config.ID = uuid.NewString()
		slog.Info("No host ID provided, generated one", "id", config.ID)
	}

	wsc, err := common.WebSocketConn(config.SigAddr, config.Token)
	if err != nil {
		return fmt.Errorf("failed to connect to signaling server: %v", err)
	}
	defer wsc.Close()

	safeWSC := newThreadSafeWriter(wsc)

	// Register with the signaling server
	regMsg := common.TypedMessage[any]{Type: common.Register, SenderID: config.ID}
	if err := safeWSC.WriteJSON(regMsg); err != nil {
		return fmt.Errorf("failed to register host: %v", err)
	}
	slog.Info("Host registered with signaling server", "id", config.ID)

	// Wait for offers and process them
	return wait(ctx, wsc, safeWSC, config)
}

func wait(ctx context.Context, wsc *websocket.Conn, safeWSC *threadSafeWriter, cfg HostConfig) error {
	var wg sync.WaitGroup
	peerConnections := &sync.Map{} // string -> *webrtc.PeerConnection

	msgChan := make(chan []byte)
	errChan := make(chan error, 1)

	// Read messages from websocket
	go func() {
		defer close(msgChan)
		for {
			_, p, err := wsc.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			msgChan <- p
		}
	}()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Host shutting down...")
			wg.Wait() // Wait for handlers to finish
			peerConnections.Range(func(key, value interface{}) bool {
				if pc, ok := value.(*webrtc.PeerConnection); ok {
					pc.Close()
				}
				return true
			})
			return nil
		case err := <-errChan:
			return fmt.Errorf("error reading from websocket: %w", err)
		case p := <-msgChan:
			if p == nil {
				return nil // Channel closed
			}
			var msg common.Message
			if err := json.Unmarshal(p, &msg); err != nil {
				slog.Error("Failed to unmarshal message", "error", err)
				continue
			}

			switch msg.Type {
			case common.Offer:
				wg.Add(1)
				go func() {
					defer wg.Done()
					handleOffer(safeWSC, msg, cfg, peerConnections)
				}()
			case common.Candidate:
				wg.Add(1)
				go func() {
					defer wg.Done()
					handleCandidate(msg, peerConnections)
				}()
			default:
				slog.Warn("Unknown message type received", "type", msg.Type)
			}
		}
	}
}

func handleOffer(wsc *threadSafeWriter, msg common.Message, cfg HostConfig, peerConnections *sync.Map) {
	var offerPayload common.OfferPayload
	if err := json.Unmarshal(msg.Payload, &offerPayload); err != nil {
		slog.Error("Failed to unmarshal offer payload", "error", err)
		return
	}

	clientID := msg.SenderID
	slog.Info("Received offer from client", "client_id", clientID)
	pc, err := answer(wsc, clientID, offerPayload.SDP, cfg)
	if err != nil {
		slog.Error("Failed to create answer", "client_id", clientID, "error", err)
		if pc != nil {
			pc.Close()
		}
		return
	}

	peerConnections.Store(clientID, pc)

	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		slog.Info("PeerConnection state with client changed", "client_id", clientID, "state", s.String())
		if s == webrtc.PeerConnectionStateClosed || s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateDisconnected {
			peerConnections.Delete(clientID)
		}
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		slog.Info("DataChannel from client opened", "label", dc.Label(), "client_id", clientID)
		dc.OnOpen(func() {
			bridgeDataChannel(dc, cfg, clientID)
		})
	})
}

func handleCandidate(msg common.Message, peerConnections *sync.Map) {
	var candidatePayload common.CandidatePayload
	if err := json.Unmarshal(msg.Payload, &candidatePayload); err != nil {
		slog.Error("Failed to unmarshal candidate payload", "error", err)
		return
	}

	clientID := msg.SenderID
	val, ok := peerConnections.Load(clientID)
	if !ok {
		slog.Warn("Received candidate for unknown client", "client_id", clientID)
		return
	}
	pc, ok := val.(*webrtc.PeerConnection)
	if !ok {
		slog.Error("Invalid type in peerConnections map for client", "client_id", clientID)
		return
	}

	if err := pc.AddICECandidate(candidatePayload.Candidate); err != nil {
		slog.Error("Failed to add ICE candidate from client", "client_id", clientID, "error", err)
	}
}

func bridgeDataChannel(dc *webrtc.DataChannel, cfg HostConfig, clientID string) {
	slog.Info("DataChannel ready, bridging to local service.", "label", dc.Label(), "client_id", clientID)

	switch cfg.Protocol {
	case common.TCP:
		localConn, err := net.Dial("tcp", cfg.LocalAddr)
		if err != nil {
			slog.Error("Failed to connect to local service", "local_addr", cfg.LocalAddr, "client_id", clientID, "error", err)
			dc.Close()
			return
		}
		slog.Info("Successfully connected to local service", "local_addr", cfg.LocalAddr, "client_id", clientID)
		if err := common.BridgeStream(dc, localConn); err != nil {
			slog.Error("Failed to bridge DataChannel", "client_id", clientID, "error", err)
		}
	case common.UDP:
		conn, err := net.Dial("udp", cfg.LocalAddr)
		if err != nil {
			slog.Error("Failed to connect to local service", "local_addr", cfg.LocalAddr, "client_id", clientID, "error", err)
			dc.Close()
			return
		}
		slog.Info("Successfully connected to local service", "local_addr", cfg.LocalAddr, "client_id", clientID)
		bridgeDialedUDP(dc, conn)
	default:
		slog.Error("Unsupported protocol", "protocol", cfg.Protocol)
		dc.Close()
	}
}

// bridgeDialedUDP handles bridging for a dialed UDP connection, which behaves like a stream.
func bridgeDialedUDP(dc *webrtc.DataChannel, localConn net.Conn) {
	slog.Info("Bridging dialed UDP connection")

	// Remote (DC) -> Local (echo server)
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if _, err := localConn.Write(msg.Data); err != nil {
			slog.Warn("Failed to write to local UDP conn", "error", err)
		}
	})

	// Local (echo server) -> Remote (DC)
	go func() {
		buf := make([]byte, 16384)
		for {
			n, err := localConn.Read(buf)
			if err != nil {
				slog.Warn("Failed to read from local UDP conn", "error", err)
				dc.Close()
				localConn.Close()
				return
			}
			if err := dc.Send(buf[:n]); err != nil {
				slog.Warn("Failed to send to DC", "error", err)
				dc.Close()
				localConn.Close()
				return
			}
		}
	}()
}

func answer(wsc *threadSafeWriter, clientID string, remoteSDP webrtc.SessionDescription, cfg HostConfig) (*webrtc.PeerConnection, error) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: cfg.STUNAddrs}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create PeerConnection: %v", err)
	}

	hostID := cfg.ID

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		payload := common.CandidatePayload{Candidate: c.ToJSON()}
		msg := common.TypedMessage[common.CandidatePayload]{
			Type:     common.Candidate,
			Payload:  payload,
			TargetID: clientID,
			SenderID: hostID,
		}
		if err := wsc.WriteJSON(msg); err != nil {
			slog.Error("Failed to send ICE candidate to client", "client_id", clientID, "error", err)
		}
	})

	if err := pc.SetRemoteDescription(remoteSDP); err != nil {
		return pc, fmt.Errorf("failed to set remote description: %w", err)
	}

	ans, err := pc.CreateAnswer(nil)
	if err != nil {
		return pc, fmt.Errorf("failed to create answer: %w", err)
	}

	if err := pc.SetLocalDescription(ans); err != nil {
		return pc, fmt.Errorf("failed to set local description: %w", err)
	}

	answerMsg := common.TypedMessage[common.AnswerPayload]{
		Type:     common.Answer,
		Payload:  common.AnswerPayload{SDP: ans},
		TargetID: clientID,
		SenderID: hostID,
	}
	if err := wsc.WriteJSON(answerMsg); err != nil {
		return pc, fmt.Errorf("failed to send answer to '%s': %w", clientID, err)
	}

	slog.Info("Sent answer to client", "client_id", clientID)
	return pc, nil
}
