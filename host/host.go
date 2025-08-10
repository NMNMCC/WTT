package host

import (
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"webrtc-tunnel/signal"
)

const (
	stunServer = "stun:stun.l.google.com:19302"
)

type peerConnectionManager struct {
	hostID      string
	remoteAddr  string
	protocol    string
	signalConn  *websocket.Conn
	peerCons    map[string]*webrtc.PeerConnection
	webRTCAPI   *webrtc.API
	config      webrtc.Configuration
}

func newPeerConnectionManager(hostID, remoteAddr, protocol string, signalConn *websocket.Conn) *peerConnectionManager {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{stunServer}},
		},
	}
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		glog.Fatalf("Failed to register default codecs: %v", err)
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))

	return &peerConnectionManager{
		hostID:      hostID,
		remoteAddr:  remoteAddr,
		protocol:    protocol,
		signalConn:  signalConn,
		peerCons:    make(map[string]*webrtc.PeerConnection),
		webRTCAPI:   api,
		config:      config,
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

		var msg signal.Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			glog.Errorf("Error unmarshaling signal message: %v", err)
			continue
		}

		switch msg.Type {
		case signal.MessageTypeOffer:
			var payload signal.OfferPayload
			if err := RemarshalPayload(msg.Payload, &payload); err != nil {
				glog.Errorf("Error parsing offer payload: %v", err)
				continue
			}
			glog.Infof("Received offer from client: %s", msg.SenderID)
			m.handleOffer(msg.SenderID, payload.SDP)
		case signal.MessageTypeCandidate:
			var payload signal.CandidatePayload
			if err := RemarshalPayload(msg.Payload, &payload); err != nil {
				glog.Errorf("Error parsing candidate payload: %v", err)
				continue
			}
			glog.Infof("Received ICE candidate from client: %s", msg.SenderID)
			m.handleCandidate(msg.SenderID, payload.Candidate)
		}
	}
}

func (m *peerConnectionManager) handleOffer(clientID string, sdp webrtc.SessionDescription) {
	pc, err := m.webRTCAPI.NewPeerConnection(m.config)
	if err != nil {
		glog.Errorf("Failed to create peer connection for client %s: %v", clientID, err)
		return
	}
	m.peerCons[clientID] = pc

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		candidate := c.ToJSON()
		payload := signal.CandidatePayload{Candidate: candidate}
		msg := signal.Message{
			Type:     signal.MessageTypeCandidate,
			Payload:  payload,
			TargetID: clientID,
			SenderID: m.hostID,
		}
		glog.Infof("Sending ICE candidate to client %s", clientID)
		m.signalConn.WriteJSON(msg)
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
			pc.Close()
			delete(m.peerCons, clientID)
		}
	})

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

	payload := signal.AnswerPayload{SDP: answer}
	msg := signal.Message{
		Type:     signal.MessageTypeAnswer,
		Payload:  payload,
		TargetID: clientID,
		SenderID: m.hostID,
	}
	glog.Infof("Sending answer to client %s", clientID)
	m.signalConn.WriteJSON(msg)
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


func Run(signalAddr, id, remoteAddr, protocol string) error {
	glog.Infof("Starting host mode. ID: %s, Protocol: %s, Forwarding to: %s", id, protocol, remoteAddr)

	c, _, err := websocket.DefaultDialer.Dial(signalAddr, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to signaling server: %v", err)
	}
	defer c.Close()
	glog.Info("Connected to signaling server.")

	registerMsg := signal.Message{
		Type:     signal.MessageTypeRegisterHost,
		SenderID: id,
	}
	if err := c.WriteJSON(registerMsg); err != nil {
		return fmt.Errorf("failed to register with signaling server: %v", err)
	}
	glog.Info("Registered with signaling server.")

	manager := newPeerConnectionManager(id, remoteAddr, protocol, c)
	manager.handleSignal()

	return nil
}

func RemarshalPayload(payload interface{}, target interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
