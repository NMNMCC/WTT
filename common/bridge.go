package common

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/pion/webrtc/v4"
)

// BridgeStream wires a WebRTC DataChannel with a stream-oriented net.Conn (like TCP) bidirectionally.
func BridgeStream(dc *webrtc.DataChannel, local net.Conn) <-chan error {
	ec := make(chan error, 1)
	slog.Info("Bridging DataChannel with local TCP connection", "label", dc.Label(), "localAddr", local.LocalAddr(), "remoteAddr", local.RemoteAddr())

	var closeOnce sync.Once
	closeAndSignal := func(err error) {
		closeOnce.Do(func() {
			local.Close()
			dc.Close()
			if err != nil && err != io.EOF {
				ec <- err
			}
			close(ec)
		})
	}

	// Remote -> Local
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if _, err := local.Write(msg.Data); err != nil {
			slog.Error("failed to write to local connection", "err", err)
			closeAndSignal(fmt.Errorf("write to local: %w", err))
		}
	})

	dc.OnClose(func() {
		slog.Debug("data channel closed")
		closeAndSignal(nil) // Clean close
	})

	// Local -> Remote
	go func() {
		buf := make([]byte, 16*1024)
		for {
			n, err := local.Read(buf)
			if n > 0 {
				if sendErr := dc.Send(buf[:n]); sendErr != nil {
					slog.Error("failed to send to data channel", "err", sendErr)
					closeAndSignal(fmt.Errorf("send to dc: %w", sendErr))
					return
				}
			}
			if err != nil {
				if err == io.EOF {
					slog.Debug("local connection closed (EOF)")
					closeAndSignal(nil)
				} else {
					slog.Error("failed to read from local connection", "err", err)
					closeAndSignal(err)
				}
				return
			}
		}
	}()

	return ec
}

// BridgePacket wires a WebRTC DataChannel with a packet-oriented net.PacketConn (like UDP) bidirectionally.
func BridgePacket(dc *webrtc.DataChannel, pconn net.PacketConn) <-chan error {
	ec := make(chan error, 1)
	slog.Info("Bridging DataChannel with packet connection", "label", dc.Label(), "localAddr", pconn.LocalAddr().String())

	var returnAddr net.Addr
	var mu sync.RWMutex

	var closeOnce sync.Once
	closeAndSignal := func(err error) {
		closeOnce.Do(func() {
			pconn.Close()
			dc.Close()
			if err != nil {
				ec <- err
			}
			close(ec)
		})
	}

	// Local -> Remote
	go func() {
		buf := make([]byte, 16*1024)
		for {
			n, addr, err := pconn.ReadFrom(buf)
			if err != nil {
				closeAndSignal(fmt.Errorf("read from packet conn: %w", err))
				return
			}

			mu.Lock()
			if returnAddr == nil {
				slog.Info("setting UDP return address", "addr", addr)
				returnAddr = addr
			}
			mu.Unlock()

			if n > 0 {
				if err := dc.Send(buf[:n]); err != nil {
					closeAndSignal(fmt.Errorf("send to dc: %w", err))
					return
				}
			}
		}
	}()

	// Remote -> Local
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		mu.RLock()
		addr := returnAddr
		mu.RUnlock()

		if addr == nil {
			slog.Warn("dropping message, no return address yet for UDP packet")
			return
		}
		if len(msg.Data) == 0 {
			return
		}
		if _, err := pconn.WriteTo(msg.Data, addr); err != nil {
			closeAndSignal(fmt.Errorf("write to packet conn: %w", err))
		}
	})

	dc.OnClose(func() {
		slog.Debug("data channel closed")
		closeAndSignal(nil) // Clean close
	})

	return ec
}
