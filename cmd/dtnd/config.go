package main

import (
	"fmt"
	"github.com/dtn7/dtn7-ng/pkg/discovery"
	"net"
	"strconv"

	"github.com/BurntSushi/toml"
	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/cla"
	"github.com/dtn7/dtn7-ng/pkg/routing"
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
	NodeID    bpv7.EndpointID
	LogLevel  log.Level
	Store     storeConfig
	Routing   routingConfig
	Listener  []cla.ListenerConfig
	Agents    agentsConfig
	Discovery []discovery.Announcement
}

type tomlConfig struct {
	NodeID   string `toml:"node_id"`
	LogLevel string `toml:"log_level"`
	Store    storeConfig
	Routing  tomlRoutingConfig
	Listener []listenerTomlConfig
	Agents   agentsConfig
}

type storeConfig struct {
	Path string
}

type tomlRoutingConfig struct {
	Algorithm string
}

type routingConfig struct {
	Algorithm routing.AlgorithmEnum
}

type listenerTomlConfig struct {
	Type    string
	Address string
}

// agentsConfig describes the ApplicationAgents/Agent-configuration block.
type agentsConfig struct {
	REST agentsRESTConfig
}

// agentsWebserverConfig describes the nested "Webserver" configuration for agents.
type agentsRESTConfig struct {
	Address string
}

func parseListenPort(endpoint string) (port int, err error) {
	var portStr string
	_, portStr, err = net.SplitHostPort(endpoint)
	if err != nil {
		return
	}
	port, err = strconv.Atoi(portStr)
	return
}

func parse(filename string) (config, error) {
	var tomlConf tomlConfig
	if _, err := toml.DecodeFile(filename, &tomlConf); err != nil {
		return config{}, NewConfigError("Error parsing toml", err)
	}

	conf := config{
		Listener: make([]cla.ListenerConfig, 0, len(tomlConf.Listener)),
	}

	// Parse and set NodeID
	nodeID, err := bpv7.NewEndpointID(tomlConf.NodeID)
	if err != nil {
		return config{}, NewConfigError("Error parsing NodeID", err)
	}
	conf.NodeID = nodeID

	// Parse and set log level
	logLevel, err := log.ParseLevel(tomlConf.LogLevel)
	if err != nil {
		return config{}, NewConfigError("Error parsing log level", err)
	}
	conf.LogLevel = logLevel

	// Store configuration needs no parsing
	conf.Store = tomlConf.Store

	algorithm, err := routing.AlgorithmEnumFromString(tomlConf.Routing.Algorithm)
	if err != nil {
		return config{}, NewConfigError("Error parsing routing Algorithm", err)
	}

	conf.Routing = routingConfig{Algorithm: algorithm}

	for _, listener := range tomlConf.Listener {
		claType, err := cla.TypeFromString(listener.Type)
		if err != nil {
			return config{}, NewConfigError("Error parsing Listener Type", err)
		}
		conf.Listener = append(conf.Listener, cla.ListenerConfig{Type: claType, Address: listener.Address, EndpointId: nodeID})

		port, err := parseListenPort(listener.Address)
		if err != nil {
			return config{}, NewConfigError("Error parsing listener port", err)
		}
		conf.Discovery = append(conf.Discovery, discovery.Announcement{Type: claType, Port: uint(port), Endpoint: nodeID})
	}

	conf.Agents = tomlConf.Agents

	return conf, nil
}
