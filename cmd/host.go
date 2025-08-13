package cmd

import (
	"context"
	"log/slog"
	"os"
	"wtt/common"
	"wtt/host"
)

type HostCmd struct {
	ID               string   `name:"id" short:"i" required:"" help:"Host ID."`
	SignalingAddress string   `name:"signaling-address" short:"s" required:"" help:"Signaling server address (ws/wss URL)."`
	LocalAddress     string   `name:"local-address" short:"l" required:"" help:"Local address to bridge (e.g. 127.0.0.1:22)."`
	Protocol         string   `name:"protocol" short:"p" default:"tcp" help:"Transport protocol: tcp or udp."`
	STUNAddresses    []string `name:"stun-addresses" short:"t" default:"stun:stun.l.google.com:19302" help:"STUN server addresses."`
	Token            string   `name:"token" short:"k" help:"Authentication token if required by server."`
}

func (h *HostCmd) Run(ctx AppContext) {
	var logLevel slog.Level
	if ctx.IsVerbose() {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})))

	if h.Protocol != "tcp" && h.Protocol != "udp" {
		slog.Error("unsupported protocol", "protocol", h.Protocol)
		return
	}

	ec := host.Run(context.Background(), h.ID, h.SignalingAddress, h.LocalAddress, common.NetProtocol(h.Protocol))
	slog.Error("host error", "err", <-ec)
}
