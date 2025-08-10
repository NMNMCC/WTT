package host

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
	"syscall"
	"time"

	"webrtc-tunnel/common"
	wtsignal "webrtc-tunnel/signal"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type peerConnectionManager struct {
	hostID     string
	remoteAddr string
	protocol   string
	signalConn *websocket.Conn
	peerCons   map[string]*webrtc.PeerConnection
	webRTCAPI  *webrtc.API
	config     webrtc.Configuration
}

func newPeerConnectionManager(hostID, remoteAddr, protocol string, signalConn *websocket.Conn, stunServer string) *peerConnectionManager {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{stunServer}},
		},
	}
	settingEngine := webrtc.SettingEngine{}
	settingEngine.DetachDataChannels()
	api := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))

	return &peerConnectionManager{
		hostID:     hostID,
		remoteAddr: remoteAddr,
		protocol:   protocol,
		signalConn: signalConn,
		peerCons:   make(map[string]*webrtc.PeerConnection),
		webRTCAPI:  api,
		config:     config,
	}
}

func (m *peerConnectionManager) handleSignal() {
	for {
		_, msgBytes, err := m.signalConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				glog.Errorf("Error reading signal message: %v", err)
			}
			return
		}

		var msg wtsignal.Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			glog.Errorf("Error unmarshaling signal message: %v", err)
			continue
		}

		switch msg.Type {
		case wtsignal.MessageTypeOffer:
			payload, err := common.RemarshalPayload[wtsignal.OfferPayload](msg.Payload)
			if err != nil {
				glog.Errorf("Error parsing offer payload: %v", err)
				continue
			}
			glog.Infof("Received offer from client: %s", msg.SenderID)
			m.handleOffer(msg.SenderID, payload.SDP)
		case wtsignal.MessageTypeCandidate:
			payload, err := common.RemarshalPayload[wtsignal.CandidatePayload](msg.Payload)
			if err != nil {
				glog.Errorf("Error parsing candidate payload: %v", err)
				continue
			}
			glog.Infof("Received ICE candidate from client: %s", msg.SenderID)
			m.handleCandidate(msg.SenderID, payload.Candidate)
		}
	}
}

func (m *peerConnectionManager) handleOffer(clientID string, sdp webrtc.SessionDescription) {
	pc, err := m.newPeerConnection(clientID)
	if err != nil {
		glog.Errorf("Failed to create peer connection for client %s: %v", clientID, err)
		return
	}
	m.peerCons[clientID] = pc

	if err := pc.SetRemoteDescription(sdp); err != nil {
		glog.Errorf("Failed to set remote description for client %s: %v", clientID, err)
		return
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		glog.Errorf("Failed to create answer for client %s: %v", clientID, err)
		return
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		glog.Errorf("Failed to set local description for client %s: %v", clientID, err)
		return
	}

	payload := wtsignal.AnswerPayload{SDP: answer}
	msg := wtsignal.Message{Type: wtsignal.MessageTypeAnswer, Payload: payload, TargetID: clientID, SenderID: m.hostID}
	glog.Infof("Sending answer to client %s", clientID)
	if err := m.signalConn.WriteJSON(msg); err != nil {
		glog.Errorf("Failed to send answer to %s: %v", clientID, err)
	}
}

func (m *peerConnectionManager) newPeerConnection(clientID string) (*webrtc.PeerConnection, error) {
	pc, err := m.webRTCAPI.NewPeerConnection(m.config)
	if err != nil {
		return nil, err
	}

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		candidate := c.ToJSON()
		payload := wtsignal.CandidatePayload{Candidate: candidate}
		msg := wtsignal.Message{Type: wtsignal.MessageTypeCandidate, Payload: payload, TargetID: clientID, SenderID: m.hostID}
		glog.Infof("Sending ICE candidate to client %s", clientID)
		if err := m.signalConn.WriteJSON(msg); err != nil {
			glog.Errorf("Failed to send ICE candidate to %s: %v", clientID, err)
		}
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		glog.Infof("New DataChannel '%s'-%d from client %s", dc.Label(), dc.ID(), clientID)
		dc.OnOpen(func() {
			glog.Infof("Data channel '%s'-%d opened", dc.Label(), dc.ID())
			if m.protocol == "udp" {
				m.proxyTrafficUDP(dc)
			} else {
				m.proxyTrafficTCP(dc)
			}
		})
	})

	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		glog.Infof("Peer connection with %s state has changed: %s", clientID, s.String())
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed {
			glog.Infof("Closing connection with client %s", clientID)
			if pc.ConnectionState() != webrtc.PeerConnectionStateClosed {
				pc.Close()
			}
			delete(m.peerCons, clientID)
		}
	})

	return pc, nil
}

func (m *peerConnectionManager) handleCandidate(clientID string, candidate webrtc.ICECandidateInit) {
	pc, ok := m.peerCons[clientID]
	if !ok {
		glog.Warningf("Received candidate for unknown client: %s", clientID)
		return
	}
	if err := pc.AddICECandidate(candidate); err != nil {
		glog.Errorf("Failed to add ICE candidate for client %s: %v", clientID, err)
	}
}

func (m *peerConnectionManager) proxyTrafficTCP(dc *webrtc.DataChannel) {
	glog.Infof("Attempting to connect to remote service at %s (tcp)", m.remoteAddr)
	conn, err := net.Dial("tcp", m.remoteAddr)
	if err != nil {
		glog.Errorf("Failed to connect to remote service: %v", err)
		dc.Close()
		return
	}
	glog.Info("Successfully connected to remote service. Proxying TCP traffic.")

	dcRaw, err := dc.Detach()
	if err != nil {
		glog.Errorf("Failed to detach data channel: %v", err)
		conn.Close()
		return
	}

	go func() {
		defer conn.Close()
		defer dcRaw.Close()
		io.Copy(conn, dcRaw)
	}()

	go func() {
		defer conn.Close()
		defer dcRaw.Close()
		io.Copy(dcRaw, conn)
	}()
}

func (m *peerConnectionManager) proxyTrafficUDP(dc *webrtc.DataChannel) {
	glog.Infof("Attempting to connect to remote service at %s (udp)", m.remoteAddr)
	conn, err := net.Dial("udp", m.remoteAddr)
	if err != nil {
		glog.Errorf("Failed to connect to remote UDP service: %v", err)
		dc.Close()
		return
	}
	glog.Info("Successfully connected to remote service. Proxying UDP traffic.")

	go func() {
		buf := make([]byte, 16384)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				glog.Errorf("UDP read from remote error: %v", err)
				conn.Close()
				dc.Close()
				return
			}
			if err := dc.Send(buf[:n]); err != nil {
				glog.Errorf("Data channel send error: %v", err)
				return
			}
		}
	}()

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if _, err := conn.Write(msg.Data); err != nil {
			glog.Errorf("UDP write to remote error: %v", err)
		}
	})
}

func Run(signalAddr, id, remoteAddr, protocol, stunServer, token string) error {
	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		glog.Info("Shutdown signal received, cancelling application context.")
		cancelApp()
	}()

	glog.Infof("Starting host mode. ID: %s, Protocol: %s, Forwarding to: %s", id, protocol, remoteAddr)

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

		err := runHostSession(appCtx, signalAddr, id, remoteAddr, protocol, stunServer, token)

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

func runHostSession(ctx context.Context, signalAddr, id, remoteAddr, protocol, stunServer, token string) error {
	headers := http.Header{}
	if token != "" {
		headers.Add("Authorization", "Bearer "+token)
	}

	c, _, err := websocket.DefaultDialer.Dial(signalAddr, headers)
	if err != nil {
		return fmt.Errorf("failed to connect to signaling server: %v", err)
	}
	defer c.Close()
	glog.Info("Connected to signaling server.")

	registerMsg := wtsignal.Message{
		Type:     wtsignal.MessageTypeRegisterHost,
		SenderID: id,
	}
	if err := c.WriteJSON(registerMsg); err != nil {
		return fmt.Errorf("failed to register with signaling server: %v", err)
	}
	glog.Info("Registered with signaling server.")

	manager := newPeerConnectionManager(id, remoteAddr, protocol, c, stunServer)

	errChan := make(chan error, 1)
	go func() {
		manager.handleSignal()
		errChan <- nil
	}()

	select {
	case <-ctx.Done():
		c.Close()
		return ctx.Err()
	case <-errChan:
		return fmt.Errorf("signaling connection lost")
	}
}
