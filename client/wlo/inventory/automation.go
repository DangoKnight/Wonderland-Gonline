package inventory

import (
	"encoding/json"
	"fmt"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func apiRejected(ok bool) error {
	if !ok {
		return fmt.Errorf("action is unavailable in current state")
	}
	return nil
}

type apiSlot struct {
	Slot int    `json:"slot"`
	Item uint16 `json:"item"`
}

func (f *Form) AutomationInfo() seui.AutomationInfo {
	definitions := map[uint16]game.ItemDefinition{}
	for _, item := range f.State.Bag {
		if !item.Empty() {
			definitions[item.ID] = f.State.Items[item.ID].Definition
		}
	}
	return seui.AutomationInfo{Kind: "inventory", Actions: []string{"selectActor", "use", "equip", "unequip", "move", "drop", "allocate", "cancelAllocation"}, Value: map[string]any{
		"definitions": definitions,
		"bag":         f.State.Bag, "equipment": f.Equipment(), "pets": automationPets(f.State.Pets[:]), "actor": f.Selected, "stats": f.DisplayStats(), "draft": f.PendingPoints, "allocationWaiting": f.AllocationWaiting,
	}}
}

func (w *destroyWarning) AutomationInfo() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "warning", Text: w.text, Actions: []string{}}
}
func (w *destroyWarning) AutomationInvoke(string, json.RawMessage) error {
	return fmt.Errorf("choose an explicit confirmation button")
}
func (w *potentialWarning) AutomationInfo() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "warning", Text: w.text, Actions: []string{}}
}
func (w *potentialWarning) AutomationInvoke(string, json.RawMessage) error {
	return fmt.Errorf("choose an explicit confirmation button")
}
func (f *Form) AutomationInvoke(action string, raw json.RawMessage) error {
	if !f.allowed() || f.Blocked() {
		return apiRejected(false)
	}
	switch action {
	case "selectActor":
		var actor int
		if seui.DecodeAutomation(raw, &actor) != nil || actor < 0 || actor > game.MaxPets || actor > 0 && f.State.Pets[actor-1].ID == 0 {
			return fmt.Errorf("invalid inventory actor")
		}
		f.Selected = byte(actor)
		return nil
	case "use", "equip", "unequip":
		var p apiSlot
		if seui.DecodeAutomation(raw, &p) != nil {
			return fmt.Errorf("slot and expected item required")
		}
		limit := game.BagSize
		if action == "unequip" {
			limit = EquipmentSlots
		}
		if p.Slot < 1 || p.Slot > limit {
			return fmt.Errorf("invalid item slot")
		}
		item := f.State.Bag[p.Slot-1]
		if action == "unequip" {
			item = f.Equipment()[p.Slot-1]
		}
		if item.Empty() || item.Locked || item.ID != p.Item {
			return fmt.Errorf("item changed or locked")
		}
		if action == "equip" {
			return f.AutomationEquip(byte(p.Slot))
		}
		f.Use(p.Slot, action == "unequip")
		return nil
	case "move", "drop":
		var p struct {
			Slot, To, Count int
			Item            uint16
		}
		if seui.DecodeAutomation(raw, &p) != nil || p.Slot < 1 || p.Slot > game.BagSize || p.Count < 1 || p.Count > game.MaxItemStack {
			return fmt.Errorf("invalid item movement")
		}
		it := f.State.Bag[p.Slot-1]
		if it.Empty() || it.Locked || it.ID != p.Item || p.Count > int(it.Count) {
			return fmt.Errorf("item changed or locked")
		}
		if action == "drop" {
			if def, known := f.State.Items[it.ID]; known && !def.Definition.Droppable() {
				f.ConfirmDestroy(byte(p.Slot), it.ID, byte(p.Count))
				return nil
			}
			f.send([]byte{protocol.CommandInventory, protocol.InventoryDrop, byte(p.Slot), byte(p.Count), 0})
			return nil
		}
		if p.To < 1 || p.To > game.BagSize || p.To == p.Slot {
			return fmt.Errorf("invalid destination slot")
		}
		bag := f.State.Bag
		if moved, err := bag.Move(byte(p.Slot), byte(p.To), byte(p.Count), f.State.Items[it.ID].Definition.StackLimit()); err != nil || int(moved) != p.Count {
			return fmt.Errorf("destination cannot hold requested quantity")
		}
		f.send([]byte{protocol.CommandInventory, protocol.InventoryMove, byte(p.Slot), byte(p.Count), byte(p.To)})
		return nil
	case "allocate":
		var points [allocationAttributes]uint16
		if seui.DecodeAutomation(raw, &points) != nil || !f.allocationAllowed() || f.AllocationWaiting {
			return apiRejected(false)
		}
		var total uint32
		values := f.Attributes()
		for i, n := range points {
			total += uint32(n)
			if uint32(values[i])+uint32(n) > 0xffff {
				return fmt.Errorf("attribute limit exceeded")
			}
		}
		if total == 0 || total > uint32(f.Stats.Points) {
			return fmt.Errorf("insufficient stat points")
		}
		f.PendingPoints = points
		f.SubmitAllocation()
		return apiRejected(f.AllocationWaiting)
	case "cancelAllocation":
		f.CancelPoints()
		return nil
	}
	return fmt.Errorf("unsupported inventory action")
}

func (f *Form) AutomationEquip(slot byte) error {
	it := f.State.Bag[slot-1]
	def, ok := f.State.Items[it.ID]
	if !ok || def.Definition.EquipSlot < 1 || def.Definition.EquipSlot > EquipmentSlots {
		return fmt.Errorf("item is not equipment")
	}
	if uint16(f.DisplayStats().Level) < def.Definition.EquipLevel {
		return game.ErrEquipLevel
	}
	f.equipBag(slot)
	return nil
}

func (f *ServiceForm) AutomationInfo() seui.AutomationInfo {
	value := map[string]any{"mode": f.Mode, "selection": f.Selected, "waiting": f.Waiting, "page": f.Page}
	if f.Gold != nil {
		value["gold"] = f.Gold()
	}
	switch f.Mode {
	case ServiceBuy:
		value["stock"], value["cart"], value["cartSlots"] = f.Stock, f.Cart, f.CartSlots
	case ServiceStorage:
		value["storage"], value["bag"] = f.Storage, f.Inventory.State.Bag
	case ServiceSell:
		value["bag"] = f.Inventory.State.Bag
	case ServiceHotel:
		value["hotel"], value["pets"] = automationPets(f.Hotel[:]), automationPets(f.Inventory.State.Pets[:])
	}
	return seui.AutomationInfo{Kind: "npcService", Actions: []string{"select", "confirm", "deposit", "withdraw", "close", "page"}, Value: value}
}

func automationPets(pets []UsePet) []map[string]any {
	out := []map[string]any{}
	for i, p := range pets {
		if p.ID != 0 {
			out = append(out, map[string]any{"index": i, "id": p.ID, "name": seui.DecodeText(p.Name), "stats": p.Stats, "amity": p.Amity, "potential": p.Potential, "skills": p.Skills, "equipment": p.Equipment})
		}
	}
	return out
}
func (f *ServiceForm) AutomationInvoke(action string, raw json.RawMessage) error {
	if !f.Visible || f.Waiting {
		return apiRejected(false)
	}
	switch action {
	case "select":
		var p struct{ Side, Index int }
		if seui.DecodeAutomation(raw, &p) != nil || p.Side < 0 || p.Side > 1 || p.Index < 0 || p.Index >= game.BagSize {
			return fmt.Errorf("invalid service selection")
		}
		beforeSelected, beforeCart := f.Selected, f.Cart
		f.Select(p.Side, p.Index)
		return apiRejected(beforeSelected != f.Selected || beforeCart != f.Cart)
	case "page":
		var page int
		if seui.DecodeAutomation(raw, &page) != nil || f.Mode != ServiceBuy || page < 0 || page >= max(1, len(f.Stock)) {
			return fmt.Errorf("invalid shop page offset")
		}
		f.Page = page
		return nil
	case "confirm":
		if f.Mode != ServiceBuy && f.Mode != ServiceSell {
			return apiRejected(false)
		}
		f.Confirm()
		return apiRejected(f.Waiting)
	case "deposit", "withdraw":
		if f.Mode != ServiceStorage && f.Mode != ServiceHotel {
			return apiRejected(false)
		}
		f.Transfer(action == "deposit")
		return apiRejected(f.Waiting)
	case "close":
		f.Close()
		return nil
	}
	return fmt.Errorf("unsupported service action")
}

func (f *CompoundForm) AutomationInfo() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "compound", Actions: []string{"select", "remove", "tier", "synthesize"}, Value: map[string]any{"selection": f.Selection, "items": f.selectedItems, "pending": f.pending, "tier": f.Tier, "result": f.result}}
}
func (f *CompoundForm) AutomationInvoke(action string, raw json.RawMessage) error {
	if !f.allowed() || f.pending {
		return apiRejected(false)
	}
	switch action {
	case "select":
		var p struct {
			Slot, Index int
			Item        uint16
		}
		if seui.DecodeAutomation(raw, &p) != nil || p.Slot < 1 || p.Slot > game.BagSize || p.Index < 0 || p.Index >= compoundIngredients || f.Inventory.State.Bag[p.Slot-1].ID != p.Item {
			return fmt.Errorf("invalid ingredient")
		}
		return apiRejected(f.SelectIngredient(byte(p.Slot), p.Index))
	case "remove":
		var index int
		if seui.DecodeAutomation(raw, &index) != nil || index < 0 || index >= compoundIngredients {
			return fmt.Errorf("invalid ingredient index")
		}
		f.clearIngredient(index)
		return nil
	case "tier":
		var tier game.AlchemyTier
		if seui.DecodeAutomation(raw, &tier) != nil {
			return fmt.Errorf("invalid alchemy tier")
		}
		return apiRejected(f.SelectTier(tier))
	case "synthesize":
		f.Synthesize()
		return apiRejected(f.pending)
	}
	return fmt.Errorf("unsupported compound action")
}

func (d *useDialog) AutomationInfo() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "potential", Actions: []string{"target", "confirm", "close"}, Value: map[string]any{"target": d.target, "selected": d.selected, "item": d.item, "pets": automationPets(d.owner.State.Pets[:]), "stats": d.owner.Stats}}
}
func (d *useDialog) AutomationInvoke(action string, raw json.RawMessage) error {
	switch action {
	case "target":
		var target int
		if seui.DecodeAutomation(raw, &target) != nil || target < 0 || target > game.MaxPets {
			return fmt.Errorf("invalid potential target")
		}
		d.SelectTarget(byte(target))
		return apiRejected(d.selected && d.target == byte(target))
	case "confirm":
		if !d.selected || d.owner.potentialRequest != nil {
			return apiRejected(false)
		}
		d.confirm()
		return nil
	case "close":
		d.Hide()
		return nil
	}
	return fmt.Errorf("unsupported potential action")
}

func (d *cardDialog) AutomationInfo() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "giftCard", Actions: []string{"select", "confirm", "close"}, Value: map[string]any{"rewards": d.rewards, "selected": d.selected, "waiting": d.waiting, "item": d.item}}
}
func (d *cardDialog) AutomationInvoke(action string, raw json.RawMessage) error {
	if d.waiting {
		return apiRejected(false)
	}
	switch action {
	case "select":
		var index int
		if seui.DecodeAutomation(raw, &index) != nil || index < 0 || index >= len(d.rewards) {
			return fmt.Errorf("invalid reward choice")
		}
		d.selected = index
		d.page = index / giftCardRows
		d.sync()
		return nil
	case "confirm":
		d.exchange()
		return apiRejected(d.waiting)
	case "close":
		d.Hide()
		return nil
	}
	return fmt.Errorf("unsupported gift card action")
}

func (f *TradeForm) AutomationInfo() seui.AutomationInfo {
	value := map[string]any{"partner": f.Partner, "offer": f.offer, "partnerItems": f.partnerItems, "confirmed": f.Confirmed, "partnerOffered": f.PartnerOffered, "decision": f.Decision}
	if f.Inventory != nil && f.Inventory.State != nil {
		value["bag"] = f.Inventory.State.Bag
	}
	if f.Gold != nil {
		value["gold"] = seui.DecodeText(f.Gold.Text)
	}
	return seui.AutomationInfo{Kind: "trade", Actions: []string{"offer", "withdraw", "confirm", "accept", "close"}, Value: value}
}
func (f *TradeForm) AutomationInvoke(action string, raw json.RawMessage) error {
	switch action {
	case "offer":
		var p apiSlot
		if seui.DecodeAutomation(raw, &p) != nil || p.Slot < 1 || p.Slot > game.BagSize || f.Inventory.State.Bag[p.Slot-1].ID != p.Item {
			return fmt.Errorf("item changed")
		}
		return apiRejected(f.Offer(byte(p.Slot)))
	case "withdraw":
		var index int
		if seui.DecodeAutomation(raw, &index) != nil || index < 0 || index >= tradeCells || f.Confirmed {
			return fmt.Errorf("invalid offer cell")
		}
		// API grid indexes are zero-based; native trade anchors are one-based.
		f.Withdraw(index + 1)
		return nil
	case "confirm":
		if f.Confirmed {
			return apiRejected(false)
		}
		f.confirm()
		return apiRejected(f.Confirmed)
	case "accept":
		if !f.Confirmed || !f.PartnerOffered {
			return apiRejected(false)
		}
		f.accept()
		return apiRejected(f.Decision == protocol.TradeDecisionAccept)
	case "close":
		f.Close()
		return nil
	}
	return fmt.Errorf("unsupported trade action")
}
