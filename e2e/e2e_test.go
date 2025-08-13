package e2e

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"
	"wtt/client"
	"wtt/common"
	"wtt/host"
	"wtt/server"

	"github.com/stretchr/testify/require"
)

// getFreePort asks the kernel for a free open port that is ready to use.
func getFreePort(t *testing.T) int {
	t.Helper()
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	l, err := net.ListenTCP("tcp", addr)
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// echoServer is a simple TCP server that echoes back any data it receives.
func echoServer(t *testing.T, listenAddr string) net.Listener {
	t.Helper()

	l, err := net.Listen("tcp", listenAddr)
	require.NoError(t, err, "failed to start echo server")

	t.Logf("echo server listening on %s", l.Addr().String())

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				t.Logf("echo server accept loop error: %v", err)
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				c.SetDeadline(time.Now().Add(5 * time.Second))
				// Use io.Copy to echo data.
				// The error is logged but doesn't fail the test, as network connections can be flaky during cleanup.
				if _, err := io.Copy(c, c); err != nil {
					t.Logf("echo handler error: %v", err)
				}
			}(conn)
		}
	}()

	return l
}

func TestE2ETCP(t *testing.T) {
	t.Parallel()

	// Set up a cancellable context for the test with a generous timeout
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Configure logging for debugging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))

	// 1. Start echo server on a random port.
	echoPort := getFreePort(t)
	echoAddr := fmt.Sprintf("127.0.0.1:%d", echoPort)
	echoLn := echoServer(t, echoAddr)
	defer echoLn.Close()
	t.Logf("echo server started on %s", echoAddr)

	// 2. Start the signaling server on another random port.
	signalPort := getFreePort(t)
	signalAddr := fmt.Sprintf("127.0.0.1:%d", signalPort)
	signalURL := fmt.Sprintf("http://%s", signalAddr)

	serverErrCh := server.Run(ctx, signalAddr, nil, 1024*1024)
	t.Logf("signaling server started on %s", signalAddr)

	// Wait a moment for the server to be ready.
	time.Sleep(100 * time.Millisecond)

	// 3. Start the host
	hostID := "test-host-tcp"
	hostErrCh := host.Run(ctx, hostID, signalURL, echoAddr, common.TCP)
	t.Logf("host started, forwarding to %s", echoAddr)

	// 4. Start the client
	clientFwdPort := getFreePort(t)
	clientFwdAddr := fmt.Sprintf("127.0.0.1:%d", clientFwdPort)
	clientErrCh := client.Run(ctx, signalURL, hostID, clientFwdAddr, common.TCP)
	t.Logf("client started, forwarding from %s", clientFwdAddr)

	// 5. Poll until we can connect to the client's forwarded port.
	var conn net.Conn
	var err error
	require.Eventually(t, func() bool {
		// Use a short timeout for dialing so that the test doesn't hang
		// if the port never opens.
		conn, err = net.DialTimeout("tcp", clientFwdAddr, time.Second)
		return err == nil
	}, 10*time.Second, 200*time.Millisecond, "client forward port never opened")
	require.NoError(t, err, "failed to connect to client forwarded port")
	defer conn.Close()
	t.Log("successfully connected to client forwarded address", "addr", clientFwdAddr)

	// 6. Send data and assert that the echoed data is correct.
	message := "hello wtt"
	t.Log("sending message:", "msg", message)
	_, err = conn.Write([]byte(message))
	require.NoError(t, err)

	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(buf)
	require.NoError(t, err, "failed to read echoed message")

	received := string(buf[:n])
	t.Log("received message:", "msg", received)
	require.Equal(t, message, received, "received message does not match sent message")

	// 7. Cleanly shut down all components by cancelling the context.
	cancel()

	// Check for errors from the components, allowing time for graceful shutdown.
	for range 3 {
		select {
		case err := <-serverErrCh:
			require.NoError(t, err, "server exited with error")
		case err := <-hostErrCh:
			if err != nil && err.Error() != "context canceled" {
				require.NoError(t, err, "host exited with error")
			}
		case err := <-clientErrCh:
			if err != nil && err.Error() != "context canceled" {
				require.NoError(t, err, "client exited with error")
			}
		case <-time.After(2 * time.Second):
			t.Log("a component did not shut down in time")
		}
	}
}
