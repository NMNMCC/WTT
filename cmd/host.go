package cmd

import (
	"context"
	"log/slog"
	"wtt/common"
	"wtt/host"
)

type HostCmd struct {
	ID               string `name:"id" short:"i" required:"" help:"Host ID."`
	SignalingAddress string `name:"signaling-address" short:"s" required:"" help:"Signaling server HTTP address (http/https), e.g. http://127.0.0.1:8080."`
	LocalAddress     string `name:"local-address" short:"l" required:"" help:"Local address to bridge (e.g. 127.0.0.1:22)."`
	Protocol         string `name:"protocol" short:"p" default:"tcp" help:"Transport protocol: tcp or udp."`
}

func (h *HostCmd) Run() error {

	if h.Protocol != "tcp" && h.Protocol != "udp" {
		slog.Error("unsupported protocol", "protocol", h.Protocol)
		return nil
	}

	return <-host.Run(context.Background(), h.ID, h.SignalingAddress, h.LocalAddress, common.NetProtocol(h.Protocol))
}
