package common

import (
	"fmt"
	"io"
	"log/slog"
	"net"

	"github.com/pion/webrtc/v4"
)

// Bridge dials a local address based on the specified protocol and bridges it with the provided WebRTC DataChannel.
// It's a convenience wrapper around BridgeStream and BridgePacket.
func Bridge(protocol Protocol, localAddr string, dc *webrtc.DataChannel) error {
	switch protocol {
	case TCP:
		conn, err := net.Dial("tcp", localAddr)
		if err != nil {
			return fmt.Errorf("failed to connect to TCP address %s: %w", localAddr, err)
		}
		// Note: BridgeStream will close the connection.
		return BridgeStream(dc, conn)
	case UDP:
		conn, err := net.ListenPacket("udp", localAddr)
		if err != nil {
			return fmt.Errorf("failed to listen on UDP address %s: %w", localAddr, err)
		}
		// Note: BridgePacket will close the connection.
		return BridgePacket(dc, conn)
	}

	return fmt.Errorf("unsupported protocol: %s", protocol)
}

// BridgeStream pipes data between a WebRTC DataChannel and a stream-oriented connection (e.g., TCP).
// It handles the bidirectional flow of data and manages the lifecycle of both connections.
// The function blocks until the local connection is closed or an error occurs.
func BridgeStream(dc *webrtc.DataChannel, local net.Conn) error {
	slog.Info("Bridging DataChannel with local stream connection", "label", dc.Label(), "localAddr", local.RemoteAddr().String())
	defer local.Close()
	defer dc.Close()

	// remote -> local
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if _, err := local.Write(msg.Data); err != nil {
			// Errors are expected here when connections are closed, so we don't log them.
			_ = local.Close()
			_ = dc.Close()
		}
	})

	// local -> remote
	// This is a blocking loop that reads from the local connection and sends to the DataChannel.
	buf := make([]byte, 16384) // 16KB buffer
	for {
		n, err := local.Read(buf)
		if err != nil {
			// If the connection is closed, io.EOF will be returned.
			return nil
		}
		if err := dc.Send(buf[:n]); err != nil {
			return fmt.Errorf("failed to send data to DataChannel: %w", err)
		}
	}
}

// BridgePacket pipes data between a WebRTC DataChannel and a packet-oriented connection (e.g., UDP).
// It handles the bidirectional flow of data and manages the lifecycle of both connections.
// This function is non-blocking and returns an error channel.
func BridgePacket(dc *webrtc.DataChannel, pconn net.PacketConn) error {
	slog.Info("Bridging DataChannel with local packet connection", "label", dc.Label(), "localAddr", pconn.LocalAddr().String())
	defer pconn.Close()
	defer dc.Close()

	errc := make(chan error, 1)
	var returnAddr net.Addr

	// remote -> local
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if returnAddr == nil {
			// We haven't received a packet from the local connection yet, so we don't know where to send the data.
			return
		}
		if _, err := pconn.WriteTo(msg.Data, returnAddr); err != nil {
			errc <- fmt.Errorf("failed to write to packet conn: %w", err)
		}
	})

	// local -> remote
	go func() {
		buf := make([]byte, 16384) // 16KB buffer
		for {
			n, addr, err := pconn.ReadFrom(buf)
			if err != nil {
				errc <- nil
				return
			}
			if returnAddr == nil {
				// Save the address of the first packet we receive, so we know where to send return packets.
				returnAddr = addr
			}
			if err := dc.Send(buf[:n]); err != nil {
				errc <- fmt.Errorf("failed to send data to DataChannel: %w", err)
				return
			}
		}
	}()

	return <-errc
}
