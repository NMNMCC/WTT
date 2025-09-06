package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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

	ConsumerInterface
}

type ConsumerInterface interface {
	Serve(ctx context.Context)

	Register(ctx context.Context) error
	Connect(ctx context.Context, sid string) error
	Close(ctx context.Context) error
}

func NewConsumer(cfg ConsumerConfig) (*Consumer, error) {
	return &Consumer{
		ConsumerConfig: &cfg,
		ConsumerState: &ConsumerState{
			Status: ServiceStatusPending,
		},
		ErrorChannel: make(chan error),
	}, nil
}

func (c *Consumer) Register(ctx context.Context) error {
	header := http.Header{}
	header.Set("ID", c.ID)
	header.Set("Type", string(typ.PeerTypeConsumer))

	conn, resp, err := websocket.Dial(ctx, c.ConsumerConfig.Endpoint, &websocket.DialOptions{
		HTTPHeader: header,
	})
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to connect to server"))
	}
	if resp.StatusCode != 101 {
		return fmt.Errorf("failed to connect to server: %s", resp.Body)
	}

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
		c.Status = ServiceStatusActive
		go listenAndBridge(ctx, c.Type, c.Input, dc)
	})

	dc.OnClose(func() {
		c.Close(ctx)
	})

	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}
		payload, err := json.Marshal(typ.ICECandidate{
			Candidate: *i,
		})
		if err != nil {
			c.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to marshal ICE candidate"))
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
			c.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to marshal ICE candidate envelope"))
			return
		}

		if err := c.ServerConn.Write(ctx, websocket.MessageText, evl); err != nil {
			c.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to send ICE candidate"))
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

func (c *Consumer) Close(ctx context.Context) error {
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
	return nil
}

func (c *Consumer) Serve(ctx context.Context) {
	for {
		type_, msg, err := c.ServerConn.Read(ctx)
		if err != nil {
			c.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to receive service message"))
			continue
		}
		if type_ != websocket.MessageText {
			c.ErrorChannel <- fmt.Errorf("invalid service message type")
			continue
		}

		var evl typ.Envelope
		if err := json.Unmarshal(msg, &evl); err != nil {
			c.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to unmarshal service message"))
			continue
		}

		switch evl.Type {
		case typ.EnvelopeTypeRTCAnswerOK:
			var ans typ.RTCAnswerOK
			if err := json.Unmarshal(evl.Payload, &ans); err != nil {
				c.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to unmarshal RTC answer OK"))
				continue
			}
			if err := c.PeerConn.SetRemoteDescription(ans.SessionDescription); err != nil {
				c.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to set remote description"))
				continue
			}
		case typ.EnvelopeTypeRTCAnswerNO:
			var ans typ.RTCAnswerNO
			if err := json.Unmarshal(evl.Payload, &ans); err != nil {
				c.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to unmarshal RTC answer NO"))
				continue
			}
			c.ErrorChannel <- fmt.Errorf("connection rejected by peer: %s", ans.Reason)
			c.Close(ctx)
		case typ.EnvelopeTypeICECandidate:
			var can typ.ICECandidate
			if err := json.Unmarshal(evl.Payload, &can); err != nil {
				c.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to unmarshal ICE candidate"))
				continue
			}
			if err := c.PeerConn.AddICECandidate(can.Candidate.ToJSON()); err != nil {
				c.ErrorChannel <- errors.Join(err, fmt.Errorf("failed to add ICE candidate"))
				continue
			}
		default:
			c.ErrorChannel <- fmt.Errorf("unknown service message type: %s", evl.Type)
			continue
		}
	}
}

var _ ConsumerInterface = (*Consumer)(nil)

func listenAndBridge(ctx context.Context, network, address string, dc *webrtc.DataChannel) {
	l, err := net.Listen(network, address)
	if err != nil {
		log.Printf("failed to listen on %s:%s: %v", network, address, err)
		return
	}
	defer l.Close()

	go func() {
		<-ctx.Done()
		l.Close()
	}()

	log.Printf("listening on %s:%s", network, address)

	for {
		conn, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Printf("listener closed")
				break
			}
			log.Printf("failed to accept connection: %v", err)
			continue
		}

		go func(c net.Conn) {
			log.Printf("accepted connection from %s", c.RemoteAddr())
			defer c.Close()

			go func() {
				<-ctx.Done()
				c.Close()
			}()

			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				if _, err := c.Write(msg.Data); err != nil {
					log.Printf("failed to write to local conn: %v", err)
				}
			})

			buf := make([]byte, 1500)
			for {
				select {
				case <-ctx.Done():
					log.Printf("connection from %s closed by context", c.RemoteAddr())
					return
				default:
				}

				n, err := c.Read(buf)
				if err != nil {
					if err != io.EOF {
						log.Printf("failed to read from local conn: %v", err)
					}
					break
				}
				if err := dc.Send(buf[:n]); err != nil {
					log.Printf("failed to send to data channel: %v", err)
					break
				}
			}
			log.Printf("connection from %s closed", c.RemoteAddr())
		}(conn)
	}
}
