package main

import (
	"fmt"

	"github.com/spf13/viper"
)

var DefaultClientConfig = ClientConfig{}

type ClientConfig struct {
	GRPC GRPCConfig
	RPC  RPCConfig
}

type GRPCConfig struct {
	URL      string
	Token    string
	Insecure bool
}

type RPCConfig struct {
	URL   string
	Token string
}

func ReadClientConfig(path string) (ClientConfig, error) {
	vp := viper.New()
	vp.SetConfigFile(path)
	if err := vp.ReadInConfig(); err != nil {
		return ClientConfig{}, err
	}
	cfg := DefaultClientConfig
	if err := vp.Unmarshal(&cfg); err != nil {
		return ClientConfig{}, fmt.Errorf("unmarshal config: %w", err)
	}
	return cfg, nil
}
