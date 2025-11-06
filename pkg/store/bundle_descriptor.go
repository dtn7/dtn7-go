package store

import (
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
)

type BundleDescriptor struct {
	ID          bpv7.BundleID
	Source      bpv7.EndpointID
	Destination bpv7.EndpointID
	ReportTo    bpv7.EndpointID

	Bundle *bpv7.Bundle

	// Node IDs of peers which already have this bundle
	// By tracking these, we can avoid wasting bandwidth by sending bundles to nodes which already have them.
	AlreadySentTo []bpv7.EndpointID

	// RetentionConstraints as defined by RFC9171 Section 5, see constraints.go for possible values
	RetentionConstraints []Constraint
	// bundle's ID in string-form. Used as the database primary-key. Return-value of ID.String()
	IDString string `badgerhold:"key"`
	// should this bundle be retained, i.e. protected from deletion
	// bundle's with constraints are also currently being processed
	Retain bool
	// should this bundle be dispatched?
	Dispatch bool
	// TTL after which the bundle will be deleted - assuming Retain == false
	Expires time.Time
	// filename of the serialised bundle on-disk
	SerialisedFileName string
}

// Load loads the entire bundle from disk
// Since bundles can be rather arbitrarily large, this can be very expensive and should only be done when necessary.
// Once a bundle has been loaded, it is stored in the BundleDescriptor's "Bundle" field,
// so further calls should be a lot faster.
func (bd *BundleDescriptor) Load() (*bpv7.Bundle, error) {
	if bd.Bundle != nil {
		return bd.Bundle, nil
	}
	bndle, err := GetStoreSingleton().loadEntireBundle(bd.SerialisedFileName)
	if err != nil {
		return nil, err
	}
	// TODO: make caching optional to conserve memory
	bd.Bundle = bndle
	return bndle, nil
}

// GetAlreadySent gets the list of EndpointIDs which we know to already have received the bundle.
func (bd *BundleDescriptor) GetAlreadySent() []bpv7.EndpointID {
	// TODO: refresh current state from db
	// TODO: give better name, since we might also know from receiving the bundle from someone else
	return bd.AlreadySentTo
}

// AddAlreadySent adds EndpointIDs to this bundle's list of known recipients.
func (bd *BundleDescriptor) AddAlreadySent(peers ...bpv7.EndpointID) {
	bd.AlreadySentTo = append(bd.AlreadySentTo, peers...)
	err := GetStoreSingleton().updateBundleMetadata(bd)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bd.IDString,
			"error":  err,
		}).Error("Error syncing bundle metadata")
	} else {
		log.WithFields(log.Fields{
			"bundle": bd.IDString,
			"peers":  peers,
		}).Debug("Peers added to already sent")
	}
}

// AddConstraint adds a Constraint to this bundle and checks if it should be retained/dispatched.
// Changes are synced to disk.
func (bd *BundleDescriptor) AddConstraint(constraint Constraint) error {
	// check if value is valid constraint
	if constraint < DispatchPending || constraint > ReassemblyPending {
		return NewInvalidConstraint(constraint)
	}

	bd.RetentionConstraints = append(bd.RetentionConstraints, constraint)
	bd.Retain = true
	bd.Dispatch = constraint != ForwardPending
	return GetStoreSingleton().updateBundleMetadata(bd)
}

// RemoveConstraint removes a Constraint from this bundle and checks if it should be retained/dispatched.
// Changes are synced to disk.
func (bd *BundleDescriptor) RemoveConstraint(constraint Constraint) error {
	constraints := make([]Constraint, 0, len(bd.RetentionConstraints))
	for _, existingConstraint := range bd.RetentionConstraints {
		if existingConstraint != constraint {
			constraints = append(constraints, existingConstraint)
		}
	}
	bd.RetentionConstraints = constraints
	bd.Retain = len(bd.RetentionConstraints) > 0
	bd.Dispatch = constraint == ForwardPending
	return GetStoreSingleton().updateBundleMetadata(bd)
}

// ResetConstraints removes all Constraints from this bundle.
// Changes are synced to disk.
func (bd *BundleDescriptor) ResetConstraints() error {
	bd.RetentionConstraints = make([]Constraint, 0)
	bd.Retain = false
	bd.Dispatch = true
	return GetStoreSingleton().updateBundleMetadata(bd)
}

func (bd *BundleDescriptor) String() string {
	return bd.ID.String()
}
