package team

import (
	"encoding/json"
	"fmt"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
)

func (f *Form) AutomationInfo() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "team", Actions: []string{"petMode", "dismissPet", "leave", "promote", "kick", "renamePet"}, Value: map[string]any{"party": f.State, "pets": f.Inventory.State.Pets}}
}
func (f *Form) AutomationInvoke(action string, raw json.RawMessage) error {
	if !f.Visible || f.Blocked() || action != "petMode" && !f.allowed() {
		return fmt.Errorf("team actions are blocked")
	}
	switch action {
	case "petMode":
		var p struct{ Slot, Mode int }
		if seui.DecodeAutomation(raw, &p) != nil || p.Slot < 1 || p.Slot > game.MaxPets || p.Mode < modeRest || p.Mode > modeRide || f.Inventory.State.Pets[p.Slot-1].ID == 0 {
			return fmt.Errorf("invalid pet mode")
		}
		f.SetMode(byte(p.Slot), p.Mode)
	case "dismissPet":
		var slot int
		if seui.DecodeAutomation(raw, &slot) != nil || slot < 1 || slot > game.MaxPets || f.Inventory.State.Pets[slot-1].ID == 0 {
			return fmt.Errorf("invalid pet")
		}
		f.DismissPet(byte(slot))
	case "leave":
		if !f.State.InParty() {
			return fmt.Errorf("not in a team")
		}
		f.End.OnClick()
	case "promote", "kick":
		var id uint32
		if seui.DecodeAutomation(raw, &id) != nil {
			return fmt.Errorf("member ID required")
		}
		found := -1
		for i, other := range f.State.Others() {
			if other == id {
				found = i
				break
			}
		}
		if found < 0 || f.State.Leader != f.State.Self {
			return fmt.Errorf("member unavailable or player is not leader")
		}
		if action == "promote" {
			f.Leader[found].OnClick()
		} else {
			f.Kick[found].OnClick()
		}
	case "renamePet":
		var p struct {
			Slot int
			Name string
		}
		if seui.DecodeAutomation(raw, &p) != nil || p.Slot < 1 || p.Slot > game.MaxPets || f.Inventory.State.Pets[p.Slot-1].ID == 0 {
			return fmt.Errorf("invalid pet")
		}
		name, err := seui.EncodeText(p.Name)
		if err != nil || len(name) < 1 || len(name) > petNameLimit {
			return fmt.Errorf("invalid pet name")
		}
		f.editName(byte(p.Slot))
		if f.Names[p.Slot-1].ReadOnly {
			return fmt.Errorf("pet name cannot be edited")
		}
		f.Names[p.Slot-1].SetText(name)
		f.finishName()
	default:
		return fmt.Errorf("unsupported team action")
	}
	return nil
}
