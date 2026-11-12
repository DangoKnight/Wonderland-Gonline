package skills

import (
	"encoding/json"
	"fmt"
	"wonderland-gonline/client/wlo/seui"
)

func (f *Form) AutomationInfo() seui.AutomationInfo {
	rows := []map[string]any{}
	t, ok := f.target()
	if ok {
		for _, id := range f.rows {
			d := f.definition(id, t)
			rows = append(rows, map[string]any{"id": id, "definition": d, "progress": t.Skills[id]})
		}
	}
	return seui.AutomationInfo{Kind: "skills", Actions: []string{"actor", "tab", "select", "use"}, Value: map[string]any{"actor": f.Selected, "tab": f.Tab, "rows": rows}}
}
func (f *Form) AutomationInvoke(action string, raw json.RawMessage) error {
	switch action {
	case "actor":
		var slot byte
		if seui.DecodeAutomation(raw, &slot) != nil {
			return fmt.Errorf("actor slot required")
		}
		found := false
		if f.Targets != nil {
			for _, target := range f.Targets() {
				if target.Slot == slot {
					found = true
					break
				}
			}
		}
		if !found {
			return fmt.Errorf("skill actor unavailable")
		}
		f.Selected = slot
		f.Refresh()
	case "tab":
		var tab int
		if seui.DecodeAutomation(raw, &tab) != nil || tab < Physical || tab > Intro {
			return fmt.Errorf("invalid skill category")
		}
		f.SetTab(tab)
	case "select", "use":
		var id uint16
		if seui.DecodeAutomation(raw, &id) != nil {
			return fmt.Errorf("skill ID required")
		}
		found := -1
		for i, row := range f.rows {
			if row == id {
				found = i
				break
			}
		}
		if found < 0 {
			return fmt.Errorf("skill not in selected list")
		}
		f.List.Selected = found
		if action == "use" {
			if f.Use == nil {
				return fmt.Errorf("skill use unavailable")
			}
			f.Use(id, f.Selected)
		}
	default:
		return fmt.Errorf("unsupported skill action")
	}
	return nil
}
func (f *BattleForm) AutomationInfo() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "battleSkills", Actions: []string{}, Value: map[string]any{"actor": f.Selected, "skills": f.rows, "recent": f.recentRows}}
}
