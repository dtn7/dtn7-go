package main

import (
	"os"

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

	err = cla.InitialiseCLAManager(conf.Listener, processing.ReceiveBundle, routing.GetAlgorithmSingleton().NotifyPeerAppeared, routing.GetAlgorithmSingleton().NotifyPeerDisappeared)
	if err != nil {
		log.WithField("error", err).Fatal("Error initialising CLAs")
	}
	defer cla.GetManagerSingleton().Shutdown()

	err = routing.InitialiseAlgorithm(conf.Routing.Algorithm)
	if err != nil {
		log.WithField("error", err).Fatal("Error initialising routing algorithm")
	}
}
