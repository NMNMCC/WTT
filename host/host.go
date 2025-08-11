package host

import (
	"fmt"

	"wtt/common"

	"github.com/golang/glog"
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

func Run(config HostConfig) error {
	wsc, err := common.WebSocketConn(config.SigAddr, config.Token)
	if err != nil {
		glog.Fatalf("failed to connect to signaling server: %v", err)
		return err
	}

	reg := common.Message[any]{Type: common.Register, SenderID: config.ID}
	err = wsc.WriteJSON(reg)
	if err != nil {
		return fmt.Errorf("failed to register host: %v", err)
	}
	glog.Infof("host '%s' registered to signaling server.", config.ID)

	pcs := wait(wsc, config.STUNAddrs)
	for pc := range pcs {
		pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
			glog.Infof("PeerConnection state: %s", s.String())
		})

		pc.OnDataChannel(func(dc *webrtc.DataChannel) {
			glog.Infof("DataChannel '%s' opened (ID=%d)", dc.Label(), *dc.ID())

			dc.OnOpen(func() {
				err := common.Bridge(config.Protocol, dc)
				if err != nil {
					glog.Errorf("failed to bridge DataChannel '%s': %v", dc.Label(), err)
					return
				}
				glog.Infof("DataChannel '%s' bridged successfully", dc.Label())
			})

			dc.OnClose(func() { glog.Infof("DataChannel '%s' closed", dc.Label()) })
		})
	}

	return nil
}

// wait for incoming messages from the signaling server
// and create PeerConnections for each client that connects
func wait(wsc *websocket.Conn, st []string) <-chan *webrtc.PeerConnection {
	connM := map[string]*webrtc.PeerConnection{}
	ch := make(chan *webrtc.PeerConnection, 1)

	go func() {
		defer close(ch)
		for {
			var msg common.Message[any]
			if err := wsc.ReadJSON(&msg); err != nil {
				glog.Errorf("Failed to read message from WebSocket: %v", err)
				return
			}

			switch msg.Type {
			case common.Offer:
				offer, err := common.ReUnmarshal[common.Message[common.OfferPayload]](msg)
				if err != nil {
					glog.Errorf("Failed to unmarshal offer message: %v", err)
					continue
				}

				pc, err := answer(wsc, offer, st)
				if err != nil {
					glog.Errorf("Failed to create PeerConnection: %v", err)
					if pc != nil {
						_ = pc.Close()
					}
					continue
				}

				sid := msg.SenderID
				pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
					if s == webrtc.PeerConnectionStateClosed || s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateDisconnected {
						delete(connM, sid)
					}
				})

				connM[sid] = pc
				ch <- pc
			case common.Candidate:
				pc, ok := connM[msg.SenderID]
				if !ok {
					glog.Warningf("unknown sender ID for candidate: %v", msg.SenderID)
					continue
				}

				cand, err := common.ReUnmarshal[common.Message[common.CandidatePayload]](msg)
				if err != nil {
					glog.Errorf("Failed to unmarshal candidate message: %v", err)
					continue
				}

				err = addICECandidate(pc, cand)
				if err != nil {
					glog.Errorf("failed to add ICE candidate: %v", err)
				}
			default:
				glog.Warningf("unknown message type: %v", msg.Type)
			}
		}
	}()
	return ch
}

// create an answer for the offer received from the client
func answer(wsc *websocket.Conn, offer common.Message[common.OfferPayload], st []string) (*webrtc.PeerConnection, error) {
	var (
		targetID = offer.SenderID
		senderID = offer.TargetID
	)

	pc, err := createPC(st)
	if err != nil {
		return nil, err
	}

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}

		_ = wsc.WriteJSON(common.Message[common.CandidatePayload]{
			Type:     common.Candidate,
			Payload:  common.CandidatePayload{Candidate: c.ToJSON()},
			TargetID: senderID, // send back to client who sent the offer
			SenderID: targetID, // this is the host ID
		})
	})

	if err := pc.SetRemoteDescription(offer.Payload.SDP); err != nil {
		return pc, err
	}
	if err := ensureLocal(pc); err != nil {
		return pc, err
	}
	ld := pc.LocalDescription()
	if ld == nil {
		return pc, fmt.Errorf("local description missing")
	}
	if err := wsc.WriteJSON(common.Message[common.AnswerPayload]{Type: common.Answer, Payload: common.AnswerPayload{SDP: *ld}, TargetID: targetID}); err != nil {
		return pc, err
	}
	return pc, nil
}

func ensureLocal(pc *webrtc.PeerConnection) error {
	ans, err := pc.CreateAnswer(nil)
	if err != nil {
		return err
	}
	return pc.SetLocalDescription(ans)
}

func createPC(st []string) (*webrtc.PeerConnection, error) {
	cfg := webrtc.Configuration{}
	if len(st) > 0 {
		cfg.ICEServers = []webrtc.ICEServer{{URLs: st}}
	} else {
		glog.Warningf("no STUN servers configured.")
	}
	pc, err := webrtc.NewPeerConnection(cfg)
	if err != nil {
		glog.Errorf("failed to create PeerConnection: %v", err)
		return nil, err
	}
	return pc, nil
}

func addICECandidate(pc *webrtc.PeerConnection, msg common.Message[common.CandidatePayload]) error {
	if msg.Type != common.Candidate {
		return fmt.Errorf("expected candidate message, got %s", msg.Type)
	}
	return pc.AddICECandidate(msg.Payload.Candidate)
}
