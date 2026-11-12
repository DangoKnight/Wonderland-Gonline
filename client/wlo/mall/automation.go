package mall

import (
	"encoding/json"
	"fmt"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
)

func (p *ContentsForm) AutomationInfo() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "bagContents", Actions: []string{}, Value: map[string]any{"item": p.ItemID, "rewards": p.Rewards}}
}

func (f *Form) AutomationInfo() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "mall", Actions: []string{"currency", "category", "page", "add", "quantity", "remove", "cart", "catalog", "checkout", "contents", "tryOn", "resetFitting", "forgeSelect", "forge", "game"}, Value: map[string]any{
		"catalog": f.State.Catalog, "balances": f.State.Balances, "bonus": f.State.Bonus, "enabled": f.State.Enabled, "cart": f.State.Cart, "busy": f.State.Busy(), "category": f.Category, "page": f.Page, "inCart": f.InCart, "entries": f.Entries(), "forgeSlot": f.ForgeSlot, "forgeItem": f.ForgeItem, "forgePending": f.ForgePending, "fitted": f.Fitted,
	}}
}
func (f *Form) AutomationInvoke(action string, raw json.RawMessage) error {
	if f.State.Busy() || f.ForgePending {
		return fmt.Errorf("mall transaction is pending")
	}
	var index int
	switch action {
	case "currency":
		var bonus bool
		if seui.DecodeAutomation(raw, &bonus) != nil {
			return fmt.Errorf("boolean currency required")
		}
		f.SelectCurrency(bonus)
	case "category":
		if seui.DecodeAutomation(raw, &index) != nil || index < int(TabHot) || index > int(TabForging) || f.State.Bonus && index > int(TabFurniture) {
			return fmt.Errorf("invalid category")
		}
		f.selectTab(byte(index))
	case "page":
		if seui.DecodeAutomation(raw, &index) != nil || index < 1 || index > f.Pages() {
			return fmt.Errorf("invalid page")
		}
		f.Page = index
		f.Refresh()
	case "add":
		var id uint16
		if seui.DecodeAutomation(raw, &id) != nil {
			return fmt.Errorf("catalog item ID required")
		}
		if f.InCart || f.Category == TabGames || f.Category == TabForging {
			return fmt.Errorf("catalog is not open")
		}
		rows := f.Entries()
		found := -1
		for i, e := range rows {
			if e.ID == id {
				found = i
				break
			}
		}
		if found < 0 {
			return fmt.Errorf("item not in selected catalog")
		}
		before := len(f.State.Cart)
		f.Page = found/CatalogRows + 1
		f.AddRow(found % CatalogRows)
		if len(f.State.Cart) == before {
			return fmt.Errorf("cart rejected item")
		}
	case "quantity":
		var p struct{ Index, Quantity int }
		if seui.DecodeAutomation(raw, &p) != nil || p.Index < 0 || p.Index >= len(f.State.Cart) || p.Quantity < 1 || p.Quantity > MaximumQuantity {
			return fmt.Errorf("invalid cart quantity")
		}
		f.State.Quantity(p.Index, p.Quantity)
		f.Refresh()
	case "remove":
		if seui.DecodeAutomation(raw, &index) != nil || index < 0 || index >= len(f.State.Cart) {
			return fmt.Errorf("invalid cart row")
		}
		f.State.Remove(index)
		f.Refresh()
	case "cart":
		f.ShowCart()
	case "catalog":
		f.ShowCatalog()
	case "checkout":
		f.CheckOut()
		if !f.State.Busy() {
			return fmt.Errorf("checkout rejected; inspect balances and cart")
		}
	case "contents", "tryOn":
		var id uint16
		if seui.DecodeAutomation(raw, &id) != nil {
			return fmt.Errorf("catalog item ID required")
		}
		found := false
		for _, e := range f.Entries() {
			if e.ID == id {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("item not in selected catalog")
		}
		if action == "contents" {
			if !f.ShowContents(id) {
				return fmt.Errorf("contents unavailable")
			}
		} else if !f.TryOn(id) {
			return fmt.Errorf("item has no preview")
		}
	case "resetFitting":
		f.ResetFitting()
	case "forgeSelect":
		var p struct {
			Slot int
			Item uint16
		}
		if seui.DecodeAutomation(raw, &p) != nil || p.Slot < 1 || p.Slot > game.BagSize || f.forgeBag()[p.Slot-1].ID != p.Item || f.Category != TabForging {
			return fmt.Errorf("invalid forge item")
		}
		f.selectForge(byte(p.Slot))
		if int(f.ForgeSlot) != p.Slot {
			return fmt.Errorf("item cannot be forged")
		}
	case "forge":
		if f.Category != TabForging || !f.selectedForgeMatches() {
			return fmt.Errorf("forge item changed")
		}
		f.confirmForge()
	case "game":
		if seui.DecodeAutomation(raw, &index) != nil || f.Category != TabGames || index < 0 || index >= len(mallGames) {
			return fmt.Errorf("invalid mall game")
		}
		f.Page = index/mallGamePageSize + 1
		f.openGame(index % mallGamePageSize)
	default:
		return fmt.Errorf("unsupported mall action")
	}
	return nil
}
