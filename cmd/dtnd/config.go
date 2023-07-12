package main

import (
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/cla"
)

type ConfigError struct {
	message string
	cause   error
}

func NewConfigError(message string, cause error) *ConfigError {
	return &ConfigError{message: message, cause: cause}
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("Error during config parsing: %v", e.message)
}

func (e *ConfigError) Unwrap() error { return e.cause }

type config struct {
	NodeID   bpv7.EndpointID
	Store    storeConfig
	Listener []cla.ListenerConfig
}

type tomlConfig struct {
	NodeID   string `toml:"node_id"`
	Store    storeConfig
	Listener []listenerTomlConfig
}

type storeConfig struct {
	Path string
}

type listenerTomlConfig struct {
	Type    string
	Address string
}

func parse(filename string) (config, error) {
	var tomlConf tomlConfig
	if _, err := toml.DecodeFile(filename, &tomlConf); err != nil {
		return config{}, NewConfigError("Error parsing toml", err)
	}

	conf := config{
		Listener: make([]cla.ListenerConfig, 0, len(tomlConf.Listener)),
	}

	nodeID, err := bpv7.NewEndpointID(tomlConf.NodeID)
	if err != nil {
		return config{}, NewConfigError("Error parsing NodeID", err)
	}

	conf.NodeID = nodeID

	conf.Store = tomlConf.Store

	for _, listener := range tomlConf.Listener {
		claType, err := cla.TypeFromString(listener.Type)
		if err != nil {
			return config{}, NewConfigError("Error parsing Listener Type", err)
		}
		conf.Listener = append(conf.Listener, cla.ListenerConfig{Type: claType, Address: listener.Address})
	}

	return conf, nil
}
