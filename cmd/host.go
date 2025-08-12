package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"wtt/common"
	"wtt/host"
)

// Host 子命令（原 urfave/cli 版本迁移到 kong）
// 使用示例：wtt host -i <id> -s <signal> -l <local> -p tcp -t stun:stun.l.google.com:19302 -k <token>
// 注：protocol 仅允许 tcp 或 udp。
type HostCmd struct {
	ID               string   `name:"id" short:"i" help:"Host ID."`
	SignalingAddress string   `name:"signaling-address" short:"s" help:"Signaling server address (ws/wss URL)."`
	LocalAddress     string   `name:"local-address" short:"l" help:"Local address to bridge (e.g. 127.0.0.1:22)."`
	Protocol         string   `name:"protocol" short:"p" default:"tcp" help:"Transport protocol: tcp or udp."`
	STUNAddresses    []string `name:"stun-addresses" short:"t" default:"stun:stun.l.google.com:19302" help:"STUN server addresses."`
	Token            string   `name:"token" short:"k" help:"Authentication token if required by server."`
}

func (h *HostCmd) Run(ctx AppContext) error {
	var logLevel slog.Level
	if ctx.IsVerbose() {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})))

	if h.Protocol != "tcp" && h.Protocol != "udp" {
		return fmt.Errorf("unsupported protocol: %s", h.Protocol)
	}

	return host.Run(context.Background(), h.SignalingAddress, h.LocalAddress, common.Protocol(h.Protocol))
}
