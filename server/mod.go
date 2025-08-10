package server

import (
	"main/common"
	"net"

	"github.com/golang/glog"
)

type ServerConfig struct {
	Addr string
	Port int
}

func Server(commonConfig common.CommonConfig, config ServerConfig) error {
	listener, err := net.Dial("tcp", net.JoinHostPort(config.Addr, string(config.Port)))
	if err != nil {
		glog.Fatal("Failed to connect to server:", err)
		return err
	}
	defer listener.Close()

	return nil
}
