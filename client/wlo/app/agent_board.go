package app

import (
	"encoding/json"
	"fmt"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/protocol"
)

func init() {
	seui.RegisterAutomation("board", map[string]any{"page": 0, "read": uint32(0), "compose": nil, "close": nil})
}

func (b *messageBoardForm) AutomationInfo() seui.AutomationInfo {
	entries := []map[string]any{}
	for _, e := range b.entries {
		entries = append(entries, map[string]any{"id": e.id, "author": e.author, "name": seui.DecodeText(e.name), "title": seui.DecodeText(e.title), "date": e.date})
	}
	return seui.AutomationInfo{Kind: "board", Actions: []string{"page", "read", "compose", "close"}, Value: map[string]any{"entries": entries, "page": b.page, "total": b.total, "pending": !b.pending.IsZero(), "selected": b.selected.id}}
}
func (b *messageBoardForm) AutomationInvoke(action string, raw json.RawMessage) error {
	if !b.pending.IsZero() {
		return fmt.Errorf("board request pending")
	}
	switch action {
	case "page":
		var page int
		if seui.DecodeAutomation(raw, &page) != nil || page < 1 || uint32(page) > b.pages() {
			return fmt.Errorf("invalid board page")
		}
		b.requestPage(byte(page))
	case "read":
		var id uint32
		if seui.DecodeAutomation(raw, &id) != nil {
			return fmt.Errorf("message ID required")
		}
		found := false
		for _, e := range b.entries {
			if e.id == id {
				b.selected = e
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("message no longer on current page")
		}
		b.pendingRead = id
		b.pending = b.c.Now()
		b.c.Net.Send(b.entryRequest(protocol.MessageBoardRead))
	case "compose":
		b.compose.title.SetText(nil)
		b.compose.body.SetText(nil)
		b.compose.Show()
	case "close":
		b.c.closeMessageBoard()
	default:
		return fmt.Errorf("unsupported board action")
	}
	return nil
}
