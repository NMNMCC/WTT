package main

import (
	"context"
	"log/slog"
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
		slog.Error("application failed", "error", err)
		os.Exit(1)
	}
}
