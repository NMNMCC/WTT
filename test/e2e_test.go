package test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
	"wtt/client"
	"wtt/common"
	"wtt/host"
	"wtt/server"
)

func TestE2ETCP(t *testing.T) {
	// Main context for the entire test with a timeout
	mainCtx, cancelAll := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelAll()

	var wg sync.WaitGroup

	// 1. Start a mock local service (an echo server)
	echoAddr, err := startEchoServer(mainCtx, &wg, t)
	if err != nil {
		t.Fatalf("Failed to start echo server: %v", err)
	}
	t.Logf("Echo server listening on %s", echoAddr)

	// 2. Start the signaling server
	sigAddr, err := startSignalingServer(mainCtx, &wg, t)
	if err != nil {
		t.Fatalf("Failed to start signaling server: %v", err)
	}
	t.Logf("Signaling server listening on %s", sigAddr)

	// Allow some time for servers to start
	time.Sleep(100 * time.Millisecond)

	hostID := "test-host"
	clientID := "test-client"

	// 3. Start the host
	wg.Add(1)
	go func() {
		defer wg.Done()
		hostCfg := host.HostConfig{
			ID:        hostID,
			SigAddr:   "ws://" + sigAddr,
			LocalAddr: echoAddr,
			Protocol:  common.TCP,
			Token:     "secret",
		}
		if err := host.Run(mainCtx, hostCfg); err != nil {
			// Don't fail the test here, as errors on shutdown are expected
			slog.Info("Host exited", "error", err)
		}
	}()

	// Give the host a moment to register with the signaling server
	time.Sleep(500 * time.Millisecond)

	// 4. Start the client
	clientListenAddr := "127.0.0.1:18080"
	wg.Add(1)
	go func() {
		defer wg.Done()
		clientCfg := client.ClientConfig{
			ID:        clientID,
			HostID:    hostID,
			SigAddr:   "ws://" + sigAddr,
			LocalAddr: clientListenAddr,
			Protocol:  common.TCP,
			Token:     "secret",
			Timeout:   15,
		}
		if err := client.Run(mainCtx, clientCfg); err != nil {
			// The client shouldn't fail unless there's a real issue.
			t.Errorf("Client failed: %v", err)
		}
	}()

	// Allow time for WebRTC connection to establish
	t.Log("Waiting for WebRTC connection to establish...")
	// Check if client listener is up
	var localConn net.Conn
	for i := 0; i < 10; i++ {
		localConn, err = net.DialTimeout("tcp", clientListenAddr, 1*time.Second)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("Failed to connect to client listener after multiple attempts: %v", err)
	}
	defer localConn.Close()
	t.Log("Successfully connected to client listener")

	// 5. Test data transfer
	testMessage := "Hello, WebRTC Tunnel!"
	t.Logf("Sending message: %s", testMessage)
	_, err = localConn.Write([]byte(testMessage))
	if err != nil {
		t.Fatalf("Failed to write to client listener: %v", err)
	}

	buf := make([]byte, 1024)
	readDeadline, _ := mainCtx.Deadline()
	localConn.SetReadDeadline(readDeadline)
	n, err := localConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read from client listener: %v", err)
	}

	receivedMessage := string(buf[:n])
	t.Logf("Received message: %s", receivedMessage)
	if receivedMessage != testMessage {
		t.Fatalf("Message mismatch: got '%s', want '%s'", receivedMessage, testMessage)
	}

	t.Log("E2E test successful!")

	// Cancel the context to shut everything down
	cancelAll()
	// Wait for all goroutines to finish
	wg.Wait()
}

func startEchoServer(ctx context.Context, wg *sync.WaitGroup, t *testing.T) (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer listener.Close()
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return // Listener closed
				}
				go func(c net.Conn) {
					defer c.Close()
					io.Copy(c, c)
				}(conn)
			}
		}()
		<-ctx.Done()
	}()

	return listener.Addr().String(), nil
}

func startSignalingServer(ctx context.Context, wg *sync.WaitGroup, t *testing.T) (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	addr := listener.Addr().String()
	listener.Close()

	cfg := server.ServerConfig{
		ListenAddr: addr,
		Tokens:     []string{"secret"},
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Run(ctx, cfg); err != nil && err != http.ErrServerClosed {
			t.Errorf("Signaling server failed: %v", err)
		}
	}()

	// Wait for the server to be ready
	for i := 0; i < 20; i++ {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return addr, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return "", fmt.Errorf("server did not start on %s", addr)
}

func TestE2EUDP(t *testing.T) {
	// Main context for the entire test with a timeout
	mainCtx, cancelAll := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelAll()

	var wg sync.WaitGroup

	// 1. Start a mock local service (a UDP echo server)
	echoAddr, err := startUDPEchoServer(mainCtx, &wg, t)
	if err != nil {
		t.Fatalf("Failed to start UDP echo server: %v", err)
	}
	t.Logf("UDP Echo server listening on %s", echoAddr)

	// 2. Start the signaling server
	sigAddr, err := startSignalingServer(mainCtx, &wg, t)
	if err != nil {
		t.Fatalf("Failed to start signaling server: %v", err)
	}
	t.Logf("Signaling server listening on %s", sigAddr)

	// Allow some time for servers to start
	time.Sleep(100 * time.Millisecond)

	hostID := "test-host-udp"
	clientID := "test-client-udp"

	// 3. Start the host
	wg.Add(1)
	go func() {
		defer wg.Done()
		hostCfg := host.HostConfig{
			ID:        hostID,
			SigAddr:   "ws://" + sigAddr,
			LocalAddr: echoAddr,
			Protocol:  common.UDP,
			Token:     "secret",
		}
		if err := host.Run(mainCtx, hostCfg); err != nil {
			slog.Info("Host exited", "error", err)
		}
	}()

	// Give the host a moment to register
	time.Sleep(500 * time.Millisecond)

	// 4. Start the client
	clientListenAddr := "127.0.0.1:18081" // Use a different port
	wg.Add(1)
	go func() {
		defer wg.Done()
		clientCfg := client.ClientConfig{
			ID:        clientID,
			HostID:    hostID,
			SigAddr:   "ws://" + sigAddr,
			LocalAddr: clientListenAddr,
			Protocol:  common.UDP,
			Token:     "secret",
			Timeout:   15,
		}
		if err := client.Run(mainCtx, clientCfg); err != nil {
			t.Errorf("Client failed: %v", err)
		}
	}()

	// Allow time for WebRTC connection to establish
	time.Sleep(2 * time.Second)

	// 5. Test data transfer
	localConn, err := net.Dial("udp", clientListenAddr)
	if err != nil {
		t.Fatalf("Failed to dial client listener: %v", err)
	}
	defer localConn.Close()

	testMessage := "Hello, UDP Tunnel!"
	t.Logf("Sending message: %s", testMessage)
	_, err = localConn.Write([]byte(testMessage))
	if err != nil {
		t.Fatalf("Failed to write to client listener: %v", err)
	}

	buf := make([]byte, 1024)
	readDeadline, _ := mainCtx.Deadline()
	localConn.SetReadDeadline(readDeadline)
	n, err := localConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read from client listener: %v", err)
	}

	receivedMessage := string(buf[:n])
	t.Logf("Received message: %s", receivedMessage)
	if receivedMessage != testMessage {
		t.Fatalf("Message mismatch: got '%s', want '%s'", receivedMessage, testMessage)
	}

	t.Log("E2E UDP test successful!")
	cancelAll()
	wg.Wait()
}

func startUDPEchoServer(ctx context.Context, wg *sync.WaitGroup, t *testing.T) (string, error) {
	pconn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	addr := pconn.LocalAddr().String()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer pconn.Close()

		go func() {
			buf := make([]byte, 16384)
			for {
				n, raddr, err := pconn.ReadFrom(buf)
				if err != nil {
					return
				}
				pconn.WriteTo(buf[:n], raddr)
			}
		}()

		<-ctx.Done()
	}()

	return addr, nil
}
