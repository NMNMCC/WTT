package server

import (
	"context"
	"encoding/json"
	"wtt/internal/typ"

	"github.com/coder/websocket"
)

func (s *Server) HandleRTCOffer(ctx context.Context, peer *Peer, service, consumer *Peer, msg json.RawMessage) {
	s.log.Debug("handling rtc offer")
	service.Conn.Write(ctx, websocket.MessageText, msg)
}

func (s *Server) HandleRTCAnswerOK(ctx context.Context, peer *Peer, service, consumer *Peer, msg json.RawMessage) {
	s.log.Debug("handling rtc answer ok")
	service.Conn.Write(ctx, websocket.MessageText, msg)
}

func (s *Server) HandleRTCAnswerNO(ctx context.Context, peer *Peer, service, consumer *Peer, msg json.RawMessage) {
	s.log.Debug("handling rtc answer no")
	service.Conn.Write(ctx, websocket.MessageText, msg)
}

func (s *Server) HandleICECandidate(ctx context.Context, peer *Peer, service, consumer *Peer, msg json.RawMessage) {
	s.log.Debug("handling ice candidate")
	switch peer.Type {
	case typ.PeerTypeService:
		service.Conn.Write(ctx, websocket.MessageText, msg)
	case typ.PeerTypeConsumer:
		consumer.Conn.Write(ctx, websocket.MessageText, msg)
	}
}
