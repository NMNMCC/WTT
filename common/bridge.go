package common

import (
	"fmt"
	"io"
	"log/slog"
	"net"

	"github.com/pion/webrtc/v4"
)

func Bridge(protocol NetProtocol, localAddr string, dc *webrtc.DataChannel) <-chan error {
	ec := make(chan error)

	switch protocol {
	case TCP:
		conn, err := net.Dial("tcp", localAddr)
		if err != nil {
			ec <- fmt.Errorf("failed to connect to TCP address %s: %w", localAddr, err)
		}
		defer conn.Close()

		return Merge(ec, BridgeStream(dc, conn))
	case UDP:
		conn, err := net.ListenPacket("udp", localAddr)
		if err != nil {
			ec <- fmt.Errorf("failed to listen on UDP address %s: %w", localAddr, err)
		}
		defer conn.Close()

		return Merge(ec, BridgePacket(dc, conn))
	default:
		ec <- fmt.Errorf("unsupported protocol: %s", protocol)
		return ec
	}
}

// BridgeStream wires a WebRTC DataChannel with a stream-oriented net.Conn (like TCP) bidirectionally.
// It installs the DataChannel handlers and blocks pumping local->remote until EOF/error.
func BridgeStream(dc *webrtc.DataChannel, local net.Conn) <-chan error {
	ec := make(chan error)

	slog.Info("Bridging DataChannel with local connection", "label", dc.Label(), "localAddr", local.RemoteAddr().String())

	if dc == nil || local == nil {
		ec <- fmt.Errorf("nil data channel or local conn")
		return ec
	}
	defer local.Close()
	defer dc.Close()

	// Remote -> Local
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if len(msg.Data) == 0 {
			return
		}
		if _, err := local.Write(msg.Data); err != nil {
			_ = local.Close()
			_ = dc.Close()
		}
	})
	// Propagate remote close to local
	dc.OnClose(func() { _ = local.Close() })

	// Local -> Remote (blocking loop)
	go func() {
		buf := make([]byte, 16384)
		for {
			n, err := local.Read(buf)
			if err != nil {
				if err == io.EOF || n == 0 {
					ec <- err
				}
				if err.Error() == "use of closed network connection" {
					ec <- err
				}
				ec <- fmt.Errorf("read local: %w", err)
			}
			if n == 0 {
				ec <- fmt.Errorf("read local: EOF or zero bytes")
			}
			if err := dc.Send(buf[:n]); err != nil {
				ec <- fmt.Errorf("send to dc: %w", err)
			}
		}
	}()

	return ec
}

// BridgePacket wires a WebRTC DataChannel with a packet-oriented net.PacketConn (like UDP) bidirectionally.
// It starts a goroutine for local->remote and configures remote->local handler.
// Caller owns pconn lifetime; handlers will close on errors.
func BridgePacket(dc *webrtc.DataChannel, pconn net.PacketConn) <-chan error {
	ec := make(chan error)

	slog.Info("Bridging DataChannel with packet connection", "label", dc.Label(), "localAddr", pconn.LocalAddr().String())

	var returnAddr net.Addr

	// Local -> Remote
	go func() {
		buf := make([]byte, 16384)
		for {
			n, addr, err := pconn.ReadFrom(buf)
			if err != nil {
				_ = pconn.Close()
				_ = dc.Close()
				ec <- fmt.Errorf("read from packet conn: %w", err)
				return
			}
			if returnAddr == nil {
				returnAddr = addr
			}
			if n > 0 {
				if err := dc.Send(buf[:n]); err != nil {
					_ = pconn.Close()
					_ = dc.Close()
					ec <- fmt.Errorf("send to dc: %w", err)
					return
				}
			}
		}
	}()

	// Remote -> Local
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if len(msg.Data) == 0 || returnAddr == nil {
			return
		}
		if _, err := pconn.WriteTo(msg.Data, returnAddr); err != nil {
			_ = pconn.Close()
			_ = dc.Close()
			select {
			case ec <- fmt.Errorf("write to packet conn: %w", err):
			default:
			}
		}
	})

	// Cleanup
	dc.OnClose(func() { _ = pconn.Close() })

	// Wait for error or return nil if DataChannel closes cleanly
	return ec
}
