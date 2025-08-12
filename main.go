package main

import (
	"fmt"
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

func (c *CLI) IsVerbose() bool {
	return c.Verbose
}

func main() {
	var cli CLI
	k := kong.Parse(&cli,
		kong.Name("WTT"),
		kong.Description("Simple WebRTC Tunnel"),
		kong.UsageOnError(),
	)
	if err := k.Run(&cli); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
