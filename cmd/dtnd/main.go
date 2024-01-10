package main

import (
	"os"

	"github.com/dtn7/dtn7-ng/pkg/cla/dummy_cla"
	"github.com/dtn7/dtn7-ng/pkg/cla/mtcp"
	"github.com/dtn7/dtn7-ng/pkg/cla/quicl"

	"github.com/dtn7/dtn7-ng/pkg/processing"
	"github.com/dtn7/dtn7-ng/pkg/routing"

	"github.com/dtn7/dtn7-ng/pkg/cla"
	"github.com/dtn7/dtn7-ng/pkg/store"

	log "github.com/sirupsen/logrus"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("Usage: %s configuration.toml", os.Args[0])
	}

	conf, err := parse(os.Args[1])
	if err != nil {
		log.WithField("error", err).Fatal("Config error")
	}

	err = store.InitialiseStore(conf.NodeID, conf.Store.Path)
	if err != nil {
		log.WithField("error", err).Fatal("Error initialising store")
	}
	defer store.GetStoreSingleton().Close()

	err = routing.InitialiseAlgorithm(conf.Routing.Algorithm)
	if err != nil {
		log.WithField("error", err).Fatal("Error initialising routing algorithm")
	}

	err = cla.InitialiseCLAManager(processing.ReceiveBundle, routing.GetAlgorithmSingleton().NotifyPeerAppeared, routing.GetAlgorithmSingleton().NotifyPeerDisappeared)
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
			srv := mtcp.NewMTCPServer(lstConf.Address, lstConf.EndpointId)
			listener = srv
			cla.GetManagerSingleton().Register(srv)
		case cla.QUICL:
			listener = quicl.NewQUICListener(lstConf.Address, lstConf.EndpointId)
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
}
