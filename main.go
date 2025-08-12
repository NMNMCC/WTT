package main

import (
	"fmt"
	"os"
	"wtt/cmd"

	"github.com/alecthomas/kong"
)

// CLI defines the command-line interface of the application, using kong for parsing.
// It has three main subcommands: client, host, and server.
type CLI struct {
	Client cmd.ClientCmd `cmd:"" help:"Run client."`
	Host   cmd.HostCmd   `cmd:"" help:"Run host."`
	Server cmd.ServerCmd `cmd:"" help:"Run signaling server."`
}

func main() {
	var cli CLI
	k := kong.Parse(&cli,
		kong.Name("WTT"),
		kong.Description("Simple WebRTC Tunnel"),
		kong.UsageOnError(),
	)
	if err := k.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
