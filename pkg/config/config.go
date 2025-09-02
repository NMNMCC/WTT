package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Auth struct {
	Token string `yaml:"token"`
}

type IceServer struct {
	URLs       string `yaml:"urls"`
	Username   string `yaml:"username"`
	Credential string `yaml:"credential"`
}

type ProviderService struct {
	ServiceName   string `yaml:"service_name"`
	TargetAddress string `yaml:"target_address"`
}

type ConsumerService struct {
	Name              string `yaml:"name"`
	LocalAddress      string `yaml:"local_address"`
	RemotePeerID      string `yaml:"remote_peer_id"`
	RemoteServiceName string `yaml:"remote_service_name"`
}

type Config struct {
	Auth            Auth              `yaml:"auth"`
	SignalingServer string            `yaml:"signaling_server"`
	IceServers      []IceServer       `yaml:"ice_servers"`
	Provide         []ProviderService `yaml:"provide"`
	Consume         []ConsumerService `yaml:"consume"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
