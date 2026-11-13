//go:build !windows && !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package main

import (
	"errors"
	"os"
)

func lockReservation(string) (*os.File, error) {
	return nil, errors.New("controller reservations are unsupported on this platform")
}
