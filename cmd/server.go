package cmd

import (
	"context"
	"strings"
	"wtt/server"

	"github.com/urfave/cli/v3"
)

var Server = cli.Command{
	Name: "server",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "listen",
			Aliases: []string{"L"},
			Value:   ":8080",
		},
		&cli.StringSliceFlag{
			Name:        "tokens",
			Aliases:     []string{"t"},
			DefaultText: "token1,token2",
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		cfg := server.ServerConfig{
			ListenAddr: c.String("listen"),
			Tokens:     normalizeTokens(c.StringSlice("tokens")),
		}
		return server.Run(cfg)
	},
}

func normalizeTokens(in []string) []string {
	var out []string
	for _, s := range in {
		for _, p := range strings.Split(s, ",") {
			if q := strings.TrimSpace(p); q != "" {
				out = append(out, q)
			}
		}
	}
	return out
}
