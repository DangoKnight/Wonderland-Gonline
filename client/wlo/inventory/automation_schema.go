package inventory

import "wonderland-gonline/client/wlo/seui"

func init() {
	seui.RegisterAutomation("inventory", map[string]any{
		"selectActor": 0, "use": apiSlot{}, "equip": apiSlot{}, "unequip": apiSlot{},
		"move": struct {
			Slot, To, Count int
			Item            uint16
		}{}, "drop": struct {
			Slot, Count int
			Item        uint16
		}{},
		"allocate": [allocationAttributes]uint16{}, "cancelAllocation": nil,
	})
	seui.RegisterAutomation("npcService", map[string]any{"select": struct{ Side, Index int }{}, "page": 0, "confirm": nil, "deposit": nil, "withdraw": nil, "close": nil})
	seui.RegisterAutomation("compound", map[string]any{"select": struct {
		Slot, Index int
		Item        uint16
	}{}, "remove": 0, "tier": 0, "synthesize": nil})
	seui.RegisterAutomation("potential", map[string]any{"target": 0, "confirm": nil, "close": nil})
	seui.RegisterAutomation("giftCard", map[string]any{"select": 0, "confirm": nil, "close": nil})
	seui.RegisterAutomation("trade", map[string]any{"offer": apiSlot{}, "withdraw": 0, "confirm": nil, "accept": nil, "close": nil})
	seui.RegisterAutomation("remote", map[string]any{"configure": RemoteOptions{}, "start": nil, "stop": nil, "advanced": nil})
	seui.RegisterAutomation("remoteAdvanced", map[string]any{"configure": [2]RemoteAdvanced{}})
}
