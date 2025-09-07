package server

import (
	"context"
	"encoding/json"
	"log/slog"
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
	Server  *http.Server
	PeerMap map[string]*Peer
}

type Server struct {
	*ServerConfig
	*ServerState

	ErrorChannel chan error
	log          *slog.Logger

	ServerInterface
}

type ServerInterface interface {
	Start(ctx context.Context)
	Shutdown()
}

func NewServer(cfg ServerConfig) (*Server, error) {
	s := &Server{
		ServerConfig: &cfg,
		ServerState: &ServerState{
			PeerMap: make(map[string]*Peer),
		},
		ErrorChannel: make(chan error),
		log: slog.With(
			slog.String("component", "server"),
		),
	}
	s.log.Info("server created")
	return s, nil
}

func (s *Server) Start(ctx context.Context) error {
	s.Server = &http.Server{
		Addr: s.Endpoint,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log := s.log
			id := r.Header.Get("ID")
			if id == "" || s.PeerMap[id] != nil {
				log.Error("invalid or duplicate ID header", "id", id)
				return
			}
			log = log.With(slog.String("peer_id", id))

			type_ := r.Header.Get("Type")
			if type_ == "" {
				log.Error("missing Type header")
				return
			} else if !slices.Contains([]typ.PeerType{typ.PeerTypeService, typ.PeerTypeConsumer}, typ.PeerType(type_)) {
				log.Error("invalid Type header", "type", type_)
				return
			}
			log = log.With(slog.String("peer_type", type_))

			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				log.Error("failed to accept websocket connection", "err", err)
				return
			}

			peer := &Peer{
				Type: typ.PeerType(type_),
				Conn: conn,
			}

			s.PeerMap[id] = peer
			log.Info("peer connected")

			defer func() {
				delete(s.PeerMap, id)
				conn.Close(websocket.StatusNormalClosure, "")
				log.Info("peer disconnected")
			}()

			for {
				type_, msg, err := conn.Read(ctx)
				if err != nil {
					log.Error("failed to read websocket message", "err", err)
					break
				}
				if type_ != websocket.MessageText {
					continue
				}

				s.Router(ctx, peer, msg)
			}
		}),
	}

	go func() {
		if err := s.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.ErrorChannel <- err
		}
	}()

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.log.Info("shutting down server")
	if s.Server != nil {
		return s.Server.Shutdown(ctx)
	}
	s.log.Info("server shut down")
	return nil
}

func (s *Server) Router(ctx context.Context, peer *Peer, msg json.RawMessage) {
	var evl typ.Envelope
	if err := json.Unmarshal(msg, &evl); err != nil {
		s.log.Error("failed to unmarshal websocket message", "err", err)
		return
	}
	log := s.log.With(
		slog.String("service_id", evl.ServiceID),
		slog.String("consumer_id", evl.ConsumerID),
		slog.String("type", string(evl.Type)),
	)
	log.Debug("routing message")
	service := s.PeerMap[evl.ServiceID]
	if service == nil || service.Type != typ.PeerTypeService {
		log.Warn("service not registered")
		peer.Conn.Write(ctx, websocket.MessageText, []byte("service not registered"))
		return
	}
	consumer := s.PeerMap[evl.ConsumerID]
	if consumer == nil || consumer.Type != typ.PeerTypeConsumer {
		log.Warn("consumer not registered")
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
		log.Error("unknown envelope type")
	}
}
