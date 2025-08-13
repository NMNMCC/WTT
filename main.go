package main

import (
	"log/slog"
	"os"
	"wtt/cmd"

	"github.com/alecthomas/kong"
)

type CLI struct {
	Client  cmd.ClientCmd `cmd:"" help:"Run client."`
	Host    cmd.HostCmd   `cmd:"" help:"Run host."`
	Server  cmd.ServerCmd `cmd:"" help:"Run signaling server."`
	Verbose bool          `name:"verbose" short:"v" help:"Verbose logging."`
	Version bool          `name:"version" help:"Show version."`
}

func main() {
	var cli CLI
	k := kong.Parse(&cli,
		kong.Name("wtt"),
		kong.Description("Simple WebRTC Tunnel"),
		kong.UsageOnError(),
	)

	if cli.Version {
		// minimal version printing; could be replaced by ldflags later
		slog.Info("wtt version", "version", "dev")
		return
	}

	// set log level based on verbose flag
	lvl := slog.LevelInfo
	if cli.Verbose {
		lvl = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))

	if err := k.Run(&cli); err != nil {
		slog.Error("error running command", "err", err)
		os.Exit(1)
	}
}
