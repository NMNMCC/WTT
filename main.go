// Filename: webrtc-tunnel/main.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/webrtc/v4"
)

const (
	dataChannelLabel = "tcp-tunnel"
	// 使用公共的 STUN 服务器
	stunServer = "stun:stun.l.google.com:19302"
)

// SignalingMessage represents a signaling message
type SignalingMessage struct {
	Type string                     `json:"type"`
	SDP  *webrtc.SessionDescription `json:"sdp,omitempty"`
	ICE  *webrtc.ICECandidate       `json:"ice,omitempty"`
}

// SignalingServer manages WebSocket connections for signaling
type SignalingServer struct {
	clients map[string]*websocket.Conn
	mutex   sync.RWMutex
	offers  map[string]*webrtc.SessionDescription
	answers map[string]*webrtc.SessionDescription
}

func main() {
	mode := flag.String("mode", "", "Mode to run in: 'server', 'client', or 'signaling'")
	listenAddr := flag.String("listen", "localhost:25565", "Address to listen on (client mode)")
	remoteAddr := flag.String("remote", "localhost:25565", "Remote address to connect to (server mode, e.g., Minecraft server)")
	signalAddr := flag.String("signal", "http://localhost:8080", "Signaling server address")
	signalPort := flag.String("signal-port", "8080", "Port for signaling server (signaling mode)")
	flag.Parse()

	if *mode == "" {
		log.Fatal("Mode is required: -mode=server, -mode=client, or -mode=signaling")
	}

	switch *mode {
	case "signaling":
		runSignalingServer(*signalPort)
	case "server":
		runServerMode(*remoteAddr, *signalAddr)
	case "client":
		runClientMode(*listenAddr, *signalAddr)
	default:
		log.Fatalf("Unknown mode: %s", *mode)
	}
}

// runSignalingServer runs the WebSocket-based signaling server
func runSignalingServer(port string) {
	log.Printf("Starting signaling server on port %s", port)
	
	server := &SignalingServer{
		clients: make(map[string]*websocket.Conn),
		offers:  make(map[string]*webrtc.SessionDescription),
		answers: make(map[string]*webrtc.SessionDescription),
	}

	http.HandleFunc("/ws", server.handleWebSocket)
	http.HandleFunc("/offer", server.handleHTTPOffer)
	http.HandleFunc("/answer", server.handleHTTPAnswer)

	log.Printf("Signaling server running on :%s", port)
	log.Printf("WebSocket endpoint: ws://localhost:%s/ws", port)
	log.Printf("HTTP endpoints: http://localhost:%s/offer, http://localhost:%s/answer", port, port)
	
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Signaling server failed: %v", err)
	}
}

// handleWebSocket handles WebSocket connections for real-time signaling
func (s *SignalingServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		log.Printf("Failed to accept WebSocket connection: %v", err)
		return
	}
	defer conn.Close(websocket.StatusInternalError, "Server error")

	clientID := r.URL.Query().Get("id")
	if clientID == "" {
		clientID = fmt.Sprintf("client_%d", time.Now().UnixNano())
	}

	s.mutex.Lock()
	s.clients[clientID] = conn
	s.mutex.Unlock()

	log.Printf("Client %s connected via WebSocket", clientID)

	ctx := context.Background()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			log.Printf("Client %s disconnected: %v", clientID, err)
			break
		}

		var msg SignalingMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("Invalid message from %s: %v", clientID, err)
			continue
		}

		log.Printf("Received from %s: %s", clientID, msg.Type)
		s.handleSignalingMessage(ctx, clientID, &msg)
	}

	s.mutex.Lock()
	delete(s.clients, clientID)
	s.mutex.Unlock()
}

// handleSignalingMessage processes signaling messages
func (s *SignalingServer) handleSignalingMessage(ctx context.Context, clientID string, msg *SignalingMessage) {
	switch msg.Type {
	case "offer":
		if msg.SDP != nil {
			s.mutex.Lock()
			s.offers[clientID] = msg.SDP
			s.mutex.Unlock()
			log.Printf("Stored offer from %s", clientID)
		}
	case "answer":
		if msg.SDP != nil {
			s.mutex.Lock()
			s.answers[clientID] = msg.SDP
			s.mutex.Unlock()
			log.Printf("Stored answer from %s", clientID)
		}
	}
}

// handleHTTPOffer handles HTTP-based offer posting (for backward compatibility)
func (s *SignalingServer) handleHTTPOffer(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var offer webrtc.SessionDescription
		if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		
		s.mutex.Lock()
		s.offers["http_client"] = &offer
		s.mutex.Unlock()
		
		log.Println("Received offer via HTTP")
		w.WriteHeader(http.StatusOK)
	} else if r.Method == http.MethodGet {
		s.mutex.RLock()
		offer, exists := s.offers["http_client"]
		s.mutex.RUnlock()
		
		if !exists {
			http.Error(w, "No offer available", http.StatusNotFound)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(offer)
	}
}

// handleHTTPAnswer handles HTTP-based answer posting (for backward compatibility)
func (s *SignalingServer) handleHTTPAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var answer webrtc.SessionDescription
		if err := json.NewDecoder(r.Body).Decode(&answer); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		
		s.mutex.Lock()
		s.answers["http_client"] = &answer
		s.mutex.Unlock()
		
		log.Println("Received answer via HTTP")
		w.WriteHeader(http.StatusOK)
	} else if r.Method == http.MethodGet {
		s.mutex.RLock()
		answer, exists := s.answers["http_client"]
		s.mutex.RUnlock()
		
		if !exists {
			http.Error(w, "No answer available", http.StatusNotFound)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(answer)
	}
}

// runServerMode 在运行目标服务的机器上运行 (例如 Minecraft 服务器)
func runServerMode(remoteAddr, signalAddr string) {
	log.Printf("Starting in SERVER mode. Will connect to %s when WebRTC connection is established", remoteAddr)

	// 创建 WebRTC PeerConnection
	peerConnection, err := createPeerConnection()
	if err != nil {
		log.Fatalf("Error creating peer connection: %v", err)
	}

	// Channel to signal when the connection is done
	done := make(chan struct{})

	// 当连接状态改变时记录日志，并在连接关闭/失败时发出信号
	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("Peer Connection State has changed: %s", s.String())
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed || s == webrtc.PeerConnectionStateDisconnected {
			log.Println("Peer connection closed/failed.")
			close(done) // 发出信号，让函数退出
		}
	})

	// 创建数据通道
	dataChannel, err := peerConnection.CreateDataChannel(dataChannelLabel, nil)
	if err != nil {
		log.Fatalf("Error creating data channel: %v", err)
	}

	// 设置 OnOpen 回调，当 WebRTC 连接建立时连接到目标服务
	dataChannel.OnOpen(func() {
		log.Printf("Data channel '%s' opened. Connecting to %s", dataChannel.Label(), remoteAddr)
		
		// 连接到目标服务 (例如 Minecraft 服务器)
		tcpConn, err := net.Dial("tcp", remoteAddr)
		if err != nil {
			log.Printf("Failed to connect to target service: %v", err)
			return
		}
		
		log.Printf("Connected to target service at %s. Starting data forwarding.", remoteAddr)
		pipeData(tcpConn, dataChannel)
	})

	// 创建 Offer
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		log.Fatalf("Error creating offer: %v", err)
	}
	if err := peerConnection.SetLocalDescription(offer); err != nil {
		log.Fatalf("Error setting local description: %v", err)
	}

	// 发送 Offer 到信令服务器
	offerURL := signalAddr + "/offer"
	if err := postSDP(offerURL, offer); err != nil {
		log.Fatalf("Error posting offer: %v", err)
	}
	log.Println("Offer posted to signaling server.")

	// 从信令服务器轮询 Answer
	answerURL := signalAddr + "/answer"
	answer, err := pollForSDP(answerURL)
	if err != nil {
		log.Fatalf("Error polling for answer: %v", err)
	}

	// 设置远端描述
	if err := peerConnection.SetRemoteDescription(*answer); err != nil {
		log.Fatalf("Error setting remote description: %v", err)
	}
	log.Println("Answer received and remote description set.")

	// 阻塞，直到连接关闭
	<-done
	log.Println("Server mode finished.")
}

// runClientMode 在用户本地机器上运行
func runClientMode(listenAddr, signalAddr string) {
	log.Printf("Starting in CLIENT mode. Listening on %s", listenAddr)

	// 从信令服务器轮询 Offer
	offerURL := signalAddr + "/offer"
	offer, err := pollForSDP(offerURL)
	if err != nil {
		log.Fatalf("Could not get offer: %v", err)
	}
	log.Println("Offer received from signaling server.")

	peerConnection, err := createPeerConnection()
	if err != nil {
		log.Fatalf("Error creating peer connection: %v", err)
	}

	// 当连接状态改变时记录日志
	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("Peer Connection State has changed: %s", s.String())
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed || s == webrtc.PeerConnectionStateDisconnected {
			log.Println("Peer connection closed/failed. Exiting.")
			os.Exit(0)
		}
	})

	// 当数据通道可用时设置回调
	peerConnection.OnDataChannel(func(dc *webrtc.DataChannel) {
		log.Printf("New DataChannel %s-%d", dc.Label(), dc.ID())

		dc.OnOpen(func() {
			log.Printf("Data channel '%s' opened. Waiting for local connection...", dc.Label())
			// 监听本地端口
			listener, err := net.Listen("tcp", listenAddr)
			if err != nil {
				log.Fatalf("Failed to listen on local address: %v", err)
			}
			log.Printf("Now listening on %s. Please connect your client.", listenAddr)

			// 接受本地连接 (例如，来自 Minecraft 客户端)
			tcpConn, err := listener.Accept()
			if err != nil {
				log.Printf("Failed to accept local connection: %v", err)
				return
			}
			log.Printf("Accepted local connection from %s. Starting to pipe data.", tcpConn.RemoteAddr())
			// 一旦接受连接，关闭监听器，因为我们只处理一个连接
			listener.Close()
			pipeData(tcpConn, dc)
		})
	})

	// 设置远端描述 (Offer)
	if err := peerConnection.SetRemoteDescription(*offer); err != nil {
		log.Fatalf("Failed to set remote description: %v", err)
	}

	// 创建 Answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		log.Fatalf("Failed to create answer: %v", err)
	}
	if err := peerConnection.SetLocalDescription(answer); err != nil {
		log.Fatalf("Failed to set local description: %v", err)
	}

	// 发送 Answer 到信令服务器
	answerURL := signalAddr + "/answer"
	if err := postSDP(answerURL, answer); err != nil {
		log.Fatalf("Failed to post answer: %v", err)
	}
	log.Println("Answer posted to signaling server.")

	// 等待程序退出信号
	waitForShutdown()
}

// createPeerConnection 创建并配置一个新的 WebRTC PeerConnection
func createPeerConnection() (*webrtc.PeerConnection, error) {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{stunServer}},
			// 添加更多的 STUN 服务器以提高连接成功率
			{URLs: []string{"stun:stun1.l.google.com:19302"}},
			{URLs: []string{"stun:stun2.l.google.com:19302"}},
		},
	}
	
	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}
	
	// 添加 ICE 候选者事件处理
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			log.Printf("ICE Candidate: %s", candidate.String())
		}
	})
	
	return pc, nil
}

// postSDP 将 SDP (offer/answer) 发送到信令服务器
func postSDP(url string, sdp webrtc.SessionDescription) error {
	payload, err := json.Marshal(sdp)
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status from signaling server: %s", resp.Status)
	}
	return nil
}

// pollForSDP 从信令服务器轮询 SDP
func pollForSDP(url string) (*webrtc.SessionDescription, error) {
	for {
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("Error getting SDP, retrying... (%v)", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return nil, err
			}
			var sdp webrtc.SessionDescription
			if err := json.Unmarshal(body, &sdp); err != nil {
				return nil, err
			}
			return &sdp, nil
		}
		resp.Body.Close()
		time.Sleep(1 * time.Second) // 等待一秒再重试
	}
}

// pipeData 在 TCP 连接和 WebRTC 数据通道之间双向传输数据
func pipeData(tcpConn net.Conn, dc *webrtc.DataChannel) {
	log.Println("Data piping started.")

	// 当数据通道关闭时，也关闭 TCP 连接
	dc.OnClose(func() {
		log.Println("Data channel has been closed.")
		tcpConn.Close()
	})

	// 设置 OnMessage 回调，将从 WebRTC 收到的数据转发到 TCP 连接
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		_, err := tcpConn.Write(msg.Data)
		if err != nil {
			if err != io.EOF {
				log.Printf("TCP write error: %v", err)
			}
		}
	})

	// 从 TCP 连接读取数据并转发到 WebRTC 数据通道
	go func() {
		defer tcpConn.Close()
		defer dc.Close()

		buf := make([]byte, 16384) // 16KB buffer
		for {
			n, err := tcpConn.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("TCP read error: %v", err)
				}
				return // 发生错误或连接关闭时退出 goroutine
			}
			if err := dc.Send(buf[:n]); err != nil {
				log.Printf("Data channel send error: %v", err)
				return // 发生错误时退出 goroutine
			}
		}
	}()
}

// waitForShutdown 等待 Ctrl+C
func waitForShutdown() {
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		log.Println("Shutting down...")
		done <- true
	}()

	<-done
}
