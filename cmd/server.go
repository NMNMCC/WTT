package cmd

import (
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"webrtc-tunnel/server"
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the signaling server",
	Long:  `Starts the WebSocket signaling server that allows hosts and clients to connect and exchange WebRTC metadata.`,
	Run: func(cmd *cobra.Command, args []string) {
		addr := viper.GetString("server.addr")
		certFile := viper.GetString("server.tls.cert-file")
		keyFile := viper.GetString("server.tls.key-file")
		allowedOrigins := viper.GetStringSlice("server.allowed-origins")
		validTokens := viper.GetStringSlice("server.valid-tokens")

		glog.Infof("Starting server on %s", addr)
		if err := server.Run(addr, certFile, keyFile, allowedOrigins, validTokens); err != nil {
			glog.Fatalf("Error in server mode: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().String("addr", ":8080", "Address for the signaling server to listen on")
	serverCmd.Flags().String("tls-cert-file", "", "Path to TLS certificate file for WSS")
	serverCmd.Flags().String("tls-key-file", "", "Path to TLS key file for WSS")
	serverCmd.Flags().StringSlice("allowed-origins", []string{"*"}, "Comma-separated list of allowed origins for WebSocket connections")
	serverCmd.Flags().StringSlice("valid-tokens", []string{}, "Comma-separated list of valid tokens for client/host authentication")

	if err := viper.BindPFlag("server.addr", serverCmd.Flags().Lookup("addr")); err != nil {
		glog.Fatalf("Failed to bind server.addr flag: %v", err)
	}
	if err := viper.BindPFlag("server.tls.cert-file", serverCmd.Flags().Lookup("tls-cert-file")); err != nil {
		glog.Fatalf("Failed to bind server.tls.cert-file flag: %v", err)
	}
	if err := viper.BindPFlag("server.tls.key-file", serverCmd.Flags().Lookup("tls-key-file")); err != nil {
		glog.Fatalf("Failed to bind server.tls.key-file flag: %v", err)
	}
	if err := viper.BindPFlag("server.allowed-origins", serverCmd.Flags().Lookup("allowed-origins")); err != nil {
		glog.Fatalf("Failed to bind server.allowed-origins flag: %v", err)
	}
	if err := viper.BindPFlag("server.auth.valid-tokens", serverCmd.Flags().Lookup("valid-tokens")); err != nil {
		glog.Fatalf("Failed to bind server.auth.valid-tokens flag: %v", err)
	}
}
