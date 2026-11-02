package wire

import (
	"math"
	"unicode/utf8"
	"wonderland-gonline/internal/protocol"
)

func QuestProgressMessage(text string) []byte {
	const maxQuestMessageBytes = math.MaxUint8
	// This diagnostic mirrors AC23:57; it does not accept, advance or reward quests.
	raw := []byte(text)
	if len(raw) > maxQuestMessageBytes {
		raw = raw[:maxQuestMessageBytes]
		for !utf8.Valid(raw) {
			raw = raw[:len(raw)-1]
		}
	}
	p, _ := protocol.Builder{protocol.CommandInventory, protocol.InventoryMessage, 0}.String(string(raw))
	return p
}

// HeadBanner is Player.SendHeadBanner: AC2:16 with raw, unprefixed ASCII text.
func HeadBanner(text string) []byte {
	return protocol.Builder{protocol.CommandChat, protocol.ChatHeadBanner}.U32(0).Bytes([]byte(text))
}

// SystemLine is AC23:57 with a length-prefixed message.
func SystemLine(text string) []byte {
	p, _ := protocol.Builder{protocol.CommandInventory, protocol.InventoryMessage, 0}.String(text)
	return p
}
