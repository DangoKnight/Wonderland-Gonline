package app

import (
	"errors"
	"strings"
	"wonderland-gonline/internal/protocol"
)

const (
	agentPermissionUnknown int32 = iota
	agentPermissionGranted
	agentPermissionDenied
)

const agentPermissionRequired = "The server has not explicitly granted agent/API access. Agent/API gameplay is disabled until the server opts in."

var ErrAgentAccessDenied = errors.New(protocol.AgentAccessDeniedMessage)

// AgentAccessError is safe on transport threads; connection permissions are
// atomic and are rechecked on dispatch as well as when querying session data.
func (c *Client) AgentAccessError() error {
	if !c.agent.serverAttached.Load() || c.agent.serverPermission.Load() == agentPermissionGranted || c.replica != nil {
		return nil
	}
	if c.agent.serverPermission.Load() == agentPermissionDenied {
		return ErrAgentAccessDenied
	}
	return errors.New(agentPermissionRequired)
}

func (c *Client) agentCommandAccess(command string) error {
	// Offline workspace/display management and reconnecting to a different
	// server remain available; neither can mutate a denied server's game state.
	if command == "server.connect" || strings.HasPrefix(command, "workspace.") || strings.HasPrefix(command, "display.") {
		return nil
	}
	return c.AgentAccessError()
}

func (c *Client) agentNetworkAccess(p []byte) error {
	if err := c.AgentAccessError(); err != nil {
		if len(p) > 0 && p[0] == protocol.CommandDiscovery {
			return nil
		}
		if flags, ok := protocol.ParseConnectionCapabilities(p, protocol.HandshakeConnectionAccept); ok && flags&protocol.ConnectionAgentAPI != 0 {
			return nil
		}
		return err
	}
	return nil
}
