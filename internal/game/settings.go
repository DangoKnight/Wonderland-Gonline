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

// Packet is ClientSettings.ToArray: 1 means on and 2 means off.
func (s ClientSettings) Packet() []byte {
	flag := func(v bool) byte {
		if v {
			return SettingEnabled
		}
		return SettingDisabled
	}
	return []byte{protocol.CommandSettings, protocol.SettingsSnapshot, flag(s.PKAllowed), flag(s.JoinAllowed), flag(s.TradeAllowed), s.Channels}
}
