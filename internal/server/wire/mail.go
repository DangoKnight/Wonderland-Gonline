package wire

import (
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

// TextMailPacket is AC14:1 as aLogin reads it (dispatcher case 0xe/1 at
// 0x2e29cc): sender, OLE Automation date, then the text as the rest of the
// packet with no length byte (the reference server's PackString prefix
// showed as the letter's first character). The 255-byte limit is kept.
// UTC timestamps make replay independent of the server's timezone. Input
// text is raw Big5-compatible bytes, not UTF-8.
func TextMailPacket(message store.TextMail) ([]byte, error) {
	if len(message.Content) > protocol.MaxStringBytes {
		return nil, protocol.ErrMalformed
	}
	date := float64(message.SentAtMillis)/millisecondsPerDay + oleAutomationUnixEpochDays
	return protocol.Builder{protocol.CommandFriends, protocol.FriendsTextMail}.U32(message.SenderID).F64(date).Bytes(message.Content), nil
}

const (
	oleAutomationUnixEpochDays = 25569
	millisecondsPerDay         = 24 * 60 * 60 * 1000
)
