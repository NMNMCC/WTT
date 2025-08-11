package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"wtt/cmd"

	"github.com/urfave/cli/v3"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := cli.Command{
		UseShortOptionHandling: true,

		Name:        "WTT",
		Description: "Simple WebRTC Tunnel",

		Commands: []*cli.Command{
			&cmd.Client,
			&cmd.Host,
			&cmd.Server,
		},
	}

	if err := app.Run(ctx, os.Args); err != nil {
		// The glog library logs to stderr by default.
		// We can just exit here. The error will be visible.
		os.Exit(1)
	}
}
