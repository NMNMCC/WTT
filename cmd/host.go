package cmd

import (
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"webrtc-tunnel/host"
)

// hostCmd represents the host command
var hostCmd = &cobra.Command{
	Use:   "host",
	Short: "Run in host mode",
	Long: `Host mode registers this instance with a signaling server using a unique ID.
It waits for clients to connect and forwards traffic to a specified remote address.`,
	Run: func(cmd *cobra.Command, args []string) {
		id := viper.GetString("host.id")
		if id == "" {
			glog.Fatal("Host mode requires an --id")
		}
		signalAddr := viper.GetString("signal")
		remoteAddr := viper.GetString("host.remote")
		protocol := viper.GetString("protocol")
		stunServer := viper.GetString("stun.server")
		token := viper.GetString("token")

		if err := host.Run(signalAddr, id, remoteAddr, protocol, stunServer, token); err != nil {
			glog.Fatalf("Error in host mode: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(hostCmd)

	hostCmd.Flags().String("id", "", "ID to register as")
	hostCmd.Flags().String("remote", "localhost:25565", "Remote address to forward traffic to")

	// Common flags with client
	hostCmd.PersistentFlags().String("signal", "ws://localhost:8080", "Signaling server address")
	hostCmd.PersistentFlags().String("protocol", "tcp", "Protocol to tunnel (tcp or udp)")
	hostCmd.PersistentFlags().String("stun-server", "stun:stun.l.google.com:19302", "STUN server address")
	hostCmd.PersistentFlags().String("token", "", "Authentication token")

	if err := viper.BindPFlag("host.id", hostCmd.Flags().Lookup("id")); err != nil {
		glog.Fatalf("Failed to bind host.id flag: %v", err)
	}
	if err := viper.BindPFlag("host.remote", hostCmd.Flags().Lookup("remote")); err != nil {
		glog.Fatalf("Failed to bind host.remote flag: %v", err)
	}
	if err := viper.BindPFlag("signal", hostCmd.PersistentFlags().Lookup("signal")); err != nil {
		glog.Fatalf("Failed to bind signal flag: %v", err)
	}
	if err := viper.BindPFlag("protocol", hostCmd.PersistentFlags().Lookup("protocol")); err != nil {
		glog.Fatalf("Failed to bind protocol flag: %v", err)
	}
	if err := viper.BindPFlag("stun.server", hostCmd.PersistentFlags().Lookup("stun-server")); err != nil {
		glog.Fatalf("Failed to bind stun.server flag: %v", err)
	}
	if err := viper.BindPFlag("token", hostCmd.PersistentFlags().Lookup("token")); err != nil {
		glog.Fatalf("Failed to bind token flag: %v", err)
	}
}
