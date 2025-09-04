package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-co-op/gocron/v2"
	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-go/pkg/application_agent"
	"github.com/dtn7/dtn7-go/pkg/application_agent/rest_agent"
	"github.com/dtn7/dtn7-go/pkg/application_agent/unix_agent"
	"github.com/dtn7/dtn7-go/pkg/cla"
	"github.com/dtn7/dtn7-go/pkg/cla/dummy_cla"
	"github.com/dtn7/dtn7-go/pkg/cla/mtcp"
	"github.com/dtn7/dtn7-go/pkg/cla/quicl"
	"github.com/dtn7/dtn7-go/pkg/discovery"
	"github.com/dtn7/dtn7-go/pkg/id_keeper"
	"github.com/dtn7/dtn7-go/pkg/processing"
	"github.com/dtn7/dtn7-go/pkg/routing"
	"github.com/dtn7/dtn7-go/pkg/store"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("Usage: %s configuration.toml", os.Args[0])
	}

	conf, err := parse(os.Args[1])
	if err != nil {
		log.WithField("error", err).Fatal("Config error")
	}

	log.SetLevel(conf.LogLevel)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02T15:04:05.000",
	})

	processing.SetOwnNodeID(conf.NodeID)

	// Setup Store
	err = store.InitialiseStore(conf.NodeID, conf.Store.Path)
	if err != nil {
		log.WithField("error", err).Fatal("Error initialising store")
	}
	defer store.GetStoreSingleton().Close()

	// Setup IdKeeper
	err = id_keeper.InitializeIdKeeper()
	if err != nil {
		log.WithField("error", err).Fatal("Error initialising IdKeeper")
	}

	// Setup routing
	err = routing.InitialiseAlgorithm(conf.Routing.Algorithm)
	if err != nil {
		log.WithField("error", err).Fatal("Error initialising routing algorithm")
	}

	// Setup CLAs
	err = cla.InitialiseCLAManager(processing.ReceiveBundle, processing.NewPeer, routing.GetAlgorithmSingleton().NotifyPeerDisappeared)
	if err != nil {
		log.WithField("error", err).Fatal("Error initialising CLAs")
	}
	defer cla.GetManagerSingleton().Shutdown()

	for _, lstConf := range conf.Listener {
		var listener cla.ConvergenceListener
		switch lstConf.Type {
		case cla.Dummy:
			listener = dummy_cla.NewDummyListener(lstConf.Address)
		case cla.MTCP:
			srv := mtcp.NewMTCPServer(lstConf.Address, lstConf.EndpointId, cla.GetManagerSingleton().NotifyReceive)
			listener = srv
			cla.GetManagerSingleton().Register(srv)
		case cla.QUICL:
			listener = quicl.NewQUICListener(lstConf.Address, lstConf.EndpointId, cla.GetManagerSingleton().NotifyReceive)
		default:
			log.WithField("Type", lstConf.Type).Fatal("Not valid convergence listener type")
		}

		err = cla.GetManagerSingleton().RegisterListener(listener)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
				"type":  lstConf,
			}).Fatal("Error starting convergence listener")
		}
	}

	// Setup neighbour discovery
	err = discovery.InitialiseManager(conf.NodeID, conf.Discovery, 2*time.Second, true, false, cla.GetManagerSingleton().NotifyReceive)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Error starting discovery manager")
	}
	defer discovery.GetManagerSingleton().Close()

	s, err := gocron.NewScheduler()
	if err != nil {
		log.WithError(err).Fatal("Error initializing cron")
	}
	_, err = s.NewJob(
		gocron.DurationJob(
			conf.Cron.Dispatch,
		),
		gocron.NewTask(
			processing.DispatchPending,
		),
	)
	if err != nil {
		log.WithError(err).Fatal("Error initializing dispatching cronjob")
	}
	s.Start()
	defer s.Shutdown()

	// Setup application agents
	err = application_agent.InitialiseApplicationAgentManager(processing.ReceiveBundle)
	if err != nil {
		log.WithField("error", err).Fatal("Error initialising Application Agent Manager")
	}
	defer application_agent.GetManagerSingleton().Shutdown()

	if conf.Agents.REST.Address != "" {
		restAgent := rest_agent.NewRestAgent("/rest", conf.Agents.REST.Address)
		err = application_agent.GetManagerSingleton().RegisterAgent(restAgent)
		if err != nil {
			log.WithError(err).Fatal("Error registering REST application agent")
		}
	}

	if conf.Agents.UNIX.Socket != "" {
		unixAgent, err := unix_agent.NewUNIXAgent(conf.Agents.UNIX.Socket)
		if err != nil {
			log.WithError(err).Fatal("Error creating UNIX application agent")
		}
		err = application_agent.GetManagerSingleton().RegisterAgent(unixAgent)
		if err != nil {
			log.WithError(err).Fatal("Error registering UNIX application agent")
		}
	}

	// wait for SIGINT or SIGTERM
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	return
}
