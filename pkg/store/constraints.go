// SPDX-FileCopyrightText: 2019, 2020 Alvar Penning
// SPDX-FileCopyrightText: 2025 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import "fmt"

// Constraint is a retention constraint as defined in the subsections RFC9171 Section 5.
type Constraint uint8

const (
	// DispatchPending is assigned to a bundle if its dispatching is pending.
	DispatchPending Constraint = 0

	// ForwardPending is assigned to a bundle if its forwarding is pending.
	ForwardPending Constraint = 1

	// ReassemblyPending is assigned to a fragmented bundle if it is being reassembled.
	// Constraint will be removed once all fragments have been received and the bundle has been reassembled.
	ReassemblyPending Constraint = 2
)

func (c Constraint) String() string {
	switch c {
	case DispatchPending:
		return "dispatch pending"

	case ForwardPending:
		return "forwarding pending"

	case ReassemblyPending:
		return "reassembly pending"

	default:
		return "unknown"
	}
}

// Valid checks if this is a "valid" constraint, that is known to this software
func (c Constraint) Valid() bool {
	return c <= ReassemblyPending
}

type InvalidConstraint Constraint

func (ic *InvalidConstraint) Error() string {
	return fmt.Sprintf("%v is not a valid retention constraint", int(*ic))
}

func NewInvalidConstraint(constraint Constraint) *InvalidConstraint {
	ic := InvalidConstraint(constraint)
	return &ic
}
