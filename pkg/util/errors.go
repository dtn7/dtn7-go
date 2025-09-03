// SPDX-FileCopyrightText: 2023 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package util provides miscellaneous types and functions which may be useful in multiple places.
package util

import "fmt"

type AlreadyInitialised string

func (err *AlreadyInitialised) Error() string {
	return fmt.Sprintf("%s was already initialised", string(*err))
}

func NewAlreadyInitialisedError(name string) *AlreadyInitialised {
	err := AlreadyInitialised(name)
	return &err
}
