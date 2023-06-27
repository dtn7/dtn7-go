package store

import (
	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	log "github.com/sirupsen/logrus"
	"time"
)

type BundleDescriptor struct {
	ID          bpv7.BundleID
	Source      bpv7.EndpointID
	Destination bpv7.EndpointID
	ReportTo    bpv7.EndpointID

	// node IDs of peers which already have this bundle
	alreadySentTo []bpv7.EndpointID

	bundle *bpv7.Bundle

	// retentionConstraints as defined by RFC9171 Section 5, see constraints.go for possible types
	retentionConstraints []Constraint
	// bundle's ID in string-form. Used as the database primary-key. Return-value of ID.String()
	idString string `badgerhold:"key"`
	// should this bundle be retained, i.e. protected from deletion
	retain bool `badgerholdIndex:"retain"`
	// is this bundle currently being processed
	processing bool `badgerholdIndex:"processing"`
	// TTL after which the bundle will be deleted - assuming retain == false
	expires time.Time `badgerholdIndex:"expires"`
	// filename of the serialised bundle on-disk
	serialisedFileName string
}

func (bd *BundleDescriptor) Load() (bpv7.Bundle, error) {
	if bd.bundle != nil {
		return *bd.bundle, nil
	}

	bndle, err := DTNStore.loadEntireBundle(bd.serialisedFileName)
	if err != nil {
		return bpv7.Bundle{}, err
	}
	bd.bundle = bndle
	return *bd.bundle, nil
}

func (bd *BundleDescriptor) GetAlreadySent() []bpv7.EndpointID {
	// TODO: refresh current state from db
	return bd.alreadySentTo
}

func (bd *BundleDescriptor) AddAlreadySent(peers ...bpv7.EndpointID) {
	bd.alreadySentTo = append(bd.alreadySentTo, peers...)
	err := DTNStore.updateBundleMetadata(bd)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bd.idString,
			"error":  err,
		}).Error("Error syncing bundle metadata")
	}
}

func (bd *BundleDescriptor) AddConstraint(constraint Constraint) {
	bd.retentionConstraints = append(bd.retentionConstraints, constraint)
	bd.retain = true
	if constraint == ForwardPending {
		bd.processing = true
	}
	err := DTNStore.updateBundleMetadata(bd)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bd.idString,
			"error":  err,
		}).Error("Error syncing bundle metadata")
	}
}

func (bd *BundleDescriptor) RemoveConstraint(constraint Constraint) {
	constraints := make([]Constraint, 0, len(bd.retentionConstraints))
	for _, existingConstraint := range bd.retentionConstraints {
		if existingConstraint != constraint {
			constraints = append(constraints, existingConstraint)
		}
	}
	bd.retentionConstraints = constraints
	bd.retain = len(bd.retentionConstraints) > 0
	bd.processing = false
	for _, constraint := range bd.retentionConstraints {
		if constraint == ForwardPending {
			bd.processing = true
		}
	}
	err := DTNStore.updateBundleMetadata(bd)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bd.idString,
			"error":  err,
		}).Error("Error syncing bundle metadata")
	}
}
