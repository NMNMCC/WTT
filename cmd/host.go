package cmd

import (
	"context"
	"fmt"
	"wtt/common"
	"wtt/host"
)

// HostCmd defines the command for running the host.
// It waits for a client to connect through the signaling server and bridges a local port.
type HostCmd struct {
	ID               string   `name:"id" short:"i" help:"Host ID."`
	SignalingAddress string   `name:"signaling-address" short:"s" help:"Signaling server address (ws/wss URL)."`
	LocalAddress     string   `name:"local-address" short:"l" help:"Local address to bridge (e.g. 127.0.0.1:22)."`
	Protocol         string   `name:"protocol" short:"p" default:"tcp" help:"Transport protocol: tcp or udp."`
	STUNAddresses    []string `name:"stun-addresses" short:"t" default:"stun:stun.l.google.com:19302" help:"STUN server addresses."`
	Token            string   `name:"token" short:"k" help:"Authentication token if required by server."`
}

// Run starts the host with the given configuration.
func (h *HostCmd) Run() error {
	if h.Protocol != "tcp" && h.Protocol != "udp" {
		return fmt.Errorf("unsupported protocol: %s", h.Protocol)
	}

	return host.Run(context.Background(), h.SignalingAddress, h.LocalAddress, common.Protocol(h.Protocol))
}
