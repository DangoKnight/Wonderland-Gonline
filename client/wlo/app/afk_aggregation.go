package app

import (
	"slices"
	"wonderland-gonline/internal/protocol"
)

func (c *Client) decodeAFKUpdate(base protocol.AFKState, p []byte) (protocol.AFKState, error) {
	if len(p) > 1 && p[1] == protocol.AFKDelta {
		if c.connection.features&protocol.ConnectionAFKDelta == 0 {
			return protocol.AFKState{}, protocol.ErrMalformed
		}
		return protocol.ApplyAFKDelta(base, p)
	}
	return protocol.DecodeAFKState(p)
}

func (c *Client) bindAFKStream(p []byte) {
	a := &c.afk
	if !a.active || a.resync || !a.groupRequested || c.afkWorkspace == nil || c.connection.features&protocol.ConnectionAFKAggregation == 0 {
		return
	}
	b, err := protocol.DecodeAFKBinding(p)
	if err != nil {
		return
	}
	if b.Stream != 0 {
		if c.World == nil || c.TeamState == nil || !slices.Contains(b.Members, c.World.Player.ID) {
			return
		}
		for _, id := range b.Members {
			if id != c.World.Player.ID && !slices.Contains(c.TeamState.Members, id) {
				return
			}
		}
	}
	if b.Stream == a.binding.Stream {
		return
	}
	a.binding = b
	a.sequence = 0
	a.latest = nil
	a.commonState = protocol.AFKState{}
	a.privateState = protocol.AFKState{}
}

func (c *Client) mergedAFKState() protocol.AFKState {
	state := c.afk.privateState
	state.Units = append([]protocol.AFKUnit(nil), state.Units...)
	state.Units = append(state.Units, c.afk.commonState.Units...)
	return state
}

func (c *Client) acceptSharedAFK(s protocol.AFKSharedState) bool {
	a := &c.afk
	if !a.active || a.resync || a.binding.Stream == 0 || a.binding.Stream != s.Stream || s.Sequence <= a.sequence {
		return false
	}
	if c.World == nil || c.TeamState == nil {
		return false
	}
	for _, id := range a.binding.Members {
		if id != c.World.Player.ID && !slices.Contains(c.TeamState.Members, id) {
			return false
		}
	}
	a.sequence = s.Sequence
	a.commonState = s.State
	a.commonState.Units = append([]protocol.AFKUnit(nil), s.State.Units...)
	if a.privateState.Map != 0 {
		c.applyAFKState(c.mergedAFKState())
	}
	return true
}

// Called serially after all socket queues are drained. Retaining one latest
// snapshot also covers a subscriber's binding arriving one frame later.
func (w *Workspace) shareAFKState() {
	for _, source := range w.Sessions {
		c := source.Client
		s := c.afk.latest
		if s == nil || !c.afk.active || c.afk.resync || c.afk.binding.Stream != s.Stream {
			continue
		}
		endpoint := c.Net.ServerAddress()
		if endpoint == "" {
			continue
		}
		for _, target := range w.Sessions {
			other := target.Client
			if other == c || other.afkWorkspace != w || !other.afk.groupRequested || other.afk.sequence >= s.Sequence {
				continue
			}
			if other.afk.binding.Stream != c.afk.binding.Stream || other.afk.binding.Carrier != c.afk.binding.Carrier || !slices.Equal(other.afk.binding.Members, c.afk.binding.Members) {
				continue
			}
			if other.Net.ServerAddress() != endpoint {
				continue
			}
			other.acceptSharedAFK(*s)
		}
	}
}
