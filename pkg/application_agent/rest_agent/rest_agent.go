// SPDX-FileCopyrightText: 2020 Alvar Penning
// SPDX-FileCopyrightText: 2023, 2025 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package rest_agent provides a RESTful Application Agent for simple bundle dispatching.
//
// A client must register itself for some endpoint ID at first. After that, bundles sent to this endpoint can be
// retrieved or new bundles can be sent. For sending, bundles can be created by calling the BundleBuilder. Finally,
// a client should unregister itself.
//
// This is all done by HTTP POSTing JSON objects. Their structure is described in `messages.go` by the types
// with the `Rest` prefix in their names.
//
// A possible conversation follows as an example.
//
//	// 1. Registration of our client, POST to /register
//	// -> {"endpoint_id":"dtn://foo/bar"}
//	// <- {"error":"","uuid":"75be76e2-23fc-da0e-eeb8-4773f84a9d2f"}
//
//	// 2. Fetching bundles for our client, POST to /fetch
//	//    There will be to answers, one with new bundles and one without
//	// -> {"uuid":"75be76e2-23fc-da0e-eeb8-4773f84a9d2f"}
//	// <- {"error":"","bundles":[
//	//      {
//	//        "primaryBlock": {
//	//          "bundleControlFlags":null,
//	//          "destination":"dtn://foo/bar",
//	//          "source":"dtn://sender/",
//	//          "reportTo":"dtn://sender/",
//	//          "creationTimestamp":{"date":"2020-04-14 14:32:06","sequenceNo":0},
//	//          "lifetime":86400000000
//	//        },
//	//        "canonicalBlocks": [
//	//          {"blockNumber":1,"blockTypeCode":1,"blockControlFlags":null,"data":"S2hlbGxvIHdvcmxk"}
//	//        ]
//	//      }
//	//    ]}
//	// <- {"error":"","bundles":[]}
//
//	// 3. Create and dispatch a new bundle, POST to /build
//	// -> {
//	//      "uuid": "75be76e2-23fc-da0e-eeb8-4773f84a9d2f",
//	//      "arguments": {
//	//        "destination": "dtn://dst/",
//	//        "source": "dtn://foo/bar",
//	//        "creation_timestamp_now": 1,
//	//        "lifetime": "24h",
//	//        "payload_block": "hello world"
//	//      }
//	//    }
//	// <- {"error":""}
//
//	// 4. Unregister the client, POST to /unregister
//	// -> {"uuid":"75be76e2-23fc-da0e-eeb8-4773f84a9d2f"}
//	// <- {"error":""}
package rest_agent

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-go/pkg/application_agent"
	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/store"
)

type RestAgent struct {
	router        *mux.Router
	listenAddress string

	// map UUIDs to EIDs and received bundles
	clients   sync.Map // uuid[string] -> bpv7.EndpointID
	mailboxes *application_agent.MailboxBank
}

func NewRestAgent(prefix, listenAddress string) (ra *RestAgent) {
	r := mux.NewRouter()
	restRouter := r.PathPrefix(prefix).Subrouter()

	ra = &RestAgent{
		router:        restRouter,
		listenAddress: listenAddress,
		mailboxes:     application_agent.NewMailboxBank(),
	}

	ra.router.HandleFunc("/register", ra.handleRegister).Methods(http.MethodPost)
	ra.router.HandleFunc("/unregister", ra.handleUnregister).Methods(http.MethodPost)
	ra.router.HandleFunc("/fetch", ra.handleFetch).Methods(http.MethodPost)
	ra.router.HandleFunc("/build", ra.handleBuild).Methods(http.MethodPost)

	return ra
}

func (ra *RestAgent) Name() string {
	return fmt.Sprintf("RestAgent(%v)", ra.listenAddress)
}

func (ra *RestAgent) Start() error {
	httpServer := &http.Server{
		Addr:              ra.listenAddress,
		Handler:           ra.router,
		ReadHeaderTimeout: 60 * time.Second,
	}

	go httpServer.ListenAndServe()

	return nil
}

func (ra *RestAgent) Shutdown() {

}

// Deliver checks incoming BundleMessages and puts them in a mailbox.
func (ra *RestAgent) Deliver(bundleDescriptor *store.BundleDescriptor) error {
	return ra.mailboxes.Deliver(bundleDescriptor)
}

// randomUuid to be used for authentication. UUID not compliant with RFC 4122.
func (_ *RestAgent) randomUuid() (uuid string, err error) {
	uuidBytes := make([]byte, 16)
	if _, err = rand.Read(uuidBytes); err == nil {
		uuid = fmt.Sprintf("%x-%x-%x-%x-%x",
			uuidBytes[0:4], uuidBytes[4:6], uuidBytes[6:8], uuidBytes[8:10], uuidBytes[10:16])
	}
	return
}

// handleRegister processes /register POST requests.
func (ra *RestAgent) handleRegister(w http.ResponseWriter, r *http.Request) {
	var (
		registerRequest  RestRegisterRequest
		registerResponse RestRegisterResponse
	)

	if jsonErr := json.NewDecoder(r.Body).Decode(&registerRequest); jsonErr != nil {
		registerResponse.Error = jsonErr.Error()
	} else if eid, eidErr := bpv7.NewEndpointID(registerRequest.EndpointId); eidErr != nil {
		registerResponse.Error = eidErr.Error()
	} else if uuid, uuidErr := ra.randomUuid(); uuidErr != nil {
		registerResponse.Error = uuidErr.Error()
	} else {
		ra.mailboxes.Register(eid)
		ra.clients.Store(uuid, eid)
		registerResponse.UUID = uuid
	}

	log.WithFields(log.Fields{
		"request":  registerRequest,
		"response": registerResponse,
	}).Info("Processing REST registration")

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(registerResponse); err != nil {
		log.WithError(err).Warn("Failed to write REST registration response")
	}
}

// handleUnregister processes /unregister POST requests.
func (ra *RestAgent) handleUnregister(w http.ResponseWriter, r *http.Request) {
	var (
		unregisterRequest  RestUnregisterRequest
		unregisterResponse RestUnregisterResponse
	)

	if jsonErr := json.NewDecoder(r.Body).Decode(&unregisterRequest); jsonErr != nil {
		log.WithError(jsonErr).Warn("Failed to parse REST unregistration request")
	} else {
		log.WithField("uuid", unregisterRequest.UUID).Info("Unregister REST client")
		if eid, ok := ra.clients.Load(unregisterRequest.UUID); ok {
			if err := ra.mailboxes.Unregister(eid.(bpv7.EndpointID)); err != nil {
				log.WithFields(log.Fields{
					"uuid":  unregisterRequest.UUID,
					"eid":   eid,
					"error": err,
				}).Debug("Error unregistering eid")
				unregisterResponse.Error = err.Error()
			}
		} else {
			log.WithField("uuid", unregisterRequest.UUID).Debug("REST client does not know client")
			unregisterResponse.Error = "REST client does not know client"
		}

		ra.clients.Delete(unregisterRequest.UUID)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(unregisterResponse); err != nil {
		log.WithError(err).Warn("Failed to write REST unregistration response")
	}
}

// handleFetch returns the bundles from some client's inbox, called by /fetch.
func (ra *RestAgent) handleFetch(w http.ResponseWriter, r *http.Request) {
	var (
		fetchRequest  RestFetchRequest
		fetchResponse RestFetchResponse
	)

	if jsonErr := json.NewDecoder(r.Body).Decode(&fetchRequest); jsonErr != nil {
		log.WithError(jsonErr).Warn("Failed to parse REST fetch request")
		fetchResponse.Error = jsonErr.Error()
	} else if eid, ok := ra.clients.Load(fetchRequest.UUID); ok {
		log.WithFields(log.Fields{
			"uuid": fetchRequest.UUID,
			"eid":  eid,
		}).Info("REST client fetches bundles")

		if mailbox, err := ra.mailboxes.GetMailbox(eid.(bpv7.EndpointID)); err == nil {
			if bundles, err := mailbox.GetAll(true); err == nil {
				fetchResponse.Bundles = make([]bpv7.Bundle, 0, len(bundles))
				for _, bundle := range bundles {
					fetchResponse.Bundles = append(fetchResponse.Bundles, *bundle)
				}
			} else {
				log.WithFields(log.Fields{
					"uuid": fetchRequest.UUID,
					"eid":  eid,
					"err":  err,
				}).Debug("Failure fetching bundles")
				fetchResponse.Error = err.Error()
			}
		} else {
			log.WithFields(log.Fields{
				"uuid": fetchRequest.UUID,
				"eid":  eid,
			}).Debug("No mailbox registered for this eid")
			fetchResponse.Error = "No mailbox registered for this eid"
		}
	} else {
		log.WithField("uuid", fetchRequest.UUID).Debug("REST client does not know client")
		fetchResponse.Error = "REST client does not know client"
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(fetchResponse); err != nil {
		log.WithError(err).Warn("Failed to write REST fetch response")
	}
}

// handleBuild creates and dispatches a new bundle, called by /build.
func (ra *RestAgent) handleBuild(w http.ResponseWriter, r *http.Request) {
	var (
		buildRequest  RestBuildRequest
		buildResponse RestBuildResponse
	)

	if jsonErr := json.NewDecoder(r.Body).Decode(&buildRequest); jsonErr != nil {
		log.WithError(jsonErr).Warn("Failed to parse REST build request")
		buildResponse.Error = jsonErr.Error()
	} else if eid, ok := ra.clients.Load(buildRequest.UUID); !ok {
		log.WithField("uuid", buildRequest.UUID).Debug("REST client cannot build for unknown UUID")
		buildResponse.Error = "Invalid UUID"
	} else if b, bErr := bpv7.BuildFromMap(buildRequest.Args); bErr != nil {
		log.WithError(bErr).WithField("uuid", buildRequest.UUID).Warn("REST client failed to build a bundle")
		buildResponse.Error = bErr.Error()
	} else if pb := b.PrimaryBlock; pb.SourceNode != eid && pb.ReportTo != eid {
		msg := "REST client's endpoint is neither the source nor the report_to field"
		log.WithFields(log.Fields{
			"uuid":     buildRequest.UUID,
			"endpoint": eid,
			"bundle":   b.ID().String(),
		}).Warn(msg)
		buildResponse.Error = msg
	} else {
		log.WithFields(log.Fields{
			"uuid":   buildRequest.UUID,
			"bundle": b.ID().String(),
		}).Info("REST client sent bundle")
		application_agent.GetManagerSingleton().Send(b)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(buildResponse); err != nil {
		log.WithError(err).Warn("Failed to write REST build response")
	}
}

func (ra *RestAgent) Endpoints() (eids []bpv7.EndpointID) {
	ra.clients.Range(func(_, v interface{}) bool {
		eids = append(eids, v.(bpv7.EndpointID))
		return false
	})
	return
}
