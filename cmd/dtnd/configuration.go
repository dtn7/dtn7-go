// SPDX-FileCopyrightText: 2019, 2020, 2022 Markus Sommer
// SPDX-FileCopyrightText: 2019, 2020, 2021 Alvar Penning
//
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/dtn7/dtn7-go/pkg/cla/quicl"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	"github.com/BurntSushi/toml"

	"github.com/dtn7/dtn7-go/pkg/agent"
	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/cla"
	"github.com/dtn7/dtn7-go/pkg/cla/bbc"
	"github.com/dtn7/dtn7-go/pkg/cla/mtcp"
	"github.com/dtn7/dtn7-go/pkg/cla/tcpclv4"
	"github.com/dtn7/dtn7-go/pkg/discovery"
	"github.com/dtn7/dtn7-go/pkg/routing"
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

// tomlConfig describes the TOML-configuration.
type tomlConfig struct {
	Core      coreConf
	Cron      cronConf
	Logging   logConf
	Discovery discoveryConf
	Agents    agentsConfig
	Listen    []convergenceConf
	Peer      []convergenceConf
	Routing   routing.RoutingConf
}

// coreConf describes the Core-configuration block.
type coreConf struct {
	Store             string
	InspectAllBundles bool   `toml:"inspect-all-bundles"`
	NodeId            string `toml:"node-id"`
	SignPriv          string `toml:"signature-private"`
}

type cronConf struct {
	CheckBundles string `toml:"check-bundles"`
	CleanStore   string `toml:"clean-store"`
	CleanID      string `toml:"clean-id"`
}

// logConf describes the Logging-configuration block.
type logConf struct {
	Level        string
	ReportCaller bool `toml:"report-caller"`
	Format       string
}

// discoveryConf describes the Discovery-configuration block.
type discoveryConf struct {
	IPv4     bool
	IPv6     bool
	Interval uint
}

// agentsConfig describes the ApplicationAgents/Agent-configuration block.
type agentsConfig struct {
	Ping      string
	Webserver agentsWebserverConfig
}

// agentsWebserverConfig describes the nested "Webserver" configuration for agents.
type agentsWebserverConfig struct {
	Address   string
	Websocket bool
	Rest      bool
}

// convergenceConf describes the Convergence-configuration block, used for
// "listen" and "peer".
type convergenceConf struct {
	Node     string
	Protocol string
	Endpoint string
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

// parseListen inspects a "listen" convergenceConf and returns a Convergable.
func parseListen(conv convergenceConf, nodeId bpv7.EndpointID) (cla.Convergable, bpv7.EndpointID, cla.CLAType, discovery.Announcement, error) {
	log.WithFields(log.Fields{
		"EndpointID": conv.Node,
		"Endpoint":   conv.Endpoint,
		"Protocol":   conv.Protocol,
	}).Debug("Initialising convergence adaptor")

	// if the user has configured an EndpointID for this convergence adaptor
	if conv.Node != "" {
		parsedId, err := bpv7.NewEndpointID(conv.Node)
		if err != nil {
			return nil, nodeId, 0, discovery.Announcement{}, err
		} else {
			log.WithFields(log.Fields{
				"listener ID": conv.Node,
			}).Debug("Using alternative configured endpoint id for listener")
			nodeId = parsedId
		}
	}

	switch conv.Protocol {
	case "bbc":
		conn, err := bbc.NewBundleBroadcastingConnector(conv.Endpoint, true)
		return conn, nodeId, cla.BBC, discovery.Announcement{}, err

	case "mtcp":
		portInt, err := parseListenPort(conv.Endpoint)
		if err != nil {
			return nil, nodeId, cla.MTCP, discovery.Announcement{}, err
		}

		msg := discovery.Announcement{
			Type:     cla.MTCP,
			Endpoint: nodeId,
			Port:     uint(portInt),
		}

		return mtcp.NewMTCPServer(conv.Endpoint, nodeId, true), nodeId, cla.MTCP, msg, nil

	case "tcpclv4":
		portInt, err := parseListenPort(conv.Endpoint)
		if err != nil {
			return nil, nodeId, cla.TCPCLv4, discovery.Announcement{}, err
		}

		listener := tcpclv4.ListenTCP(conv.Endpoint, nodeId)

		msg := discovery.Announcement{
			Type:     cla.TCPCLv4,
			Endpoint: nodeId,
			Port:     uint(portInt),
		}

		return listener, nodeId, cla.TCPCLv4, msg, nil

	case "tcpclv4-ws":
		listener := tcpclv4.ListenWebSocket(nodeId)

		httpMux := http.NewServeMux()
		httpMux.Handle("/tcpclv4", listener)
		httpServer := &http.Server{
			Addr:              conv.Endpoint,
			Handler:           httpMux,
			ReadHeaderTimeout: 60 * time.Second,
		}

		errChan := make(chan error)
		go func() { errChan <- httpServer.ListenAndServe() }()

		select {
		case err := <-errChan:
			return nil, nodeId, cla.TCPCLv4WebSocket, discovery.Announcement{}, err

		case <-time.After(100 * time.Millisecond):
			return listener, nodeId, cla.TCPCLv4WebSocket, discovery.Announcement{}, nil
		}

	case "quicl":
		portInt, err := parseListenPort(conv.Endpoint)
		if err != nil {
			return nil, nodeId, cla.QUICL, discovery.Announcement{}, err
		}

		listener := quicl.NewQUICListener(conv.Endpoint, nodeId)

		msg := discovery.Announcement{
			Type:     cla.QUICL,
			Endpoint: nodeId,
			Port:     uint(portInt),
		}

		return listener, nodeId, cla.QUICL, msg, nil

	default:
		return nil, nodeId, 0, discovery.Announcement{}, fmt.Errorf("unknown listen.protocol \"%s\"", conv.Protocol)
	}
}

func parsePeer(conv convergenceConf, nodeId bpv7.EndpointID) (cla.ConvergenceSender, error) {

	switch conv.Protocol {
	case "mtcp":
		if endpointID, err := bpv7.NewEndpointID(conv.Node); err != nil {
			return nil, err
		} else {
			return mtcp.NewMTCPClient(conv.Endpoint, endpointID, true), nil
		}

	case "tcpclv4":
		return tcpclv4.DialTCP(conv.Endpoint, nodeId, true), nil

	case "tcpclv4-ws":
		return tcpclv4.DialWebSocket(conv.Endpoint, nodeId, true), nil

	case "quicl":
		return quicl.NewDialerEndpoint(conv.Endpoint, nodeId, true), nil

	default:
		return nil, fmt.Errorf("unknown peer.protocol \"%s\"", conv.Protocol)
	}
}

// parseAgents for the ApplicationAgents.
func parseAgents(conf agentsConfig) (agents []agent.ApplicationAgent, err error) {
	if conf.Ping != "" {
		if pingEid, pingEidErr := bpv7.NewEndpointID(conf.Ping); pingEidErr != nil {
			err = pingEidErr
			return
		} else {
			agents = append(agents, agent.NewPing(pingEid))
		}
	}

	if (conf.Webserver != agentsWebserverConfig{}) {
		if !conf.Webserver.Websocket && !conf.Webserver.Rest {
			err = fmt.Errorf("webserver agent needs at least one of Websocket or REST")
			return
		}

		r := mux.NewRouter()

		if conf.Webserver.Websocket {
			ws := agent.NewWebSocketAgent()
			r.HandleFunc("/ws", ws.ServeHTTP)

			agents = append(agents, ws)
		}

		if conf.Webserver.Rest {
			restRouter := r.PathPrefix("/rest").Subrouter()
			ra := agent.NewRestAgent(restRouter)

			agents = append(agents, ra)
		}

		httpServer := &http.Server{
			Addr:              conf.Webserver.Address,
			Handler:           r,
			ReadHeaderTimeout: 60 * time.Second,
		}

		errChan := make(chan error)
		go func() { errChan <- httpServer.ListenAndServe() }()

		select {
		case err = <-errChan:
			return

		case <-time.After(100 * time.Millisecond):
			break
		}
	}

	return
}

func parseCron(config cronConf, c *routing.Core) (*routing.Cron, error) {
	cron := routing.NewCron()

	interval, err := time.ParseDuration(config.CheckBundles)
	if err != nil {
		return nil, NewConfigError(fmt.Sprintf("Error parsing duration: %v", config.CheckBundles), err)
	}
	if err := cron.Register("pending_bundles", c.CheckPendingBundles, interval); err != nil {
		return nil, NewConfigError("Failed to register pending_bundles at cron", err)
	}

	interval, err = time.ParseDuration(config.CleanStore)
	if err != nil {
		return nil, NewConfigError(fmt.Sprintf("Error parsing duration: %v", config.CleanStore), err)
	}
	if err := cron.Register("clean_store", c.Store.DeleteExpired, interval); err != nil {
		return nil, NewConfigError("Failed to register clean_store at cron", err)
	}

	interval, err = time.ParseDuration(config.CleanID)
	if err != nil {
		return nil, NewConfigError(fmt.Sprintf("Error parsing duration: %v", config.CleanID), err)
	}
	if err := cron.Register("clean_ids", c.IdKeeper.Clean, interval); err != nil {
		return nil, NewConfigError("Failed to register clean_ids at cron", err)
	}

	return cron, nil
}

// parseCore creates the Core based on the given TOML configuration.
func parseCore(filename string) (c *routing.Core, ds *discovery.Manager, err error) {
	var conf tomlConfig
	if _, err = toml.DecodeFile(filename, &conf); err != nil {
		return
	}

	// Logging
	if conf.Logging.Level != "" {
		if lvl, err := log.ParseLevel(conf.Logging.Level); err != nil {
			log.WithFields(log.Fields{
				"level":    conf.Logging.Level,
				"error":    err,
				"provided": "panic,fatal,error,warn,info,debug,trace",
			}).Warn("Failed to set log level. Please select one of the provided ones")
		} else {
			log.SetLevel(lvl)
		}
	}

	log.SetReportCaller(conf.Logging.ReportCaller)

	switch conf.Logging.Format {
	case "", "text":
		log.SetFormatter(&log.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "15:04:05.000",
		})

	case "json":
		log.SetFormatter(&log.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
		})

	default:
		log.Warn("Unknown logging format")
	}

	var discoveryMsgs []discovery.Announcement

	// Core
	if conf.Core.Store == "" {
		err = fmt.Errorf("routing.store is empty")
		return
	}

	log.WithFields(log.Fields{
		"routing": conf.Routing.Algorithm,
	}).Debug("Selected routing algorithm")

	nodeId, nodeErr := bpv7.NewEndpointID(conf.Core.NodeId)
	if nodeErr != nil {
		err = nodeErr
		return
	}

	var signPriv ed25519.PrivateKey = nil
	if conf.Core.SignPriv != "" {
		if signPriv, err = hex.DecodeString(conf.Core.SignPriv); err != nil {
			return
		}
	}

	if c, err = routing.NewCore(conf.Core.Store, nodeId, conf.Core.InspectAllBundles, conf.Routing, signPriv); err != nil {
		return
	}

	cron, err := parseCron(conf.Cron, c)
	if err != nil {
		return
	}
	c.Cron = cron

	// Agents
	if conf.Agents != (agentsConfig{}) {
		if appAgents, appErr := parseAgents(conf.Agents); appErr != nil {
			err = appErr
			return
		} else {
			for _, appAgent := range appAgents {
				c.RegisterApplicationAgent(appAgent)
			}
		}
	}

	// Listen/ConvergenceReceiver
	for _, conv := range conf.Listen {
		if convRec, eid, claType, discoMsg, lErr := parseListen(conv, c.NodeId); lErr != nil {
			err = lErr
			return
		} else {
			c.RegisterCLA(convRec, claType, eid)
			if discoMsg != (discovery.Announcement{}) {
				discoveryMsgs = append(discoveryMsgs, discoMsg)
			}
		}
	}

	// Peer/ConvergenceSender
	for _, conv := range conf.Peer {
		convRec, err := parsePeer(conv, c.NodeId)
		if err != nil {
			log.WithFields(log.Fields{
				"peer":  conv.Endpoint,
				"error": err,
			}).Warn("Failed to establish a connection to a peer")
			continue
		}

		c.RegisterConvergable(convRec)
	}

	// Discovery
	if conf.Discovery.IPv4 || conf.Discovery.IPv6 {
		if conf.Discovery.Interval == 0 {
			conf.Discovery.Interval = 10
		}

		ds, err = discovery.NewManager(
			c.NodeId, c.RegisterConvergable, discoveryMsgs,
			time.Duration(conf.Discovery.Interval)*time.Second, conf.Discovery.IPv4, conf.Discovery.IPv6)
		if err != nil {
			return
		}
	}

	return
}
