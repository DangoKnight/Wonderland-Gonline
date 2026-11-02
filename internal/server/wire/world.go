package wire

import (
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

// FriendEntry is AC14.PackFriendEntry, including saved class and nickname.
// Guild presentation retains its existing empty defaults. Colors retain their words.
func FriendEntry(c game.Character, tab bool, online bool) []byte {
	p, _ := protocol.Builder{}.U32(c.ID).String(c.Name)
	p = p.U8(c.Level).U8(c.RebornByte()).U8(c.Job).U8(c.Element).U8(byte(c.Body)).U8(byte(c.Head)).U16(uint16(c.Color1)).U16(uint16(c.Color1 >> 16)).U16(uint16(c.Color2)).U16(uint16(c.Color2 >> 16))
	p, _ = p.String(c.Nickname)
	if !tab {
		p = p.U8(0)
		flag := byte(0)
		if online {
			flag = 1
		}
		p = p.U8(flag)
	}
	return p
}

func GuildBadge(character uint32, guild *store.GuildInfo) []byte {
	icon, name := uint32(0), ""
	if guild != nil {
		icon, name = guild.Icon, guild.Name
	}
	p, _ := protocol.Builder{protocol.CommandGuild, protocol.GuildBadge}.U32(character).U32(icon).String(name)
	return p
}

func PropFrame(click uint16, state byte) []byte {
	return protocol.Builder{protocol.CommandScene, protocol.SceneActorState}.U16(click).U8(state)
}

// VitalPacket is the AC8 absolute stat update StatPackets sends for one vital.
func VitalPacket(stat byte, value uint32) []byte {
	return protocol.Builder{protocol.CommandStats, protocol.StatsStatUpdate, stat, protocol.StatsValueAbsolute}.U32(value).U32(0)
}

// PeerPackets describes char to another player. Only a login arrival refreshes the sprite.
func PeerPackets(char game.Character, arriving, login bool) ([][]byte, error) {
	appearance, err := char.AppearancePacket(true)
	if err != nil {
		return nil, err
	}
	packets := [][]byte{appearance, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateEquipmentSnapshot}.U32(char.ID).Bytes(char.WornEquipment())}
	if char.Title != 0 {
		packets = append(packets, protocol.Builder{protocol.CommandTitle, protocol.TitleBroadcast}.U32(char.ID).U16(char.Title))
	}
	if arriving {
		packets = append(packets, protocol.Builder{protocol.CommandPresence, protocol.PresenceOnline}.U32(char.ID).U8(protocol.PresenceMapAvailable))
		if login {
			packets = append(packets, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(char.ID).U8(0))
		}
	} else {
		packets = append(packets, char.PositionPacket())
	}
	return packets, nil
}
