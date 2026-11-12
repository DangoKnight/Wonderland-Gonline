package main

import (
	"fmt"
	"net"
	"strconv"
	"wonderland-gonline/internal/wsgateway"
)

const agentNativeLoginPort = 6414 // ClientSocket1 in client/wlo/login/net.go.
const agentNativeServiceCount = 3 // Login, world, status; consecutive native ports.

// Native clients use the fixed login/world/status port family. Keep a headless
// process scoped to -game, including when its asset pack names another server.
func agentGameTarget(requested, configured string) (string, error) {
	if err := wsgateway.DefaultValidateTarget(configured); err != nil {
		return "", err
	}
	if requested == "" || requested == configured {
		return configured, nil
	}
	_, port, err := net.SplitHostPort(requested)
	if err != nil {
		return "", fmt.Errorf("invalid requested game endpoint")
	}
	logical, err := strconv.Atoi(port)
	if err != nil {
		return "", fmt.Errorf("invalid requested game port")
	}
	host, basePort, _ := net.SplitHostPort(configured)
	base, _ := strconv.Atoi(basePort)
	offset := logical - agentNativeLoginPort
	if logical >= base && logical < base+agentNativeServiceCount {
		offset = logical - base
	}
	if offset < 0 || offset >= agentNativeServiceCount {
		return "", fmt.Errorf("requested endpoint is outside configured game services")
	}
	target := net.JoinHostPort(host, strconv.Itoa(base+offset))
	if err := wsgateway.DefaultValidateTarget(target); err != nil {
		return "", err
	}
	return target, nil
}
