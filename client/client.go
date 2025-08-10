package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/golang/glog"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	wtsignal "webrtc-tunnel/signal"
)

const (
	stunServer = "stun:stun.l.google.com:19302"
)

func Run(signalAddr, hostID, localAddr, protocol string) error {
	clientID := uuid.New().String()
	glog.Infof("Starting client mode. Client ID: %s, Protocol: %s", clientID, protocol)

	conn, _, err := websocket.DefaultDialer.Dial(signalAddr, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to signaling server: %v", err)
	}
	defer conn.Close()
	glog.Info("Connected to signaling server.")

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{stunServer}}},
	}
	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("failed to create peer connection: %v", err)
	}

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

	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		glog.Infof("Peer connection state has changed: %s", s.String())
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed {
			glog.Info("Peer connection failed or closed. Exiting.")
			pc.Close()
			os.Exit(0)
		}
	})

	var dc *webrtc.DataChannel
	if protocol == "udp" {
		ordered := false
		maxRetransmits := uint16(0)
		opts := &webrtc.DataChannelInit{
			Ordered:        &ordered,
			MaxRetransmits: &maxRetransmits,
		}
		dc, err = pc.CreateDataChannel("tunnel-udp", opts)
		if err != nil {
			return fmt.Errorf("failed to create UDP data channel: %v", err)
		}
	} else { // Default to TCP
		dc, err = pc.CreateDataChannel("tunnel-tcp", nil)
		if err != nil {
			return fmt.Errorf("failed to create TCP data channel: %v", err)
		}
	}

	dc.OnOpen(func() {
		glog.Infof("Data channel '%s'-%d opened. Waiting for local connection on %s.", dc.Label(), dc.ID(), localAddr)
		if protocol == "udp" {
			localConn, err := net.ListenPacket(protocol, localAddr)
			if err != nil {
				glog.Errorf("Failed to listen on local address (UDP): %v", err)
				os.Exit(1)
			}
			glog.Infof("Listening on %s. Please connect your application.", localAddr)
			proxyTrafficUDP(dc, localConn)
		} else {
			listener, err := net.Listen(protocol, localAddr)
			if err != nil {
				glog.Errorf("Failed to listen on local address (TCP): %v", err)
				os.Exit(1)
			}
			glog.Infof("Listening on %s. Please connect your application.", localAddr)

			localConn, err := listener.Accept()
			if err != nil {
				glog.Errorf("Failed to accept local connection: %v", err)
				dc.Close()
				return
			}
			glog.Infof("Accepted local connection from %s. Proxying traffic.", localConn.RemoteAddr())
			listener.Close()
			proxyTrafficTCP(dc, localConn)
		}
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("failed to create offer: %v", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("failed to set local description: %v", err)
	}

	offerPayload := wtsignal.OfferPayload{SDP: offer}
	offerMsg := wtsignal.Message{
		Type:     wtsignal.MessageTypeOffer,
		Payload:  offerPayload,
		TargetID: hostID,
		SenderID: clientID,
	}
	if err := conn.WriteJSON(offerMsg); err != nil {
		return fmt.Errorf("failed to send offer: %v", err)
	}
	glog.Info("Offer sent to host.")

	go func() {
		for {
			_, msgBytes, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					glog.Errorf("Error reading signal message: %v", err)
				}
				pc.Close()
				break
			}

			var msg wtsignal.Message
			if err := json.Unmarshal(msgBytes, &msg); err != nil {
				glog.Errorf("Error unmarshaling signal message: %v", err)
				continue
			}

			switch msg.Type {
			case wtsignal.MessageTypeAnswer:
				var payload wtsignal.AnswerPayload
				if err := RemarshalPayload(msg.Payload, &payload); err != nil {
					glog.Errorf("Error parsing answer payload: %v", err)
					continue
				}
				glog.Info("Received answer from host.")
				if err := pc.SetRemoteDescription(payload.SDP); err != nil {
					glog.Errorf("Failed to set remote description: %v", err)
				}
			case wtsignal.MessageTypeCandidate:
				var payload wtsignal.CandidatePayload
				if err := RemarshalPayload(msg.Payload, &payload); err != nil {
					glog.Errorf("Error parsing candidate payload: %v", err)
					continue
				}
				glog.Info("Received ICE candidate from host.")
				if err := pc.AddICECandidate(payload.Candidate); err != nil {
					glog.Errorf("Failed to add ICE candidate: %v", err)
				}
			}
		}
	}()

	waitForShutdown()
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

func RemarshalPayload(payload interface{}, target interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func waitForShutdown() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	glog.Info("Shutting down...")
}
