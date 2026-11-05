//go:build !(js && wasm)

// Package wsdial gives the browser client a net.Dial replacement. Outside
// wasm it is a plain TCP dialer.
package wsdial

import (
	"net"
	"time"
)

// Dial is net.Dialer.Dial with the client's connection timeout.
func Dial(network, address string) (net.Conn, error) {
	d := net.Dialer{Timeout: 10 * time.Second}
	return d.Dial(network, address)
}
