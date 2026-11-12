package hud

import "wonderland-gonline/client/wlo/seui"

func init() {
	seui.RegisterAutomation("social", map[string]any{"tab": 0, "page": 0, "selectFriend": uint32(0), "mailFriend": uint32(0), "deleteSelected": nil, "mailSelected": nil, "profile": struct {
		Nickname         string
		Blood            byte
		Year, Month, Day int
	}{}, "emote": byte(0), "pose": 0, "shortcut": struct {
		Index int
		Slot  EmoteSlot
	}{}, "phrase": struct {
		Index int
		Text  string
	}{}})
	seui.RegisterAutomation("mailbox", map[string]any{"tab": 0, "page": 0, "open": 0, "delete": 0, "compose": nil})
}
