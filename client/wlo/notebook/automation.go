package notebook

import (
	"encoding/json"
	"fmt"
	"wonderland-gonline/client/wlo/seui"
)

func (b *Book) AutomationInfo() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "notebook", Actions: []string{"select", "page", "scroll"}, Value: map[string]any{"quests": b.State.Entries(), "monsters": b.State.MonsterList(), "monsterBook": b.Monsters, "page": b.Page, "selected": b.Selected, "scroll": b.Scroll}}
}
func (b *Book) AutomationInvoke(action string, raw json.RawMessage) error {
	var index int
	if seui.DecodeAutomation(raw, &index) != nil || index < 0 {
		return fmt.Errorf("nonnegative index required")
	}
	switch action {
	case "select":
		if index >= b.count() {
			return fmt.Errorf("entry unavailable")
		}
		b.Selected = index
		b.Page = index / RowsPerPage
		b.Scroll = 0
	case "page":
		if index*RowsPerPage >= max(1, b.count()) {
			return fmt.Errorf("page unavailable")
		}
		b.Page = index
		b.Selected = index * RowsPerPage
		b.Scroll = 0
	case "scroll":
		if index > 4096 {
			return fmt.Errorf("scroll limit exceeded")
		}
		b.Scroll = index
	default:
		return fmt.Errorf("unsupported notebook action")
	}
	return nil
}
