package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"webrtc-tunnel/common"
	wtsignal "webrtc-tunnel/signal"
)

type tcpProxy struct {
	pc        *webrtc.PeerConnection
	localAddr string
	once      sync.Once
}

func (p *tcpProxy) manage() {
	p.once.Do(func() {
		go func() {
			listener, err := net.Listen("tcp", p.localAddr)
			if err != nil {
				glog.Fatalf("Failed to listen on %s for TCP: %v", p.localAddr, err)
			}
			defer listener.Close()
			glog.Infof("Listening on %s for multiple TCP connections.", p.localAddr)
			for {
				localConn, err := listener.Accept()
				if err != nil {
					glog.Errorf("Failed to accept new TCP connection: %v", err)
					return
				}
				glog.Infof("Accepted new local connection from %s", localConn.RemoteAddr())
				dcLabel := fmt.Sprintf("tcp-%s", localConn.RemoteAddr().String())
				dc, err := p.pc.CreateDataChannel(dcLabel, nil)
				if err != nil {
					glog.Errorf("Failed to create data channel for %s: %v", localConn.RemoteAddr(), err)
					localConn.Close()
					continue
				}
				dc.OnOpen(func() {
					glog.Infof("Data channel '%s' opened for connection %s.", dc.Label(), localConn.RemoteAddr())
					proxyTrafficTCP(dc, localConn)
				})
				dc.OnClose(func() {
					glog.Infof("Data channel '%s' for %s closed.", dc.Label(), localConn.RemoteAddr())
				})
			}
		}()
	})
}

func Run(signalAddr, hostID, localAddr, protocol, stunServer, token string) error {
	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		glog.Info("Shutdown signal received, cancelling application context.")
		cancelApp()
	}()

	clientID := uuid.New().String()
	glog.Infof("Starting client mode. Client ID: %s", clientID)

	var attempt int
	for {
		attempt++
		select {
		case <-appCtx.Done():
			glog.Info("Application shutdown initiated. Will not reconnect.")
			return nil
		default:
			glog.Infof("Connection attempt #%d to %s", attempt, signalAddr)
		}

		sessionCtx, cancelSession := context.WithCancel(appCtx)
		err := runClientSession(sessionCtx, cancelSession, clientID, signalAddr, hostID, localAddr, protocol, stunServer, token)
		cancelSession()

		if err != nil {
			glog.Errorf("Session ended with error: %v", err)
		} else {
			glog.Info("Session ended gracefully.")
		}

		if appCtx.Err() != nil {
			glog.Info("Application context is done, exiting.")
			return nil
		}

		backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
		glog.Infof("Will attempt to reconnect in %v.", backoff)

		select {
		case <-appCtx.Done():
			glog.Info("Application shutdown initiated during backoff.")
			return nil
		case <-time.After(backoff):
		}
	}
}

func runClientSession(sessionCtx context.Context, cancelSession context.CancelFunc, clientID, signalAddr, hostID, localAddr, protocol, stunServer, token string) error {
	conn, err := connectToSignalingServer(signalAddr, token)
	if err != nil {
		return err
	}
	defer conn.Close()

	pc, err := newPeerConnection(stunServer)
	if err != nil {
		return err
	}

	proxy := &tcpProxy{pc: pc, localAddr: localAddr}
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		glog.Infof("Peer connection state has changed: %s", s.String())
		if s == webrtc.PeerConnectionStateConnected && protocol == "tcp" {
			proxy.manage()
		}
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed || s == webrtc.PeerConnectionStateDisconnected {
			glog.Warningf("Peer connection state is %s, ending session.", s.String())
			if pc.ConnectionState() != webrtc.PeerConnectionStateClosed {
				pc.Close()
			}
			cancelSession()
		}
	})

	if err := setupDataChannel(pc, protocol, localAddr); err != nil {
		return err
	}

	go handleSignaling(sessionCtx, cancelSession, conn, pc, clientID, hostID)

	<-sessionCtx.Done()
	return sessionCtx.Err()
}

func connectToSignalingServer(signalAddr, token string) (*websocket.Conn, error) {
	headers := http.Header{}
	if token != "" {
		headers.Add("Authorization", "Bearer "+token)
	}
	conn, _, err := websocket.DefaultDialer.Dial(signalAddr, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to signaling server: %v", err)
	}
	glog.Info("Connected to signaling server.")
	return conn, nil
}

func newPeerConnection(stunServer string) (*webrtc.PeerConnection, error) {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{stunServer}}},
	}
	settingEngine := webrtc.SettingEngine{}
	settingEngine.DetachDataChannels()
	api := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))
	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %v", err)
	}
	return pc, nil
}

func handleSignaling(sessionCtx context.Context, cancelSession context.CancelFunc, conn *websocket.Conn, pc *webrtc.PeerConnection, clientID, hostID string) {
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		payload := wtsignal.CandidatePayload{Candidate: c.ToJSON()}
		msg := wtsignal.Message{
			Type:     wtsignal.MessageTypeCandidate,
			Payload:  payload,
			TargetID: hostID,
			SenderID: clientID,
		}
		glog.Info("Sending ICE candidate to host.")
		conn.WriteJSON(msg)
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		glog.Errorf("failed to create offer: %v", err)
		cancelSession()
		return
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		glog.Errorf("failed to set local description: %v", err)
		cancelSession()
		return
	}

	offerPayload := wtsignal.OfferPayload{SDP: offer}
	offerMsg := wtsignal.Message{Type: wtsignal.MessageTypeOffer, Payload: offerPayload, TargetID: hostID, SenderID: clientID}
	if err := conn.WriteJSON(offerMsg); err != nil {
		glog.Errorf("failed to send offer: %v", err)
		cancelSession()
		return
	}
	glog.Info("Offer sent to host.")

	for {
		select {
		case <-sessionCtx.Done():
			return
		default:
			_, msgBytes, err := conn.ReadMessage()
			if err != nil {
				select {
				case <-sessionCtx.Done():
				default:
					glog.Warning("Signaling connection closed unexpectedly.")
					cancelSession()
				}
				return
			}

			var msg wtsignal.Message
			if err := json.Unmarshal(msgBytes, &msg); err != nil {
				glog.Errorf("Error unmarshaling signal message: %v", err)
				continue
			}

			switch msg.Type {
			case wtsignal.MessageTypeAnswer:
				payload, err := common.RemarshalPayload[wtsignal.AnswerPayload](msg.Payload)
				if err != nil {
					glog.Errorf("Error parsing answer payload: %v", err)
					continue
				}
				glog.Info("Received answer from host.")
				if err := pc.SetRemoteDescription(payload.SDP); err != nil {
					glog.Errorf("Failed to set remote description: %v", err)
				}
			case wtsignal.MessageTypeCandidate:
				payload, err := common.RemarshalPayload[wtsignal.CandidatePayload](msg.Payload)
				if err != nil {
					glog.Errorf("Error parsing candidate payload: %v", err)
					continue
				}
				glog.Info("Received ICE candidate from host.")
				if err := pc.AddICECandidate(payload.Candidate); err != nil {
					glog.Errorf("Failed to add ICE candidate: %v", err)
				}
			}
		}
	}
}

func setupDataChannel(pc *webrtc.PeerConnection, protocol, localAddr string) error {
	if protocol == "udp" {
		ordered := false
		maxRetransmits := uint16(0)
		opts := &webrtc.DataChannelInit{Ordered: &ordered, MaxRetransmits: &maxRetransmits}
		dc, err := pc.CreateDataChannel("tunnel-udp", opts)
		if err != nil {
			return fmt.Errorf("failed to create UDP data channel: %v", err)
		}
		dc.OnOpen(func() {
			glog.Infof("Data channel '%s' opened for UDP. Waiting for local connection on %s.", dc.Label(), dc.ID())
			localConn, err := net.ListenPacket(protocol, localAddr)
			if err != nil {
				glog.Fatalf("Failed to listen on local address (UDP): %v", err)
			}
			glog.Infof("Listening on %s. Please connect your application.", localAddr)
			proxyTrafficUDP(dc, localConn)
		})
	} else {
		glog.Info("TCP mode enabled. Waiting for peer connection to be established...")
	}
	return nil
}

func proxyTrafficTCP(dc *webrtc.DataChannel, localConn net.Conn) {
	dcRaw, err := dc.Detach()
	if err != nil {
		glog.Errorf("Failed to detach data channel: %v", err)
		localConn.Close()
		return
	}
	go func() {
		defer localConn.Close()
		defer dcRaw.Close()
		io.Copy(localConn, dcRaw)
	}()
	go func() {
		defer localConn.Close()
		defer dcRaw.Close()
		io.Copy(dcRaw, localConn)
	}()
}

func proxyTrafficUDP(dc *webrtc.DataChannel, pconn net.PacketConn) {
	var remoteAddr net.Addr
	glog.Info("UDP proxy started. Waiting for first packet from local app to establish return address.")
	go func() {
		buf := make([]byte, 16384)
		for {
			n, addr, err := pconn.ReadFrom(buf)
			if err != nil {
				glog.Errorf("UDP read from local error: %v", err)
				return
			}
			if remoteAddr == nil {
				remoteAddr = addr
				glog.Infof("Established return address for UDP proxy: %s", addr)
			}
			if err := dc.Send(buf[:n]); err != nil {
				glog.Errorf("Data channel send error: %v", err)
				return
			}
		}
	}()
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if remoteAddr == nil {
			glog.Warning("Dropping UDP packet from WebRTC, no return address yet.")
			return
		}
		if _, err := pconn.WriteTo(msg.Data, remoteAddr); err != nil {
			glog.Errorf("UDP write to local error: %v", err)
		}
	})
}
