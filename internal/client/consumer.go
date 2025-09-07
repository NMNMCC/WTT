package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"wtt/internal/typ"

	"github.com/coder/websocket"
	"github.com/pion/webrtc/v4"
)

type ConsumerConfig struct {
	ID string

	Type   string
	Input  string
	Output string

	Endpoint string
}

type ConsumerState struct {
	Status ServiceStatus

	ServerConn *websocket.Conn
	PeerConn   *webrtc.PeerConnection
	DataCh     *webrtc.DataChannel
}

type Consumer struct {
	*ConsumerConfig
	*ConsumerState

	ErrorChannel chan error
	log          *slog.Logger

	ConsumerInterface
}

type ConsumerInterface interface {
	Serve(ctx context.Context)
	Shutdown(ctx context.Context) error

	Register(ctx context.Context) error
	Connect(ctx context.Context, sid string) error
}

func NewConsumer(cfg ConsumerConfig) (*Consumer, error) {
	c := &Consumer{
		ConsumerConfig: &cfg,
		ConsumerState: &ConsumerState{
			Status: ServiceStatusPending,
		},
		ErrorChannel: make(chan error),
		log: slog.With(
			slog.String("component", "consumer"),
			slog.String("id", cfg.ID),
		),
	}
	c.log.Info("consumer created")
	return c, nil
}

func (c *Consumer) Register(ctx context.Context) error {
	header := http.Header{}
	header.Set("ID", c.ID)
	header.Set("Type", string(typ.PeerTypeConsumer))

	c.log.Info("registering consumer")
	conn, resp, err := websocket.Dial(ctx, c.ConsumerConfig.Endpoint, &websocket.DialOptions{
		HTTPHeader: header,
	})
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to connect to server"))
	}
	if resp.StatusCode != 101 {
		return fmt.Errorf("failed to connect to server: %s", resp.Body)
	}
	c.log.Info("consumer registered")

	c.ServerConn = conn
	c.Status = ServiceStatusActive

	return nil
}

func (c *Consumer) Connect(ctx context.Context, sid string) error {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to create peer connection"))
	}
	c.PeerConn = pc

	dc, err := pc.CreateDataChannel("data", nil)
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to create data channel"))
	}
	c.DataCh = dc

	dc.OnOpen(func() {
		c.log.Info("data channel opened")
		c.Status = ServiceStatusActive
		go listenAndBridge(ctx, c.Type, c.Input, dc, c.log)
	})

	dc.OnClose(func() {
		c.Shutdown(ctx)
	})

	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}
		payload, err := json.Marshal(typ.ICECandidate{
			Candidate: *i,
		})
		if err != nil {
			c.log.Error("failed to marshal ICE candidate", "err", err)
			return
		}
		evl, err := json.Marshal(typ.Envelope{
			ID: typ.ID{
				ServiceID:  sid,
				ConsumerID: c.ID,
			},
			Type:    typ.EnvelopeTypeICECandidate,
			Payload: payload,
		})
		if err != nil {
			c.log.Error("failed to marshal ICE candidate envelope", "err", err)
			return
		}

		if err := c.ServerConn.Write(ctx, websocket.MessageText, evl); err != nil {
			c.log.Error("failed to send ICE candidate", "err", err)
			return
		}
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to create offer"))
	}

	if err := pc.SetLocalDescription(offer); err != nil {
		return errors.Join(err, fmt.Errorf("failed to set local description"))
	}

	payload, err := json.Marshal(typ.RTCOffer{
		SessionDescription: offer,
	})
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to marshal RTC offer"))
	}
	evl, err := json.Marshal(typ.Envelope{
		ID: typ.ID{
			ServiceID:  sid,
			ConsumerID: c.ID,
		},
		Type:    typ.EnvelopeTypeRTCOffer,
		Payload: payload,
	})
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to marshal RTC offer envelope"))
	}

	if err := c.ServerConn.Write(ctx, websocket.MessageText, evl); err != nil {
		return errors.Join(err, fmt.Errorf("failed to send RTC offer"))
	}

	return nil
}

func (c *Consumer) Shutdown(ctx context.Context) error {
	c.log.Info("shutting down consumer")
	if c.DataCh != nil {
		c.DataCh.Close()
		c.DataCh = nil
	}
	if c.PeerConn != nil {
		c.PeerConn.Close()
		c.PeerConn = nil
	}
	if c.ServerConn != nil {
		c.ServerConn.Close(1000, "user requested")
		c.ServerConn = nil
	}
	c.log.Info("consumer shut down")
	return nil
}

func (c *Consumer) Serve(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		type_, msg, err := c.ServerConn.Read(ctx)
		if err != nil {
			c.log.Error("failed to receive service message", "err", err)
			continue
		}
		if type_ != websocket.MessageText {
			c.log.Error("invalid service message type")
			continue
		}

		var evl typ.Envelope
		if err := json.Unmarshal(msg, &evl); err != nil {
			c.log.Error("failed to unmarshal service message", "err", err)
			continue
		}
		c.log.Debug("received service message", "type", evl.Type)

		switch evl.Type {
		case typ.EnvelopeTypeRTCAnswerOK:
			var ans typ.RTCAnswerOK
			if err := json.Unmarshal(evl.Payload, &ans); err != nil {
				c.log.Error("failed to unmarshal RTC answer OK", "err", err)
				continue
			}
			if err := c.PeerConn.SetRemoteDescription(ans.SessionDescription); err != nil {
				c.log.Error("failed to set remote description", "err", err)
				continue
			}
		case typ.EnvelopeTypeRTCAnswerNO:
			var ans typ.RTCAnswerNO
			if err := json.Unmarshal(evl.Payload, &ans); err != nil {
				c.log.Error("failed to unmarshal RTC answer NO", "err", err)
				continue
			}
			c.log.Error("connection rejected by peer", "reason", ans.Reason)
			c.Shutdown(ctx)
		case typ.EnvelopeTypeICECandidate:
			var can typ.ICECandidate
			if err := json.Unmarshal(evl.Payload, &can); err != nil {
				c.log.Error("failed to unmarshal ICE candidate", "err", err)
				continue
			}
			if err := c.PeerConn.AddICECandidate(can.Candidate.ToJSON()); err != nil {
				c.log.Error("failed to add ICE candidate", "err", err)
				continue
			}
		default:
			c.log.Error("unknown service message type", "type", evl.Type)
			continue
		}
	}
}

var _ ConsumerInterface = (*Consumer)(nil)

func listenAndBridge(ctx context.Context, network, address string, dc *webrtc.DataChannel, log *slog.Logger) {
	log = log.With(slog.String("network", network), slog.String("address", address))
	l, err := net.Listen(network, address)
	if err != nil {
		log.Error("failed to listen", "err", err)
		return
	}
	defer l.Close()

	go func() {
		<-ctx.Done()
		l.Close()
	}()

	log.Info("listening")

	for {
		conn, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Info("listener closed")
				break
			}
			log.Error("failed to accept connection", "err", err)
			continue
		}

		go func(c net.Conn) {
			log := log.With(slog.String("remote_addr", c.RemoteAddr().String()))
			log.Info("accepted connection")
			defer c.Close()

			go func() {
				<-ctx.Done()
				c.Close()
			}()

			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				if _, err := c.Write(msg.Data); err != nil {
					log.Error("failed to write to local conn", "err", err)
				}
			})

			buf := make([]byte, 1500)
			for {
				select {
				case <-ctx.Done():
					log.Info("connection closed by context")
					return
				default:
				}

				n, err := c.Read(buf)
				if err != nil {
					if err != io.EOF {
						log.Error("failed to read from local conn", "err", err)
					}
					break
				}
				if err := dc.Send(buf[:n]); err != nil {
					log.Error("failed to send to data channel", "err", err)
					break
				}
			}
			log.Info("connection closed")
		}(conn)
	}
}
