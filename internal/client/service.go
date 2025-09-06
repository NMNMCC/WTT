package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"wtt/internal/typ"

	"github.com/coder/websocket"
	"github.com/pion/webrtc/v4"
)

type ServiceConfig struct {
	ID    string
	Token string

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
	InputMap map[string]*Peer

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

	ServiceInterface
}

type ServiceInterface interface {
	Serve(ctx context.Context)

	Register(ctx context.Context) error
	AnswerOK(ctx context.Context, id string, sdp webrtc.SessionDescription) error
	AnswerNO(ctx context.Context, id, reason string) error
}

func NewService(cfg ServiceConfig) (*Service, error) {
	return &Service{
		ServiceConfig: &cfg,
		ServiceState: &ServiceState{
			Status:   ServiceStatusPending,
			InputMap: make(map[string]*Peer),
		},
		ErrorChannel: make(chan error),
	}, nil
}

func (s *Service) Register(ctx context.Context) error {
	header := http.Header{}
	header.Set("ID", s.ID)
	header.Set("Type", string(typ.PeerTypeService))

	conn, resp, err := websocket.Dial(ctx, s.ServiceConfig.Endpoint, &websocket.DialOptions{
		HTTPHeader: header,
	})
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to connect to server"))
	}
	if resp.StatusCode != 101 {
		return fmt.Errorf("failed to connect to server: %s", resp.Body)
	}

	s.ServerConn = conn
	s.Status = ServiceStatusActive

	return nil
}

func (s *Service) Serve(ctx context.Context) {
	for {
		type_, msg, err := s.ServerConn.Read(ctx)
		if err != nil {
			s.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to receive service message"))
			continue
		}
		if type_ != websocket.MessageBinary {
			s.ErrorChannel <- fmt.Errorf("invalid service message type")
			continue
		}

		var evl typ.Envelope
		if err := json.Unmarshal(msg, &evl); err != nil {
			s.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to unmarshal service message"))
			continue
		}

		switch evl.Type {
		case typ.EnvelopeTypeRTCOffer:
			var offer typ.RTCOffer
			if err := json.Unmarshal(evl.Payload, &offer); err != nil {
				s.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to unmarshal RTC offer"))
				continue
			}

			if s.HandleOffer == nil {
				s.ErrorChannel <- fmt.Errorf("no OnOffer handler registered")
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
				s.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to unmarshal ICE candidate"))
				continue
			}

			if err := s.InputMap[evl.ConsumerID].Conn.AddICECandidate(can.Candidate.ToJSON()); err != nil {
				s.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to add ICE candidate"))
				continue
			}
		default:
			s.ErrorChannel <- fmt.Errorf("unknown service message type: %s", evl.Type)
			continue
		}
	}
}

func (s *Service) AnswerOK(ctx context.Context, id string, sdp webrtc.SessionDescription) error {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to create peer connection"))
	}

	pc.SetRemoteDescription(sdp)

	ans, err := pc.CreateAnswer(nil)
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to create answer"))
	}

	pc.SetLocalDescription(ans)

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnOpen(func() {
			s.InputMap[id] = &Peer{
				Conn:        pc,
				DataChannel: dc,
			}
			s.Status = ServiceStatusActive

			bridge(ctx, s.Type, s.Output, dc)
		})

		dc.OnClose(func() {
			delete(s.InputMap, id)
		})
	})

	payload, err := json.Marshal(typ.RTCAnswerOK{
		SessionDescription: ans,
	})
	if err != nil {
		s.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to marshal RTC answer OK"))
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
		s.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to marshal RTC answer OK"))
		return err
	}

	if err := s.ServerConn.Write(ctx, websocket.MessageText, evl); err != nil {
		s.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to send RTC answer OK"))
		return err
	}

	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		payload, err := json.Marshal(typ.ICECandidate{
			Candidate: *i,
		})
		if err != nil {
			s.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to marshal ICE candidate"))
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
			s.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to marshal ICE candidate envelope"))
			return
		}

		if err := s.ServerConn.Write(ctx, websocket.MessageText, evl); err != nil {
			s.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to send ICE candidate"))
			return
		}
	})

	return nil
}

func (s *Service) AnswerNO(ctx context.Context, id, reason string) error {
	body, err := json.Marshal(typ.RTCAnswerNO{
		Reason: reason,
	})
	if err != nil {
		ne := errors.Join(err, fmt.Errorf("failed to marshal RTC answer"))
		s.ErrorChannel <- ne
		return ne
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
		ne := errors.Join(err, fmt.Errorf("failed to marshal RTC answer"))
		s.ErrorChannel <- ne
		return ne
	}

	if err := s.ServerConn.Write(ctx, websocket.MessageText, evl); err != nil {
		ne := errors.Join(err, fmt.Errorf("failed to send RTC answer"))
		s.ErrorChannel <- ne
		return ne
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
