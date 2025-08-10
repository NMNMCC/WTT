package main

import (
	"flag"

	"github.com/golang/glog"
	"webrtc-tunnel/client"
	"webrtc-tunnel/host"
	"webrtc-tunnel/server"
)

func main() {
	mode := flag.String("mode", "", "Mode to run in: 'server', 'host', or 'client'")

	// Server flags
	serverAddr := flag.String("addr", ":8080", "Address for the signaling server to listen on (server mode)")

	// Client/Host flags
	signalAddr := flag.String("signal", "ws://localhost:8080", "Signaling server address (client/host mode)")
	id := flag.String("id", "", "ID to connect to (client mode) or register as (host mode)")

	// Client flags
	localAddr := flag.String("local", "localhost:25565", "Local address to listen on for incoming connections (client mode)")

	// Host flags
	remoteAddr := flag.String("remote", "localhost:25565", "Remote address to forward traffic to (host mode)")

	protocol := flag.String("protocol", "tcp", "Protocol to tunnel (tcp or udp)")

	// glog flags are registered in init(), so they will be included here.
	flag.Parse()

	// Defer flushing all logs before the program exits.
	defer glog.Flush()


	switch *mode {
	case "server":
		if err := server.Run(*serverAddr); err != nil {
			glog.Fatalf("Error in server mode: %v", err)
		}
	case "host":
		if *id == "" {
			glog.Fatal("Host mode requires an -id")
		}
		if err := host.Run(*signalAddr, *id, *remoteAddr, *protocol); err != nil {
			glog.Fatalf("Error in host mode: %v", err)
		}
	case "client":
		if *id == "" {
			glog.Fatal("Client mode requires an -id to connect to")
		}
		if err := client.Run(*signalAddr, *id, *localAddr, *protocol); err != nil {
			glog.Fatalf("Error in client mode: %v", err)
		}
	case "":
		glog.Fatal("-mode is required. Please specify 'server', 'host', or 'client'.")
	default:
		glog.Fatalf("Unknown mode: %s. Please use 'server', 'host', or 'client'.", *mode)
	}
}
