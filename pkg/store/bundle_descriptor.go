package store

import (
	"time"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	log "github.com/sirupsen/logrus"
)

type BundleDescriptor struct {
	ID          bpv7.BundleID
	Source      bpv7.EndpointID
	Destination bpv7.EndpointID
	ReportTo    bpv7.EndpointID

	Bundle *bpv7.Bundle

	// node IDs of peers which already have this bundle
	AlreadySentTo []bpv7.EndpointID

	// RetentionConstraints as defined by RFC9171 Section 5, see constraints.go for possible types
	RetentionConstraints []Constraint
	// bundle's ID in string-form. Used as the database primary-key. Return-value of ID.String()
	IDString string `badgerhold:"key"`
	// should this bundle be retained, i.e. protected from deletion
	// bundle's with constraints are also currently being processed
	Retain bool
	// TTL after which the bundle will be deleted - assuming Retain == false
	Expires time.Time
	// filename of the serialised bundle on-disk
	SerialisedFileName string
}

func (bd *BundleDescriptor) Load() (bpv7.Bundle, error) {
	if bd.Bundle != nil {
		return *bd.Bundle, nil
	}
	bndle, err := GetStoreSingleton().loadEntireBundle(bd.SerialisedFileName)
	if err != nil {
		return bpv7.Bundle{}, err
	}
	bd.Bundle = bndle
	return *bndle, nil
}

func (bd *BundleDescriptor) GetAlreadySent() []bpv7.EndpointID {
	// TODO: refresh current state from db
	return bd.AlreadySentTo
}

func (bd *BundleDescriptor) AddAlreadySent(peers ...bpv7.EndpointID) {
	bd.AlreadySentTo = append(bd.AlreadySentTo, peers...)
	err := GetStoreSingleton().updateBundleMetadata(bd)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bd.IDString,
			"error":  err,
		}).Error("Error syncing bundle metadata")
	}
}

func (bd *BundleDescriptor) AddConstraint(constraint Constraint) error {
	// check if value is valid constraint
	if constraint < DispatchPending || constraint > ReassemblyPending {
		return NewInvalidConstraint(constraint)
	}

	bd.RetentionConstraints = append(bd.RetentionConstraints, constraint)
	bd.Retain = true
	return GetStoreSingleton().updateBundleMetadata(bd)
}

func (bd *BundleDescriptor) RemoveConstraint(constraint Constraint) error {
	constraints := make([]Constraint, 0, len(bd.RetentionConstraints))
	for _, existingConstraint := range bd.RetentionConstraints {
		if existingConstraint != constraint {
			constraints = append(constraints, existingConstraint)
		}
	}
	bd.RetentionConstraints = constraints
	bd.Retain = len(bd.RetentionConstraints) > 0
	return GetStoreSingleton().updateBundleMetadata(bd)
}

func (bd *BundleDescriptor) ResetConstraints() error {
	bd.RetentionConstraints = make([]Constraint, 0)
	bd.Retain = false
	return GetStoreSingleton().updateBundleMetadata(bd)
}
