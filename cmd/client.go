package cmd

import (
	"context"
	"log/slog"
	"os"
	"wtt/client"
	"wtt/common"
)

type ClientCmd struct {
	HostID           string `name:"host-id" short:"i" required:"" help:"Target host ID to connect to."`
	SignalingAddress string `name:"signaling-address" short:"s" required:"" help:"Signaling server HTTP address (http/https), e.g. http://127.0.0.1:8080."`
	LocalAddress     string `name:"local-address" short:"l" required:"" help:"Local address to bridge (eg. 127.0.0.1:22)."`
	Protocol         string `name:"protocol" short:"p" default:"tcp" help:"Transport protocol: tcp or udp."`
}

func (c *ClientCmd) Run() error {
	var logLevel slog.Level
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})))

	if c.Protocol != "tcp" && c.Protocol != "udp" {
		slog.Error("unsupported protocol", "protocol", c.Protocol)
		return nil
	}

	ec := client.Run(context.Background(), c.SignalingAddress, c.HostID, c.LocalAddress, common.NetProtocol(c.Protocol))
	slog.Info("client started")

	return <-ec
}
