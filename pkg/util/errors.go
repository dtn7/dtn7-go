package util

import "fmt"

type AlreadyInitialised string

func (err *AlreadyInitialised) Error() string {
	return fmt.Sprintf("%v was already initialised", err)
}

func NewAlreadyInitialisedError(name string) *AlreadyInitialised {
	err := AlreadyInitialised(name)
	return &err
}
