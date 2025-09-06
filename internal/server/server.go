package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"wtt/internal/typ"

	"github.com/coder/websocket"
)

type ServerConfig struct {
	Endpoint string
}

type Peer struct {
	Type typ.PeerType
	Conn *websocket.Conn
}

type ServerState struct {
	PeerConnectionMap map[string]*Peer
}

type Server struct {
	*ServerConfig
	*ServerState

	ErrorChannel chan error

	ServerInterface
}

type ServerInterface interface {
	Start(ctx context.Context)
	Shutdown()
}

func NewServer(cfg ServerConfig) (*Server, error) {
	return &Server{
		ServerConfig: &cfg,
		ServerState: &ServerState{
			PeerConnectionMap: make(map[string]*Peer),
		},
		ErrorChannel: make(chan error),
	}, nil
}

func (s *Server) Start(ctx context.Context) error {
	http.ListenAndServe(s.Endpoint, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("ID")
		if id == "" || s.PeerConnectionMap[id] != nil {
			ne := errors.New("invalid or duplicate ID header")
			s.ErrorChannel <- ne
			return
		}

		type_ := r.Header.Get("Type")
		if type_ == "" {
			ne := errors.New("missing Type header")
			s.ErrorChannel <- ne
			return
		} else if !slices.Contains([]typ.PeerType{typ.PeerTypeService, typ.PeerTypeConsumer}, typ.PeerType(type_)) {
			ne := errors.New("invalid Type header")
			s.ErrorChannel <- ne
			return
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			ne := errors.Join(err, errors.New("failed to accept websocket connection"))
			s.ErrorChannel <- ne
			return
		}

		peer := &Peer{
			Type: typ.PeerType(type_),
			Conn: conn,
		}

		s.PeerConnectionMap[id] = peer

		for {
			type_, msg, err := conn.Read(ctx)
			if err != nil {
				ne := errors.Join(err, errors.New("failed to read websocket message"))
				s.ErrorChannel <- ne
				continue
			}
			if type_ != websocket.MessageText {
				continue
			}

			s.Router(ctx, peer, msg)
		}
	}))

	return nil
}

func (s *Server) Router(ctx context.Context, peer *Peer, msg json.RawMessage) {
	var evl typ.Envelope
	if err := json.Unmarshal(msg, &evl); err != nil {
		s.ErrorChannel <- errors.Join(err, errors.New("failed to unmarshal websocket message"))
		return
	}
	service := s.PeerConnectionMap[evl.ServiceID]
	if service == nil || service.Type != typ.PeerTypeService {
		peer.Conn.Write(ctx, websocket.MessageText, []byte("service not registered"))
		return
	}
	consumer := s.PeerConnectionMap[evl.ConsumerID]
	if consumer == nil || consumer.Type != typ.PeerTypeConsumer {
		peer.Conn.Write(ctx, websocket.MessageText, []byte("consumer not registered"))
		return
	}

	switch evl.Type {
	case typ.EnvelopeTypeRTCOffer:
		s.HandleRTCOffer(ctx, peer, service, consumer, msg)
	case typ.EnvelopeTypeRTCAnswerOK:
		s.HandleRTCAnswerOK(ctx, peer, service, consumer, msg)
	case typ.EnvelopeTypeRTCAnswerNO:
		s.HandleRTCAnswerNO(ctx, peer, service, consumer, msg)
	case typ.EnvelopeTypeICECandidate:
		s.HandleICECandidate(ctx, peer, service, consumer, msg)
	default:
		s.ErrorChannel <- errors.New("unknown envelope type")
	}
}
