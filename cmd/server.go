package cmd

import (
	"context"
	"wtt/server"
)

// ServerCmd represents the server command with its flags.
type ServerCmd struct {
	Listen     string   `name:"listen" short:"l" default:":8080" help:"Listen address for signaling server."`
	Tokens     []string `name:"tokens" short:"t" help:"Allowed tokens for authentication."`
	MaxMsgSize int64    `name:"max-msg-size" default:"1048576" help:"Max websocket message size (bytes)."`
}

// Run executes the server command.
func (s *ServerCmd) Run() error {
	ec := server.Run(context.Background(), s.Listen, s.Tokens, s.MaxMsgSize)
	select {
	case err := <-ec:
		return err
	case <-context.Background().Done():
		return nil
	}
}
