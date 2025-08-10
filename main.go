// Filename: webrtc-tunnel/main.go
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pion/webrtc/v4"
)

const (
	dataChannelLabel = "tcp-tunnel"
	// 使用公共的 STUN 服务器
	stunServer = "stun:stun.l.google.com:19302"
)

func main() {
	mode := flag.String("mode", "", "Mode to run in: 'server' or 'client'")
	listenAddr := flag.String("listen", "localhost:25565", "Address to listen on (client mode)")
	remoteAddr := flag.String("remote", "localhost:25565", "Remote address to connect to (server mode, e.g., Minecraft server)")
	signalAddr := flag.String("signal", "http://localhost:8080", "Signaling server address")
	flag.Parse()

	if *mode == "" {
		log.Fatal("Mode is required: -mode=server or -mode=client")
	}

	switch *mode {
	case "server":
		runServerMode(*remoteAddr, *signalAddr)
	case "client":
		runClientMode(*listenAddr, *signalAddr)
	default:
		log.Fatalf("Unknown mode: %s", *mode)
	}
}

// runServerMode 在运行目标服务的机器上运行 (例如 Minecraft 服务器)
func runServerMode(remoteAddr, signalAddr string) {
	log.Printf("Starting in SERVER mode. Forwarding to %s", remoteAddr)

	// 监听本地 TCP 连接 (例如，来自游戏的连接)
	listener, err := net.Listen("tcp", remoteAddr)
	if err != nil {
		log.Fatalf("Failed to listen on remote address: %v", err)
	}
	defer listener.Close()
	log.Printf("Listening for connections on %s to forward...", remoteAddr)

	for {
		// 接受一个新的 TCP 连接
		tcpConn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}
		log.Printf("Accepted new connection from %s", tcpConn.RemoteAddr())

		// 为每个新的 TCP 连接创建一个新的 WebRTC PeerConnection
		go handleServerConnection(tcpConn, signalAddr)
	}
}

func handleServerConnection(tcpConn net.Conn, signalAddr string) {
	defer tcpConn.Close()

	peerConnection, err := createPeerConnection()
	if err != nil {
		log.Printf("Error creating peer connection: %v", err)
		return
	}

	// Channel to signal when the connection is done
	done := make(chan struct{})

	// 当连接状态改变时记录日志，并在连接关闭/失败时发出信号
	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("Peer Connection State has changed: %s", s.String())
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed || s == webrtc.PeerConnectionStateDisconnected {
			log.Println("Peer connection closed/failed.")
			close(done) // 发出信号，让 handleServerConnection 退出
		}
	})

	// 创建数据通道
	dataChannel, err := peerConnection.CreateDataChannel(dataChannelLabel, nil)
	if err != nil {
		log.Printf("Error creating data channel: %v", err)
		peerConnection.Close()
		return
	}

	// 设置 OnOpen 回调，当 WebRTC 连接建立时开始数据转发
	dataChannel.OnOpen(func() {
		log.Printf("Data channel '%s' opened. Starting to pipe data.", dataChannel.Label())
		pipeData(tcpConn, dataChannel)
	})

	// 创建 Offer
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		log.Printf("Error creating offer: %v", err)
		peerConnection.Close()
		return
	}
	if err := peerConnection.SetLocalDescription(offer); err != nil {
		log.Printf("Error setting local description: %v", err)
		peerConnection.Close()
		return
	}

	// 发送 Offer 到信令服务器
	if err := postSDP(signalAddr+"/offer", offer); err != nil {
		log.Printf("Error posting offer: %v", err)
		peerConnection.Close()
		return
	}
	log.Println("Offer posted to signaling server.")

	// 从信令服务器轮询 Answer
	answer, err := pollForSDP(signalAddr + "/answer")
	if err != nil {
		log.Printf("Error polling for answer: %v", err)
		peerConnection.Close()
		return
	}

	// 设置远端描述
	if err := peerConnection.SetRemoteDescription(*answer); err != nil {
		log.Printf("Error setting remote description: %v", err)
		peerConnection.Close()
		return
	}
	log.Println("Answer received and remote description set.")

	// 阻塞，直到 OnConnectionStateChange 发出 done 信号
	<-done
	log.Println("handleServerConnection finished.")
}

// runClientMode 在用户本地机器上运行
func runClientMode(listenAddr, signalAddr string) {
	log.Printf("Starting in CLIENT mode. Listening on %s", listenAddr)

	// 从信令服务器轮询 Offer
	offer, err := pollForSDP(signalAddr + "/offer")
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
	if err := postSDP(signalAddr+"/answer", answer); err != nil {
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
		},
	}
	return webrtc.NewPeerConnection(config)
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
