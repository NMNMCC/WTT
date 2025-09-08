package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"wtt/internal/typ"

	"github.com/coder/websocket"
	"github.com/cornelk/hashmap"
	"github.com/pion/webrtc/v4"
)

type ServiceConfig struct {
	ID string

	Type   string
	Output string

	Endpoint string
}

type Peer struct {
	Conn        *webrtc.PeerConnection
	DataChannel *webrtc.DataChannel
}

type ServiceState struct {
	Status   ServiceStatus
	InputMap *hashmap.Map[string, *Peer]

	ServerConn *websocket.Conn
}

type ServiceStatus string

const (
	ServiceStatusPending ServiceStatus = "pending"
	ServiceStatusActive  ServiceStatus = "active"
	ServiceStatusFailed  ServiceStatus = "failed"
)

type Service struct {
	*ServiceConfig
	*ServiceState

	// OK "" | NO <reason>
	HandleOffer func(offer *typ.RTCOffer) (reason string)

	ErrorChannel chan error
	log          *slog.Logger

	ServiceInterface
}

type ServiceInterface interface {
	Serve(ctx context.Context)
	Shutdown(ctx context.Context) error

	Register(ctx context.Context) error
	AnswerOK(ctx context.Context, id string, sdp webrtc.SessionDescription) error
	AnswerNO(ctx context.Context, id, reason string) error
}

func NewService(cfg ServiceConfig) (*Service, error) {
	s := &Service{
		ServiceConfig: &cfg,
		ServiceState: &ServiceState{
			Status:   ServiceStatusPending,
			InputMap: hashmap.New[string, *Peer](),
		},
		HandleOffer: func(offer *typ.RTCOffer) (reason string) {
			return ""
		},
		ErrorChannel: make(chan error),
		log: slog.With(
			slog.String("component", "service"),
			slog.String("id", cfg.ID),
		),
	}
	s.log.Info("service created")
	return s, nil
}

func (s *Service) Register(ctx context.Context) error {
	header := http.Header{}
	header.Set("ID", s.ID)
	header.Set("Type", string(typ.PeerTypeService))

	s.log.Info("registering service")
	conn, resp, err := websocket.Dial(ctx, s.ServiceConfig.Endpoint, &websocket.DialOptions{
		HTTPHeader: header,
	})
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to connect to server"))
	}
	if resp.StatusCode != 101 {
		return fmt.Errorf("failed to connect to server: %s", resp.Body)
	}
	s.log.Info("service registered")

	s.ServerConn = conn
	s.Status = ServiceStatusActive

	return nil
}

func (s *Service) Serve(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		type_, msg, err := s.ServerConn.Read(ctx)
		if err != nil {
			s.log.Error("failed to receive service message", "err", err)
			continue
		}
		if type_ != websocket.MessageText {
			s.log.Error("invalid service message type")
			continue
		}

		var evl typ.Envelope
		if err := json.Unmarshal(msg, &evl); err != nil {
			s.log.Error("failed to unmarshal service message", "err", err)
			continue
		}
		s.log.Debug("received service message", "type", evl.Type)

		switch evl.Type {
		case typ.EnvelopeTypeRTCOffer:
			var offer typ.RTCOffer
			if err := json.Unmarshal(evl.Payload, &offer); err != nil {
				s.log.Error("failed to unmarshal RTC offer", "err", err)
				continue
			}

			if s.HandleOffer == nil {
				s.log.Error("no OnOffer handler registered")
				continue
			}

			reason := s.HandleOffer(&offer)
			if reason == "" {
				s.AnswerOK(ctx, evl.ID.ConsumerID, offer.SessionDescription)
			} else {
				s.AnswerNO(ctx, evl.ID.ConsumerID, reason)
			}
		case typ.EnvelopeTypeICECandidate:
			var can typ.ICECandidate
			if err := json.Unmarshal(evl.Payload, &can); err != nil {
				s.log.Error("failed to unmarshal ICE candidate", "err", err)
				continue
			}

			consumer, ok := s.InputMap.Get(evl.ConsumerID)
			if !ok {
				s.log.Error("unknown consumer", "id", evl.ConsumerID)
				continue
			}

			if err := consumer.Conn.AddICECandidate(can.Candidate); err != nil {
				s.log.Error("failed to add ICE candidate", "err", err)
				continue
			}
		default:
			s.log.Error("unknown service message type", "type", evl.Type)
			continue
		}
	}
}

func (s *Service) Shutdown(ctx context.Context) error {
	s.log.Info("shutting down service")
	s.InputMap.Range(func(k string, v *Peer) bool {
		if err := v.Conn.Close(); err != nil {
			s.log.Error("failed to close peer connection", "err", err)
			return false
		}
		return true
	})
	if err := s.ServerConn.Close(websocket.StatusNormalClosure, ""); err != nil {
		s.log.Error("failed to close server connection", "err", err)
		return err
	}
	s.log.Info("service shut down")
	return nil
}

func (s *Service) AnswerOK(ctx context.Context, id string, sdp webrtc.SessionDescription) error {
	log := s.log.With(slog.String("consumer_id", id))
	log.Info("answering OK")
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to create peer connection"))
	}

	pc.SetRemoteDescription(sdp)

	s.InputMap.Set(id, &Peer{Conn: pc})

	ans, err := pc.CreateAnswer(nil)
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to create answer"))
	}

	pc.SetLocalDescription(ans)

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		log.Info("data channel opened")
		dc.OnOpen(func() {
			if peer, ok := s.InputMap.Get(id); ok && peer != nil {
				peer.DataChannel = dc
				s.InputMap.Set(id, peer)
			} else {
				s.InputMap.Set(id, &Peer{Conn: pc, DataChannel: dc})
			}
			s.Status = ServiceStatusActive

			bridge(ctx, s.Type, s.Output, dc)
		})

		dc.OnClose(func() {
			log.Info("data channel closed")
			s.InputMap.Del(id)
		})
	})

	payload, err := json.Marshal(typ.RTCAnswerOK{
		SessionDescription: ans,
	})
	if err != nil {
		log.Error("failed to marshal RTC answer OK", "err", err)
		return err
	}
	evl, err := json.Marshal(typ.Envelope{
		ID: typ.ID{
			ServiceID:  s.ID,
			ConsumerID: id,
		},
		Type:    typ.EnvelopeTypeRTCAnswerOK,
		Payload: payload,
	})
	if err != nil {
		log.Error("failed to marshal RTC answer OK", "err", err)
		return err
	}

	if err := s.ServerConn.Write(ctx, websocket.MessageText, evl); err != nil {
		log.Error("failed to send RTC answer OK", "err", err)
		return err
	}

	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}
		payload, err := json.Marshal(typ.ICECandidate{
			Candidate: i.ToJSON(),
		})
		if err != nil {
			log.Error("failed to marshal ICE candidate", "err", err)
			return
		}
		evl, err := json.Marshal(typ.Envelope{
			ID: typ.ID{
				ServiceID:  s.ID,
				ConsumerID: id,
			},
			Type:    typ.EnvelopeTypeICECandidate,
			Payload: payload,
		})
		if err != nil {
			log.Error("failed to marshal ICE candidate envelope", "err", err)
			return
		}

		if err := s.ServerConn.Write(ctx, websocket.MessageText, evl); err != nil {
			log.Error("failed to send ICE candidate", "err", err)
			return
		}
	})

	return nil
}

func (s *Service) AnswerNO(ctx context.Context, id, reason string) error {
	log := s.log.With(slog.String("consumer_id", id))
	log.Info("answering NO", "reason", reason)
	body, err := json.Marshal(typ.RTCAnswerNO{
		Reason: reason,
	})
	if err != nil {
		log.Error("failed to marshal RTC answer NO", "err", err)
		return err
	}
	evl, err := json.Marshal(typ.Envelope{
		ID: typ.ID{
			ServiceID:  s.ID,
			ConsumerID: id,
		},
		Type:    typ.EnvelopeTypeRTCAnswerNO,
		Payload: body,
	})
	if err != nil {
		log.Error("failed to marshal RTC answer NO", "err", err)
		return err
	}

	if err := s.ServerConn.Write(ctx, websocket.MessageText, evl); err != nil {
		log.Error("failed to send RTC answer NO", "err", err)
		return err
	}

	return nil
}

var _ ServiceInterface = (*Service)(nil)

func bridge(ctx context.Context, network, endpoint string, dc *webrtc.DataChannel) error {
	conn, err := net.Dial(network, endpoint)
	if err != nil {
		return err
	}
	defer conn.Close()

	go func() {
		for {
			var b [1024]byte
			n, err := conn.Read(b[:])
			if err != nil {
				return
			}
			dc.Send(b[:n])
		}
	}()

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		conn.Write(msg.Data)
	})

	<-ctx.Done()

	dc.Close()
	conn.Close()

	return nil
}
