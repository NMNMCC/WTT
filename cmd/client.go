package cmd

import (
	"context"
	"fmt"
	"wtt/client"
	"wtt/common"
)

// Client 子命令（kong）
type ClientCmd struct {
	HostID           string `name:"host-id" short:"h" required:"" help:"Target host ID to connect to."`
	SignalingAddress string `name:"signaling-address" short:"s" required:"" help:"Signaling server address (ws/wss URL)."`
	LocalAddress     string `name:"local-address" short:"l" required:"" help:"Local address to bridge (eg. 127.0.0.1:22)."`
	Protocol         string `name:"protocol" short:"p" default:"tcp" help:"Transport protocol: tcp or udp."`
}

func (c *ClientCmd) Run() error {
	if c.Protocol != "tcp" && c.Protocol != "udp" {
		return fmt.Errorf("unsupported protocol: %s", c.Protocol)
	}
	return client.Run(context.Background(), c.SignalingAddress, c.HostID, c.LocalAddress, common.Protocol(c.Protocol))
}
