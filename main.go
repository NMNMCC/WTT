package main

import (
	"flag"

	"webrtc-tunnel/client"
	"webrtc-tunnel/host"
	"webrtc-tunnel/server"

	"github.com/golang/glog"
	"github.com/spf13/pflag"
)

func main() {
	mode := pflag.String("mode", "", "Mode to run in: 'server', 'host', or 'client'")

	// Server flags
	serverAddr := pflag.String("addr", ":8080", "Address for the signaling server to listen on (server mode)")

	// Client/Host flags
	signalAddr := pflag.String("signal", "ws://localhost:8080", "Signaling server address (client/host mode)")
	id := pflag.String("id", "", "ID to connect to (client mode) or register as (host mode)")

	// Client flags
	localAddr := pflag.String("local", "localhost:25565", "Local address to listen on for incoming connections (client mode)")

	// Host flags
	remoteAddr := pflag.String("remote", "localhost:25565", "Remote address to forward traffic to (host mode)")

	protocol := pflag.String("protocol", "tcp", "Protocol to tunnel (tcp or udp)")

	// glog flags are registered in init(), so they will be included here.
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	// Defer flushing all logs before the program exits.
	defer glog.Flush()

	switch *mode {
	case "server":
		if err := server.Run(*serverAddr); err != nil {
			glog.Errorf("Error in server mode: %v", err)
		}
	case "host":
		if *id == "" {
			glog.Error("Host mode requires an -id")
		}
		if err := host.Run(*signalAddr, *id, *remoteAddr, *protocol); err != nil {
			glog.Errorf("Error in host mode: %v", err)
		}
	case "client":
		if *id == "" {
			glog.Error("Client mode requires an -id to connect to")
		}
		if err := client.Run(*signalAddr, *id, *localAddr, *protocol); err != nil {
			glog.Errorf("Error in client mode: %v", err)
		}
	case "":
		glog.Error("--mode is required. Please specify 'server', 'host', or 'client'.")
	default:
		glog.Errorf("Unknown mode: %s. Please use 'server', 'host', or 'client'.", *mode)
	}
}
