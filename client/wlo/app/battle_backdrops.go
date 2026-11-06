package app

import "wonderland-gonline/internal/protocol"

type backdropRuntime struct {
	offered, enabled bool
	location         protocol.BattleBackdropLocation
	pending          bool
}

func (c *Client) backdropPacket(p []byte) bool {
	if len(p) < 2 || p[0] != protocol.CommandHandshake {
		return false
	}
	switch p[1] {
	case protocol.HandshakeBackdropOffer:
		if c.connection.offered || c.connection.enabled {
			return true
		}
		if protocol.IsBattleBackdropCapability(p, p[1]) && !c.backdrops.offered && c.resources != nil && len(c.resources.battleBackdrops) > 0 {
			if c.Net.Send(protocol.BattleBackdropCapability(protocol.HandshakeBackdropAccept)) == nil {
				c.backdrops.offered = true
			}
		}
		return true
	case protocol.HandshakeBackdropEnabled:
		if c.connection.offered || c.connection.enabled {
			return true
		}
		if c.backdrops.offered && protocol.IsBattleBackdropCapability(p, p[1]) {
			c.backdrops.enabled = true
		}
		return true
	case protocol.HandshakeBackdropLocation:
		if l, ok := protocol.ParseBattleBackdropLocation(p); ok && c.backdrops.enabled {
			c.backdrops.location, c.backdrops.pending = l, true
		}
		return true
	}
	return false
}

func (c *Client) resolveBattleBackdrop() {
	defer func() { c.backdrops.pending = false }()
	if !c.backdrops.enabled || !c.backdrops.pending {
		return
	}
	l := c.backdrops.location
	if b, ok := c.resources.battleBackdrops[l.Map]; ok {
		c.battle.state.Background = uint16(b.At(l.X, l.Y))
	}
}
