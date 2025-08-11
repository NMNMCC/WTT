package main

import (
	"context"
	"os"
	"wtt/cmd"

	"github.com/urfave/cli/v3"
)

func main() {
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

	app.Run(context.Background(), os.Args)
}
