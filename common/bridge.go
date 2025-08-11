package common

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

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
// It blocks until the bridge is closed.
func BridgePacket(dc *webrtc.DataChannel, pconn net.PacketConn) error {
	ctx, cancel := context.WithCancel(context.Background())
	var closeOnce sync.Once
	closer := func() {
		closeOnce.Do(func() {
			pconn.Close()
			dc.Close()
			cancel()
		})
	}

	var returnAddr net.Addr
	var mu sync.Mutex

	// Local -> Remote
	go func() {
		defer closer()
		buf := make([]byte, 16384)
		for {
			n, addr, err := pconn.ReadFrom(buf)
			if err != nil {
				if err != io.EOF {
					slog.Warn("Failed to read from packet conn, closing bridge", "error", err)
				}
				return
			}
			mu.Lock()
			if returnAddr == nil {
				returnAddr = addr
			}
			mu.Unlock()
			if n > 0 {
				if err := dc.Send(buf[:n]); err != nil {
					slog.Warn("Failed to send to dc", "error", err)
					return
				}
			}
		}
	}()

	// Remote -> Local
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		mu.Lock()
		addr := returnAddr
		mu.Unlock()
		if addr == nil {
			// We don't know where to send the packet yet.
			slog.Warn("Dropping packet, no return address yet")
			return
		}
		if _, err := pconn.WriteTo(msg.Data, addr); err != nil {
			slog.Warn("Failed to write to packet conn, closing bridge", "error", err)
			closer()
		}
	})

	dc.OnClose(func() {
		slog.Info("DataChannel closed, closing bridge")
		closer()
	})

	<-ctx.Done()
	return nil
}
