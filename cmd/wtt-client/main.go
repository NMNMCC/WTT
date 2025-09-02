package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"wtt/pkg/config"
	"wtt/pkg/signal"
)

// --- Proxying and DataChannel Wrapper ---

// dataChannelWrapper wraps a webrtc.DataChannel to implement io.ReadWriteCloser
type dataChannelWrapper struct {
	dc      *webrtc.DataChannel
	readBuf *bytes.Buffer
	readCh  chan []byte
	mu      sync.Mutex
}

func newDCWrapper(dc *webrtc.DataChannel) (*dataChannelWrapper, error) {
	w := &dataChannelWrapper{
		dc:      dc,
		readBuf: new(bytes.Buffer),
		readCh:  make(chan []byte),
	}
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		w.readCh <- msg.Data
	})
	return w, nil
}

func (w *dataChannelWrapper) Read(p []byte) (n int, err error) {
	// If buffer is empty, wait for new data
	if w.readBuf.Len() == 0 {
		data, ok := <-w.readCh
		if !ok {
			return 0, io.EOF
		}
		w.readBuf.Write(data)
	}
	return w.readBuf.Read(p)
}

func (w *dataChannelWrapper) Write(p []byte) (n int, err error) {
	err = w.dc.Send(p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *dataChannelWrapper) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.readCh != nil {
		close(w.readCh)
		w.readCh = nil
	}
	return w.dc.Close()
}

// proxy copies data between two connections and closes them when done.
func proxy(a, b io.ReadWriteCloser) {
	defer a.Close()
	defer b.Close()
	log.Println("Proxying started...")
	go func() {
		io.Copy(a, b)
		a.Close() // unblock the other Copy
	}()
	io.Copy(b, a)
	log.Println("Proxying finished.")
}

// AuthResponse is the expected response from the /api/v1/auth endpoint.
type AuthResponse struct {
	JWT string `json:"jwt"`
}

// WTTClient manages the connection to the signaling server and all P2P connections.
type WTTClient struct {
	cfg        *config.Config
	ws         *websocket.Conn
	peerID     string
	iceServers []webrtc.ICEServer

	// A map of peer connections, keyed by the remote peer's ID.
	connections map[string]*webrtc.PeerConnection
	connLock    sync.Mutex
}

func main() {
	log.Println("Starting WTT Client...")

	// 1. Load configuration
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// 2. Authenticate and connect
	ws, jwt := authenticateAndConnect(cfg)

	// 3. Register with signaling server
	if err := register(ws, jwt); err != nil {
		log.Fatalf("Registration failed: %v", err)
	}

	// 4. Listen for 'registered' message to get our PeerID
	peerID, iceServers := listenForRegistration(ws, cfg)

	client := &WTTClient{
		cfg:         cfg,
		ws:          ws,
		peerID:      peerID,
		iceServers:  iceServers,
		connections: make(map[string]*webrtc.PeerConnection),
	}
	log.Printf("Successfully registered with PeerID: %s", client.peerID)

	// 5. Publish services if any
	client.publishServices()

	// 6. Initiate consumer connections if any
	go client.initiateConsumerConnections()

	// 7. Start the main message processing loop
	client.messageLoop()
}

// --- Setup Functions ---

func authenticateAndConnect(cfg *config.Config) (*websocket.Conn, string) {
	wsURL, err := url.Parse(cfg.SignalingServer)
	if err != nil {
		log.Fatalf("Invalid signaling server URL: %v", err)
	}
	authURL := *wsURL
	if authURL.Scheme == "wss" {
		authURL.Scheme = "https"
	} else {
		authURL.Scheme = "http"
	}
	authURL.Path = "/api/v1/auth"

	authReqBody, _ := json.Marshal(map[string]string{"token": cfg.Auth.Token})
	resp, err := http.Post(authURL.String(), "application/json", bytes.NewBuffer(authReqBody))
	if err != nil {
		log.Fatalf("Authentication request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Authentication failed with status: %s", resp.Status)
	}

	var authResp AuthResponse
	json.NewDecoder(resp.Body).Decode(&authResp)
	log.Println("Authentication successful.")

	c, _, err := websocket.DefaultDialer.Dial(cfg.SignalingServer, nil)
	if err != nil {
		log.Fatalf("Failed to connect to signaling server: %v", err)
	}
	log.Println("Connected to signaling server.")
	return c, authResp.JWT
}

func register(ws *websocket.Conn, jwt string) error {
	regMsg := signal.Message{
		Type:    "register",
		Payload: map[string]string{"jwt": jwt},
	}
	regBytes, _ := json.Marshal(regMsg)
	log.Println("Sent register message.")
	return ws.WriteMessage(websocket.TextMessage, regBytes)
}

func listenForRegistration(ws *websocket.Conn, cfg *config.Config) (string, []webrtc.ICEServer) {
	_, msgBytes, err := ws.ReadMessage()
	if err != nil {
		log.Fatalf("Failed to read registration confirmation: %v", err)
	}

	var serverMsg signal.Message
	json.Unmarshal(msgBytes, &serverMsg)

	if serverMsg.Type != "registered" {
		log.Fatalf("Expected 'registered' message, got '%s'", serverMsg.Type)
	}

	payloadBytes, _ := json.Marshal(serverMsg.Payload)
	var registeredPayload signal.RegisteredPayload
	json.Unmarshal(payloadBytes, &registeredPayload)

	// Convert config ICE servers to webrtc.ICEServer
	var iceServers []webrtc.ICEServer
	for _, s := range cfg.IceServers {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs:       []string{s.URLs},
			Username:   s.Username,
			Credential: s.Credential,
		})
	}

	return registeredPayload.PeerID, iceServers
}

func (c *WTTClient) publishServices() {
	if len(c.cfg.Provide) == 0 {
		return
	}
	var serviceNames []string
	for _, s := range c.cfg.Provide {
		serviceNames = append(serviceNames, s.ServiceName)
	}
	pubMsg := signal.Message{
		Type:    "publish_services",
		Payload: signal.PublishServicesPayload{Services: serviceNames},
	}
	c.sendMessage(pubMsg)
	log.Printf("Published services: %s", strings.Join(serviceNames, ", "))
}

func (c *WTTClient) initiateConsumerConnections() {
	for _, service := range c.cfg.Consume {
		log.Printf("Requesting connection to service '%s' from peer '%s'", service.RemoteServiceName, service.RemotePeerID)
		reqMsg := signal.Message{
			Type: "request_connection",
			Payload: signal.RequestConnectionPayload{
				TargetPeerID: service.RemotePeerID,
				ServiceName:  service.RemoteServiceName,
			},
		}
		c.sendMessage(reqMsg)
	}
}

// --- Main Loop & Message Handling ---

func (c *WTTClient) messageLoop() {
	defer c.ws.Close()
	log.Println("Client is running. Waiting for messages...")
	for {
		_, msgBytes, err := c.ws.ReadMessage()
		if err != nil {
			log.Printf("Read error: %v. Exiting.", err)
			break
		}

		var msg signal.Message
		json.Unmarshal(msgBytes, &msg)
		log.Printf("Received message from server: %s", msg.Type)

		switch msg.Type {
		case "connection_request":
			c.handleConnectionRequest(msg.Payload)
		case "signal":
			c.handleSignal(msg.Payload)
		default:
			log.Printf("Unknown message type: %s", msg.Type)
		}
	}
}

func (c *WTTClient) handleConnectionRequest(payload interface{}) {
	payloadBytes, _ := json.Marshal(payload)
	var req signal.ConnectionRequestPayload
	json.Unmarshal(payloadBytes, &req)
	log.Printf("Received connection request from peer %s for service %s", req.FromPeerID, req.ServiceName)

	// This client is a Provider. Create a PeerConnection and send an offer.
	pc, err := c.createPeerConnection(req.FromPeerID)
	if err != nil {
		log.Printf("Failed to create peer connection: %v", err)
		return
	}

	// This client is a Provider. Create a PeerConnection and send an offer.
	dc, err := pc.CreateDataChannel(req.ServiceName, nil)
	if err != nil {
		log.Printf("Failed to create data channel: %v", err)
		return
	}

	// Find the corresponding provider config
	var serviceConfig *config.ProviderService
	for i := range c.cfg.Provide {
		if c.cfg.Provide[i].ServiceName == req.ServiceName {
			serviceConfig = &c.cfg.Provide[i]
			break
		}
	}
	if serviceConfig == nil {
		log.Printf("No provider config found for service %s", req.ServiceName)
		return
	}

	dc.OnOpen(func() {
		log.Printf("Data channel '%s' opened", dc.Label())

		// Connect to the backend service
		parts := strings.Split(serviceConfig.TargetAddress, "://")
		if len(parts) != 2 {
			log.Printf("Invalid target address: %s", serviceConfig.TargetAddress)
			return
		}

		backendConn, err := net.Dial(parts[0], parts[1])
		if err != nil {
			log.Printf("Failed to connect to backend service %s: %v", serviceConfig.TargetAddress, err)
			return
		}
		log.Printf("Connected to backend service: %s", serviceConfig.TargetAddress)

		dcAsReadWriteCloser, err := newDCWrapper(dc)
		if err != nil {
			log.Printf("Failed to wrap datachannel: %v", err)
			return
		}

		// Start proxying
		proxy(backendConn, dcAsReadWriteCloser)
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Printf("Failed to create offer: %v", err)
		return
	}

	if err := pc.SetLocalDescription(offer); err != nil {
		log.Printf("Failed to set local description: %v", err)
		return
	}

	log.Printf("Sending offer to %s", req.FromPeerID)
	c.sendSignal(req.FromPeerID, offer)
}

func (c *WTTClient) handleSignal(payload interface{}) {
	payloadBytes, _ := json.Marshal(payload)
	var sigData signal.SignalData
	json.Unmarshal(payloadBytes, &sigData)

	dataMap, ok := sigData.Data.(map[string]interface{})
	if !ok {
		log.Printf("signal data is not a map")
		return
	}

	// Check if it is an SDP offer or answer
	if _, ok := dataMap["sdp"]; ok {
		var sdp webrtc.SessionDescription
		b, _ := json.Marshal(dataMap)
		json.Unmarshal(b, &sdp)

		if sdp.Type == webrtc.SDPTypeOffer {
			log.Printf("Received offer from %s", sigData.FromPeerID)
			pc, err := c.createPeerConnection(sigData.FromPeerID)
			if err != nil {
				log.Printf("Failed to create peer connection: %v", err)
				return
			}
			if err := pc.SetRemoteDescription(sdp); err != nil {
				log.Printf("Failed to set remote description: %v", err)
				return
			}
			answer, err := pc.CreateAnswer(nil)
			if err != nil {
				log.Printf("Failed to create answer: %v", err)
				return
			}
			if err := pc.SetLocalDescription(answer); err != nil {
				log.Printf("Failed to set local description: %v", err)
				return
			}
			log.Printf("Sending answer to %s", sigData.FromPeerID)
			c.sendSignal(sigData.FromPeerID, answer)
		} else if sdp.Type == webrtc.SDPTypeAnswer {
			log.Printf("Received answer from %s", sigData.FromPeerID)
			c.connLock.Lock()
			pc := c.connections[sigData.FromPeerID]
			c.connLock.Unlock()
			if pc == nil {
				log.Printf("No peer connection found for %s", sigData.FromPeerID)
				return
			}
			if err := pc.SetRemoteDescription(sdp); err != nil {
				log.Printf("Failed to set remote description: %v", err)
			}
		}
	} else if _, ok := dataMap["candidate"]; ok {
		// It's an ICE candidate
		var candidate webrtc.ICECandidateInit
		b, _ := json.Marshal(dataMap)
		json.Unmarshal(b, &candidate)

		log.Printf("Received ICE candidate from %s", sigData.FromPeerID)
		c.connLock.Lock()
		pc := c.connections[sigData.FromPeerID]
		c.connLock.Unlock()
		if pc != nil {
			if err := pc.AddICECandidate(candidate); err != nil {
				log.Printf("Failed to add ICE candidate: %v", err)
			}
		}
	}
}


// --- Helper Functions ---

func (c *WTTClient) createPeerConnection(remotePeerID string) (*webrtc.PeerConnection, error) {
	c.connLock.Lock()
	defer c.connLock.Unlock()

	if pc, ok := c.connections[remotePeerID]; ok {
		return pc, nil
	}

	config := webrtc.Configuration{ICEServers: c.iceServers}
	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}

	c.connections[remotePeerID] = pc

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("Peer connection with %s state has changed: %s", remotePeerID, state.String())
		// TODO: Handle cleanup on failed/closed connections
	})

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		log.Printf("Sending ICE candidate to %s", remotePeerID)
		c.sendSignal(remotePeerID, candidate.ToJSON())
	})

	// This handler is for consumers receiving a data channel
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		log.Printf("New DataChannel '%s' - '%s'\n", dc.Label(), dc.Protocol())

		// Find the corresponding consumer config
		var serviceConfig *config.ConsumerService
		for i := range c.cfg.Consume {
			if c.cfg.Consume[i].RemotePeerID == remotePeerID {
				serviceConfig = &c.cfg.Consume[i]
				break
			}
		}

		if serviceConfig == nil {
			log.Printf("No consumer config found for remote peer %s", remotePeerID)
			return
		}

		dc.OnOpen(func() {
			log.Printf("Data channel '%s' opened", dc.Label())
			// Start listening on the local port
			listener, err := net.Listen("tcp", serviceConfig.LocalAddress)
			if err != nil {
				log.Printf("Failed to listen on %s: %v", serviceConfig.LocalAddress, err)
				return
			}
			log.Printf("Listening on %s for local connections", serviceConfig.LocalAddress)

			// For simplicity, handle one connection at a time
			localConn, err := listener.Accept()
			if err != nil {
				log.Printf("Failed to accept local connection: %v", err)
				return
			}
			log.Printf("Accepted connection from %s", localConn.RemoteAddr())

			// Wrap the data channel
			dcAsReadWriteCloser, err := newDCWrapper(dc)
			if err != nil {
				log.Printf("Failed to wrap datachannel: %v", err)
				return
			}

			// Start proxying
			proxy(localConn, dcAsReadWriteCloser)
		})
	})

	return pc, nil
}

func (c *WTTClient) sendMessage(msg signal.Message) {
	bytes, _ := json.Marshal(msg)
	if err := c.ws.WriteMessage(websocket.TextMessage, bytes); err != nil {
		log.Printf("Failed to send message: %v", err)
	}
}

func (c *WTTClient) sendSignal(toPeerID string, data interface{}) {
	sig := signal.Message{
		Type: "signal",
		Payload: signal.SignalPayload{
			ToPeerID: toPeerID,
			Data:     data,
		},
	}
	c.sendMessage(sig)
}
