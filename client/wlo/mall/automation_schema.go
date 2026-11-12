package mall

import "wonderland-gonline/client/wlo/seui"

func init() {
	seui.RegisterAutomation("mall", map[string]any{
		"currency": false, "category": 0, "page": 0, "add": uint16(0), "quantity": struct{ Index, Quantity int }{}, "remove": 0,
		"cart": nil, "catalog": nil, "checkout": nil, "contents": uint16(0), "tryOn": uint16(0), "resetFitting": nil,
		"forgeSelect": struct {
			Slot int
			Item uint16
		}{}, "forge": nil, "game": 0,
	})
}
