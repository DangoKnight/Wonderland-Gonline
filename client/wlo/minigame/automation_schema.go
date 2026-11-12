package minigame

import "wonderland-gonline/client/wlo/seui"

func init() {
	for _, kind := range []string{"mole", "dream"} {
		var payload any
		if kind == "mole" {
			payload = 0
		}
		seui.RegisterAutomation(kind, map[string]any{"strike": payload})
	}
	seui.RegisterAutomation("memory", map[string]any{"choose": 0})
	seui.RegisterAutomation("arcade", map[string]any{"requestPlay": nil, "confirmPlay": nil})
	seui.RegisterAutomation("fruitMachine", map[string]any{"play": nil, "leave": nil})
	seui.RegisterAutomation("catch", map[string]any{"move": 0})
	seui.RegisterAutomation("hunter", map[string]any{"shoot": 0})
	seui.RegisterAutomation("pumpkin", map[string]any{"shoot": 0})
	seui.RegisterAutomation("fighter", map[string]any{"left": false, "right": false, "duck": false, "jump": false, "attack": false})
	seui.RegisterAutomation("dodgeball", map[string]any{"move": struct{ X, Y int }{}, "charge": nil, "throw": nil})
}
