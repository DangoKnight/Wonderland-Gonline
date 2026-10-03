package server

import (
	"context"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// settingsCommand ports AC16 and AC33. Caller holds worldMu. Walk mode and
// team-follow are native acknowledgments; neither changes movement in C#.
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
			prefs.JoinAllowed = value != game.SettingEnabled
			changed = true
		case protocol.DirectSettingsWalkMode:
			c.walkMode = value
		default:
			return ErrUnsupported
		}
		ack = []byte{protocol.CommandDirectSettings, p[1], value}
	} else {
		switch p[1] {
		case protocol.SettingsToggle:
			if len(p) < 3 {
				return protocol.ErrMalformed
			}
			setting := p[2]
			size := 3
			if setting == protocol.SettingKeyChannels {
				size = 4
			}
			if len(p) != size {
				return protocol.ErrMalformed
			}
			flag := true
			switch setting {
			case protocol.SettingKeyPK:
				prefs.PKAllowed = !prefs.PKAllowed
				flag = prefs.PKAllowed
			case protocol.SettingKeyJoin:
				prefs.JoinAllowed = !prefs.JoinAllowed
				flag = prefs.JoinAllowed
			case protocol.SettingKeyChannels:
				prefs.Channels = p[3]
			case protocol.SettingKeyTrade:
				prefs.TradeAllowed = !prefs.TradeAllowed
				flag = prefs.TradeAllowed
			default:
				return ErrUnsupported
			}
			changed = true
			if setting != protocol.SettingKeyChannels {
				value := byte(game.SettingDisabled)
				if flag {
					value = game.SettingEnabled
				}
				ack = []byte{protocol.CommandSettings, protocol.SettingsToggle, setting, value}
			}
		case protocol.SettingsSnapshot:
			if len(p) != 2 {
				return protocol.ErrMalformed
			}
			return c.send(prefs.Packet())
		case protocol.SettingsFollow:
			if len(p) > 3 {
				return protocol.ErrMalformed
			}
			value := byte(1)
			if len(p) == 3 {
				value = p[2]
			}
			return c.send([]byte{protocol.CommandSettings, protocol.SettingsFollow, value})
		default:
			return ErrUnsupported
		}
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
		if !prefs.JoinAllowed {
			c.partyRequests = nil
		}
	}
	if ack != nil {
		return c.send(ack)
	}
	return nil
}
