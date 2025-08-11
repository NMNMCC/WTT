package host

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"wtt/common"

	"github.com/golang/glog"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

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
		glog.Infof("No host ID provided, generated one: %s", config.ID)
	}

	wsc, err := common.WebSocketConn(config.SigAddr, config.Token)
	if err != nil {
		return fmt.Errorf("failed to connect to signaling server: %v", err)
	}
	defer wsc.Close()

	// Register with the signaling server
	regMsg := common.Message[any]{Type: common.Register, SenderID: config.ID}
	if err := wsc.WriteJSON(regMsg); err != nil {
		return fmt.Errorf("failed to register host: %v", err)
	}
	glog.Infof("Host '%s' registered with signaling server.", config.ID)

	// Wait for offers and process them
	return wait(ctx, wsc, config)
}

func wait(ctx context.Context, wsc *websocket.Conn, cfg HostConfig) error {
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
			glog.Info("Host shutting down...")
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
			var msg common.Message[any]
			if err := json.Unmarshal(p, &msg); err != nil {
				glog.Errorf("Failed to unmarshal message: %v", err)
				continue
			}

			switch msg.Type {
			case common.Offer:
				wg.Add(1)
				go func() {
					defer wg.Done()
					handleOffer(wsc, msg, cfg, peerConnections)
				}()
			case common.Candidate:
				wg.Add(1)
				go func() {
					defer wg.Done()
					handleCandidate(msg, peerConnections)
				}()
			default:
				glog.Warningf("Unknown message type received: %v", msg.Type)
			}
		}
	}
}

func handleOffer(wsc *websocket.Conn, msg common.Message[any], cfg HostConfig, peerConnections *sync.Map) {
	offer, err := common.ReUnmarshal[common.Message[common.OfferPayload]](msg)
	if err != nil {
		glog.Errorf("Failed to unmarshal offer message: %v", err)
		return
	}

	clientID := offer.SenderID
	glog.Infof("Received offer from client '%s'", clientID)
	pc, err := answer(wsc, offer, cfg)
	if err != nil {
		glog.Errorf("Failed to create answer for '%s': %v", clientID, err)
		if pc != nil {
			pc.Close()
		}
		return
	}

	peerConnections.Store(clientID, pc)

	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		glog.Infof("PeerConnection state with '%s' changed: %s", clientID, s.String())
		if s == webrtc.PeerConnectionStateClosed || s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateDisconnected {
			peerConnections.Delete(clientID)
		}
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		glog.Infof("DataChannel '%s' from '%s' opened", dc.Label(), clientID)
		dc.OnOpen(func() {
			bridgeDataChannel(dc, cfg, clientID)
		})
	})
}

func handleCandidate(msg common.Message[any], peerConnections *sync.Map) {
	candidateMsg, err := common.ReUnmarshal[common.Message[common.CandidatePayload]](msg)
	if err != nil {
		glog.Errorf("Failed to unmarshal candidate message: %v", err)
		return
	}

	clientID := candidateMsg.SenderID
	val, ok := peerConnections.Load(clientID)
	if !ok {
		glog.Warningf("Received candidate for unknown client: %s", clientID)
		return
	}
	pc, ok := val.(*webrtc.PeerConnection)
	if !ok {
		glog.Errorf("Invalid type in peerConnections map for client: %s", clientID)
		return
	}

	if err := pc.AddICECandidate(candidateMsg.Payload.Candidate); err != nil {
		glog.Errorf("Failed to add ICE candidate from '%s': %v", clientID, err)
	}
}

func bridgeDataChannel(dc *webrtc.DataChannel, cfg HostConfig, clientID string) {
	glog.Infof("DataChannel '%s' from '%s' ready, bridging to local service.", dc.Label(), clientID)
	var localConn net.Conn
	var err error
	switch cfg.Protocol {
	case common.TCP:
		localConn, err = net.Dial("tcp", cfg.LocalAddr)
	case common.UDP:
		localConn, err = net.Dial("udp", cfg.LocalAddr)
	default:
		err = fmt.Errorf("unsupported protocol: %s", cfg.Protocol)
	}

	if err != nil {
		glog.Errorf("Failed to connect to local service at %s for client '%s': %v", cfg.LocalAddr, clientID, err)
		dc.Close()
		return
	}

	glog.Infof("Successfully connected to local service at %s for client '%s'", cfg.LocalAddr, clientID)
	if err := common.BridgeStream(dc, localConn); err != nil {
		glog.Errorf("Failed to bridge DataChannel for '%s': %v", clientID, err)
	}
}

func answer(wsc *websocket.Conn, offer common.Message[common.OfferPayload], cfg HostConfig) (*webrtc.PeerConnection, error) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: cfg.STUNAddrs}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create PeerConnection: %v", err)
	}

	clientID := offer.SenderID
	hostID := cfg.ID

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		payload := common.CandidatePayload{Candidate: c.ToJSON()}
		msg := common.Message[common.CandidatePayload]{
			Type:     common.Candidate,
			Payload:  payload,
			TargetID: clientID,
			SenderID: hostID,
		}
		if err := wsc.WriteJSON(msg); err != nil {
			glog.Errorf("Failed to send ICE candidate to '%s': %v", clientID, err)
		}
	})

	if err := pc.SetRemoteDescription(offer.Payload.SDP); err != nil {
		return pc, fmt.Errorf("failed to set remote description: %w", err)
	}

	ans, err := pc.CreateAnswer(nil)
	if err != nil {
		return pc, fmt.Errorf("failed to create answer: %w", err)
	}

	if err := pc.SetLocalDescription(ans); err != nil {
		return pc, fmt.Errorf("failed to set local description: %w", err)
	}

	answerMsg := common.Message[common.AnswerPayload]{
		Type:     common.Answer,
		Payload:  common.AnswerPayload{SDP: ans},
		TargetID: clientID,
		SenderID: hostID,
	}
	if err := wsc.WriteJSON(answerMsg); err != nil {
		return pc, fmt.Errorf("failed to send answer to '%s': %w", clientID, err)
	}

	glog.Infof("Sent answer to client '%s'", clientID)
	return pc, nil
}
