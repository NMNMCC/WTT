package common

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/golang/glog"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

// tcp, udp, tcp/udp
type Protocol string

const (
	TCP  Protocol = "tcp"
	UDP  Protocol = "udp"
	Both Protocol = "tcp/udp"
)

type Destination struct {
	Addr string
	Port string
}

type Portal struct {
	Protocol Protocol
	Destination
}

type Tunnel struct {
	Initiator  Portal
	Bootstrap  Destination
	Respondent Portal
}

func setup(ctx context.Context, wrc webrtc.Configuration, tnc Tunnel) (uuid.UUID, *webrtc.PeerConnection, *websocket.Conn, error) {
	id, _ := uuid.NewRandom()
	wr, err := webrtc.NewPeerConnection(wrc)
	if err != nil {
		glog.Error("Failed to create peer connection:", wrc)
		return id, nil, nil, err
	}
	glog.Info("Create connection:", wrc)

	ws, httpResponse, err := websocket.Dial(ctx, tnc.Bootstrap.Addr, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"ID": []string{id.String()},
		},
	})
	if err != nil || httpResponse.StatusCode != 101 {
		glog.Error("Failed to connect to signaling server:", err, httpResponse)
		return id, wr, nil, err
	}

	return id, wr, ws, nil
}

func Init(config webrtc.Configuration, tunnel Tunnel) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	id, wr, ws, err := setup(ctx, config, tunnel)
	if err != nil {
		return err
	}

	go func() {
		for {
			typ, msg, err := ws.Read(ctx)
			if err != nil {
				glog.Error("Failed to read from websocket:", err)
				break
			}
			if typ == websocket.MessageBinary {
				continue
			}
			msgText := string(msg)
			glog.Info("Received message from signaling server:", msgText)

			err = wr.SetRemoteDescription(webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  msgText,
			})
			if err != nil {
				glog.Error("Failed to set remote description:", err)
				continue
			}
			answer, err := wr.CreateAnswer(&webrtc.AnswerOptions{})
			if err != nil {
				glog.Error("Failed to create answer:", err)
				continue
			}
			glog.Info("Create SDP answer:", answer)

			ws.Write(ctx, websocket.MessageText, []byte(answer.SDP))

			ordered := true
			wrDataChannel, err := wr.CreateDataChannel(id.String(), &webrtc.DataChannelInit{
				Ordered: &ordered,
			})
			if err != nil {
				glog.Error("Fail to create data channel:", err)
			}
			glog.Info("Create data channel:", id, wr)

			wr.OnDataChannel(func(dc *webrtc.DataChannel) {
				dc.OnOpen(func() {
					glog.Info("Data channel opened:", dc.Label())
				})

				dc.OnError(func(err error) {
					glog.Error("Data channel error:", err)
				})
			})

		}

		<-ctx.Done()
	}()

	return nil
}
