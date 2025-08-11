package common

import (
	"fmt"
	"io"
	"net"

	"github.com/pion/webrtc/v4"
)

// BridgeStream wires a WebRTC DataChannel with a stream-oriented net.Conn (like TCP) bidirectionally.
// It installs the DataChannel handlers and blocks pumping local->remote until EOF/error.
func BridgeStream(dc *webrtc.DataChannel, local net.Conn) error {
	if dc == nil || local == nil {
		return fmt.Errorf("nil data channel or local conn")
	}
	// We need to close both if one closes.
	closer := func() {
		local.Close()
		dc.Close()
	}

	// Remote -> Local
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if _, err := local.Write(msg.Data); err != nil {
			closer()
		}
	})
	// Propagate remote close to local
	dc.OnClose(closer)

	// Local -> Remote (blocking loop)
	buf := make([]byte, 16384)
	for {
		n, err := local.Read(buf)
		if err != nil {
			closer()
			if err == io.EOF {
				return nil // Clean close
			}
			return fmt.Errorf("read local: %w", err)
		}
		if err := dc.Send(buf[:n]); err != nil {
			closer()
			return fmt.Errorf("send to dc: %w", err)
		}
	}
}

// BridgePacket wires a WebRTC DataChannel with a packet-oriented net.PacketConn (like UDP) bidirectionally.
// It starts a goroutine for local->remote and configures remote->local handler.
// Caller owns pconn lifetime; handlers will close on errors.
func BridgePacket(dc *webrtc.DataChannel, pconn net.PacketConn) error {
	var returnAddr net.Addr
	errc := make(chan error, 1)

	closer := func() {
		pconn.Close()
		dc.Close()
	}

	// Local -> Remote
	go func() {
		buf := make([]byte, 16384)
		for {
			n, addr, err := pconn.ReadFrom(buf)
			if err != nil {
				select {
				case errc <- fmt.Errorf("read from packet conn: %w", err):
				default:
				}
				closer()
				return
			}
			if returnAddr == nil {
				returnAddr = addr
			}
			if n > 0 {
				if err := dc.Send(buf[:n]); err != nil {
					select {
					case errc <- fmt.Errorf("send to dc: %w", err):
					default:
					}
					closer()
					return
				}
			}
		}
	}()

	// Remote -> Local
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if returnAddr == nil {
			// We don't know where to send the packet yet.
			return
		}
		if _, err := pconn.WriteTo(msg.Data, returnAddr); err != nil {
			select {
			case errc <- fmt.Errorf("write to packet conn: %w", err):
			default:
			}
			closer()
		}
	})

	// Cleanup
	dc.OnClose(closer)

	return <-errc
}
