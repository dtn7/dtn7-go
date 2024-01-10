// SPDX-FileCopyrightText: 2019, 2022 Markus Sommer
// SPDX-FileCopyrightText: 2019, 2020 Alvar Penning
//
// SPDX-License-Identifier: GPL-3.0-or-later

package routing

import (
	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/cla"
	"github.com/dtn7/dtn7-ng/pkg/store"
)

// EpidemicRouting is an implementation of an Algorithm and behaves in a
// flooding-based epidemic way.
type EpidemicRouting struct{}

// NewEpidemicRouting creates a new EpidemicRouting Algorithm interacting
// with the given Core.
func NewEpidemicRouting() *EpidemicRouting {
	log.Debug("Initialised epidemic routing")

	return &EpidemicRouting{}
}

// NotifyNewBundle tells the EpidemicRouting about new bundles.
//
// In our case, the PreviousNodeBlock will be inspected.
func (er *EpidemicRouting) NotifyNewBundle(_ *store.BundleDescriptor) {}

func (er *EpidemicRouting) SelectPeersForForwarding(bp *store.BundleDescriptor) (css []cla.ConvergenceSender) {
	css = filterCLAs(bp, cla.GetManagerSingleton().GetSenders())

	endpoints := make(map[bpv7.EndpointID]bool)
	unique := make([]cla.ConvergenceSender, 0, len(css))
	for _, sender := range css {
		_, present := endpoints[sender.GetPeerEndpointID()]
		if !present {
			endpoints[sender.GetPeerEndpointID()] = true
			unique = append(unique, sender)
		}
	}

	css = unique

	log.WithFields(log.Fields{
		"bundle":        bp.ID,
		"new receivers": css,
	}).Debug("EpidemicRouting selected Convergence Senders for an outgoing bundle")

	return
}

func (_ *EpidemicRouting) NotifyPeerAppeared(_ bpv7.EndpointID) {}

func (_ *EpidemicRouting) NotifyPeerDisappeared(_ bpv7.EndpointID) {}

func (_ *EpidemicRouting) String() string {
	return "epidemic"
}
