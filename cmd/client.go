package cmd

import (
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"webrtc-tunnel/client"
)

// clientCmd represents the client command
var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Run in client mode",
	Long: `Client mode connects to a specific host (via the signaling server)
and exposes the tunnel on a local port.`,
	Run: func(cmd *cobra.Command, args []string) {
		id := viper.GetString("client.id")
		if id == "" {
			glog.Fatal("Client mode requires an --id to connect to")
		}
		signalAddr := viper.GetString("signal")
		localAddr := viper.GetString("client.local")
		protocol := viper.GetString("protocol")
		stunServer := viper.GetString("stun.server")
		token := viper.GetString("token")

		if err := client.Run(signalAddr, id, localAddr, protocol, stunServer, token); err != nil {
			glog.Fatalf("Error in client mode: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(clientCmd)

	clientCmd.Flags().String("id", "", "ID of the host to connect to")
	clientCmd.Flags().String("local", "localhost:25565", "Local address to listen on")

	// Common flags with host
	clientCmd.PersistentFlags().String("signal", "ws://localhost:8080", "Signaling server address")
	clientCmd.PersistentFlags().String("protocol", "tcp", "Protocol to tunnel (tcp or udp)")
	clientCmd.PersistentFlags().String("stun-server", "stun:stun.l.google.com:19302", "STUN server address")
	clientCmd.PersistentFlags().String("token", "", "Authentication token")

	if err := viper.BindPFlag("client.id", clientCmd.Flags().Lookup("id")); err != nil {
		glog.Fatalf("Failed to bind client.id flag: %v", err)
	}
	if err := viper.BindPFlag("client.local", clientCmd.Flags().Lookup("local")); err != nil {
		glog.Fatalf("Failed to bind client.local flag: %v", err)
	}
	if err := viper.BindPFlag("signal", clientCmd.PersistentFlags().Lookup("signal")); err != nil {
		glog.Fatalf("Failed to bind signal flag: %v", err)
	}
	if err := viper.BindPFlag("protocol", clientCmd.PersistentFlags().Lookup("protocol")); err != nil {
		glog.Fatalf("Failed to bind protocol flag: %v", err)
	}
	if err := viper.BindPFlag("stun.server", clientCmd.PersistentFlags().Lookup("stun-server")); err != nil {
		glog.Fatalf("Failed to bind stun.server flag: %v", err)
	}
	if err := viper.BindPFlag("token", clientCmd.PersistentFlags().Lookup("token")); err != nil {
		glog.Fatalf("Failed to bind token flag: %v", err)
	}
}
