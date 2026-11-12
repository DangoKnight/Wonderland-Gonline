package team

import "wonderland-gonline/client/wlo/seui"

func init() {
	seui.RegisterAutomation("team", map[string]any{"petMode": struct{ Slot, Mode int }{}, "dismissPet": 0, "leave": nil, "promote": uint32(0), "kick": uint32(0), "renamePet": struct {
		Slot int
		Name string
	}{}})
}
