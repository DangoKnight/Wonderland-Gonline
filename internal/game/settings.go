package game

import "wonderland-go/internal/protocol"

// ClientSettings mirrors ClientSettings in inGameSettings.cs. A nil settings
// record on an older character uses the native constructor defaults.
// Channel bits match ChannelCodeType in inGameSettings.cs.
const (
	ChatChannelLocal   = 1 << 0
	ChatChannelWhisper = 1 << 1
	ChatChannelTeam    = 1 << 2
	ChatChannelGuild   = 1 << 3
	ChatChannelWorld   = 1 << 4
	AllChatChannels    = ChatChannelLocal | ChatChannelWorld | ChatChannelTeam | ChatChannelGuild | ChatChannelWhisper
	SettingEnabled     = 1
	SettingDisabled    = 2
)

type ClientSettings struct {
	PKAllowed    bool `json:"pk_allowed"`
	JoinAllowed  bool `json:"join_allowed"`
	TradeAllowed bool `json:"trade_allowed"`
	Channels     byte `json:"channels"`
}

func (c Character) Preferences() ClientSettings {
	if c.Settings != nil {
		return *c.Settings
	}
	return ClientSettings{PKAllowed: true, JoinAllowed: true, TradeAllowed: true, Channels: AllChatChannels}
}

// The native AC33:2 decoder at 0x2ea812..0x2ea872 reads four option
// values, a channel mask, then one byte passed to a no-op. The C# ToArray
// omitted option 10 and the tail, so native Local chat read a missing mask.
// Option 10's gameplay meaning is unresolved; retain the native constructor's
// enabled default (FUN_00282990). Do not mistake this slot for the channel mask.
const (
	nativeSettingsOption10Default     = SettingEnabled
	nativeSettingsSnapshotTailDefault = 0
)

// Packet sends the native eight-byte settings snapshot: 1 is on, 2 is off.
func (s ClientSettings) Packet() []byte {
	flag := func(v bool) byte {
		if v {
			return SettingEnabled
		}
		return SettingDisabled
	}
	return []byte{protocol.CommandSettings, protocol.SettingsSnapshot, flag(s.PKAllowed), flag(s.JoinAllowed), flag(s.TradeAllowed), nativeSettingsOption10Default, s.Channels, nativeSettingsSnapshotTailDefault}
}
