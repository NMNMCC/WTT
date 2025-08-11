package cmd

import (
	"context"
	"fmt"
	"wtt/client"
	"wtt/common"

	"github.com/urfave/cli/v3"
)

var Client = cli.Command{
	Name: "client",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "id",
			Aliases: []string{"i"},
		},
		&cli.StringFlag{
			Name:    "host-id",
			Aliases: []string{"h"},
		},
		&cli.StringFlag{
			Name:    "signaling-address",
			Aliases: []string{"s"},
		},
		&cli.StringFlag{
			Name:    "local-address",
			Aliases: []string{"l"},
		},
		&cli.StringFlag{
			Name:    "protocol",
			Aliases: []string{"p"},
			Value:   "tcp",
			Validator: func(v string) error {
				if v != "tcp" && v != "udp" {
					return fmt.Errorf("unsupported protocol: %s", v)
				}
				return nil
			},
		},
		&cli.StringSliceFlag{
			Name:        "stun-addresses",
			Aliases:     []string{"t"},
			DefaultText: "stun:stun.l.google.com:19302",
		},
		&cli.StringFlag{
			Name:    "token",
			Aliases: []string{"k"},
		},
		&cli.Int16Flag{
			Name:    "timeout",
			Aliases: []string{"T"},
			Value:   10,
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		cfg := client.ClientConfig{
			ID:        c.String("id"),
			HostID:    c.String("host-id"),
			SigAddr:   c.String("signaling-address"),
			LocalAddr: c.String("local-address"),
			Protocol:  common.Protocol(c.String("protocol")),
			STUNAddrs: c.StringSlice("stun-addresses"),
			Token:     c.String("token"),
			Timeout:   c.Int("timeout"),
		}

		client.Run(ctx, cfg)
		return nil
	},
}
