package server

import (
	"context"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// settingsCommand ports AC16 and native AC33 desired-state requests. Caller
// holds worldMu. Walk mode remains an acknowledgment, as in C#.
func (s *Server) settingsCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	prefs := c.character.Preferences()
	var ack []byte
	changed := false
	if p[0] == protocol.CommandDirectSettings {
		if len(p) > 3 {
			return protocol.ErrMalformed
		}
		value := byte(game.SettingEnabled)
		if len(p) == 3 {
			value = p[2]
		}
		switch p[1] {
		case protocol.DirectSettingsPK:
			prefs.PKAllowed = value == game.SettingEnabled
			changed = true
		case protocol.DirectSettingsTradeBlock:
			prefs.TradeAllowed = value != game.SettingEnabled
			changed = true
		case protocol.DirectSettingsJoinBlock:
			prefs.PartyInvitesBlocked = value == game.SettingEnabled
			changed = true
		case protocol.DirectSettingsWalkMode:
			c.walkMode = value
		default:
			return ErrUnsupported
		}
		ack = []byte{protocol.CommandDirectSettings, p[1], value}
	} else {
		// Native System sends [33, option, desired value], not a generic toggle
		// key. AC33:2 is a snapshot query only when it has no value. See
		// TSe_SystemForm callback 0x284310 and sender 0x2d1594..0x2d1757.
		if p[1] == protocol.SettingsSnapshot && len(p) == 2 {
			return c.send(prefs.Packet())
		}
		if len(p) != 3 {
			return protocol.ErrMalformed
		}
		value := p[2]
		if p[1] != protocol.SettingsChannels && value != game.SettingEnabled && value != game.SettingDisabled {
			return protocol.ErrMalformed
		}
		switch p[1] {
		case protocol.SettingsPK:
			prefs.PKAllowed = value == game.SettingEnabled
		case protocol.SettingsJoinBattle:
			prefs.JoinAllowed = value == game.SettingEnabled
		case protocol.SettingsTrade:
			prefs.TradeAllowed = value == game.SettingEnabled
		case protocol.SettingsPartyInvites:
			prefs.PartyInvitesBlocked = value == game.SettingDisabled
		case protocol.SettingsChannels:
			if value & ^byte(game.AllChatChannels) != 0 {
				return protocol.ErrMalformed
			}
			prefs.Channels = value
		default:
			return ErrUnsupported
		}
		changed = true
		// Native AC33:1 replies are error notices, not successful setting ACKs.
		// A complete snapshot both confirms the change and refreshes channels.
		ack = prefs.Packet()
	}
	if changed {
		next := c.character.Clone()
		next.Settings = &prefs
		if err := s.commit(ctx, c, next); err != nil {
			return err
		}
		if !prefs.TradeAllowed {
			s.cancelTrade(c)
		}
		if prefs.PartyInvitesBlocked {
			c.partyRequests = nil
		}
	}
	if ack != nil {
		return c.send(ack)
	}
	return nil
}
