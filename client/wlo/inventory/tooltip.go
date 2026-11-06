package inventory

import "wonderland-gonline/internal/localization"

import (
	"encoding/binary"
	"fmt"
	"image"
	"strings"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
)

// TSe_ItemInfo constructor/builder/painter: FUN_00287e0c, FUN_00287f58,
// FUN_0028cc74. Offsets refer to decoded Item.dat records, without the
// native object's four-byte prefix. These are distinct from legacy Level.
const (
	itemRankOffset               = 45
	itemEquipLevelOffset         = 113
	itemTradeFlagsOffset         = 123
	itemNonTradeableFlag         = 1 << 1
	itemInfoWidth                = 196
	itemInfoTextWidth            = 176
	itemInfoLineHeight           = 20
	itemInfoInk                  = uint16(0xffe0)
	itemInfoPaper                = uint16(0)
	nativeDescriptionSizingWidth = 130
	moveQuantityTitle            = "ui.inventory.moving_quantity_3f622375"
	itemExperienceStatus         = 36
	itemShootResistanceStatus    = 100
	itemAttackBonusStatus        = 210
	itemDefenseBonusStatus       = 211
	itemSpeedBonusStatus         = 214
	itemMagicAttackBonusStatus   = 215
	itemMagicDefenseBonusStatus  = 216
	itemHoverColor               = 0x0080ff // Warm footprint tint from the legacy reference.
	itemHoverAlpha               = 80
)

// FUN_00485a20; authored type names are client presentation data.
var itemTypeNames = map[byte]string{
	1: localization.Text("ui.inventory.saber_5f002f3d"), 2: localization.Text("ui.inventory.sword_d0b9cd97"), 3: localization.Text("ui.inventory.halberd_60c39704"), 4: localization.Text("ui.inventory.bow_a0154612"), 5: localization.Text("ui.inventory.wand_3e7962d2"),
	6: localization.Text("ui.inventory.claw_587cfbbb"), 7: localization.Text("ui.inventory.knuckle_825f9bd6"), 8: localization.Text("ui.inventory.axe_9e28648f"), 9: localization.Text("ui.inventory.club_72e81fea"), 10: localization.Text("ui.inventory.exotic_a280e8b7"),
	11: localization.Text("ui.inventory.hammer_c157cbf6"), 12: localization.Text("ui.inventory.body_6ccaa641"), 13: localization.Text("ui.inventory.head_9bb0aacc"), 14: localization.Text("ui.inventory.hands_6b720477"), 15: localization.Text("ui.inventory.shoes_b5efa9ec"),
	16: localization.Text("ui.inventory.special_eq_ba3b07a5"), 21: localization.Text("ui.inventory.quest_item_0d0ed5d1"), 25: localization.Text("ui.inventory.spec_prop_6ba6c664"),
}

func itemInfoLines(item assets.NativeItem, id uint16) []string {
	name := item.Definition.Name
	if name == "" {
		name = fmt.Sprintf(localization.Text("ui.inventory.item_d_c8edea5f"), id)
	}
	lines := []string{"[ " + name + " ]"}
	// FUN_0028cbec's catalog flag. Socketed/bound item metadata requires
	// additional native rules; do not infer it from inventory reservation locks.
	if binary.LittleEndian.Uint16(item.Record[itemTradeFlagsOffset:])&itemNonTradeableFlag != 0 {
		lines = append(lines, localization.Text("ui.inventory.nontradeable"))
	}
	lines = append(lines, itemEffectLines(item.Definition)...)
	kind := itemTypeNames[item.Definition.Type]
	if kind == "" {
		kind = localization.Text("ui.inventory.prop_57d96886")
	}
	lines = append(lines, localization.Text("ui.inventory.type_46f8246f")+kind, fmt.Sprintf(localization.Text("ui.inventory.rank_d_47d44302"), item.Record[itemRankOffset]))
	if item.Definition.EquipSlot >= 1 && item.Definition.EquipSlot <= EquipmentSlots {
		if level := item.Record[itemEquipLevelOffset]; level != 0 {
			lines = append(lines, fmt.Sprintf(localization.Text("ui.inventory.lv_d_fc231d7a"), level))
		}
	}
	if item.Description != "" {
		lines = append(lines, item.Description)
	}
	return lines
}

// FUN_00287f58 lists the two catalog effects before type/rank. Values are
// encoded around 100: 400 means +300, 95 means -5. Zero/100 are inert.
func itemEffectLines(def game.ItemDefinition) []string {
	labels := map[uint16]string{itemHPStatus: localization.Text("ui.inventory.hp_8f1d4a34"), itemSPStatus: localization.Text("ui.inventory.sp_8cce03ee"), itemHPBonusStatus: localization.Text("ui.inventory.maxhp"), itemSPBonusStatus: localization.Text("ui.inventory.maxsp"), itemAmityStatus: localization.Text("ui.inventory.amity_56851ac2"), itemExperienceStatus: localization.Text("ui.inventory.exp_06a1ad63"), itemShootResistanceStatus: localization.Text("ui.inventory.shoot_res_ecd0d723"), itemAttackBonusStatus: localization.Text("ui.inventory.atk_ad5b433f"), itemDefenseBonusStatus: localization.Text("ui.inventory.def_967c5a5b"), itemSpeedBonusStatus: localization.Text("ui.inventory.spd_94424ddc"), itemMagicAttackBonusStatus: localization.Text("ui.inventory.mat_321081e2"), itemMagicDefenseBonusStatus: localization.Text("ui.inventory.mdf_64d74883")}
	var lines []string
	for i, status := range def.Status {
		label, known := labels[status]
		value := def.Values[i]
		if known && value != 0 && value != recoveryValueBase {
			lines = append(lines, fmt.Sprintf("%s: %+d", label, value-recoveryValueBase))
		}
	}
	return lines
}

// Wrap encoded glyphs, matching the bitmap renderer's eight-pixel advance.
// Wrapping is by character, including spaces, rather than by word.
func wrapItemInfo(lines []string) [][]byte {
	var out [][]byte
	for _, line := range lines {
		for _, part := range strings.Split(line, "\n") {
			data := clientassets.Big5Text(strings.TrimSuffix(part, "\r"))
			var row []byte
			width := 0
			for i := 0; i < len(data); {
				n, advance := 1, 8
				if i+1 < len(data) && clientassets.Big5Index(data[i], data[i+1]) <= clientassets.MaxGlyph {
					n, advance = 2, 16
				}
				if width+advance > itemInfoTextWidth {
					out = append(out, row)
					row = nil
					width = 0
				}
				row = append(row, data[i:i+n]...)
				width += advance
				i += n
			}
			out = append(out, row)
		}
	}
	return out
}

func (c *slotControl) Hint() {
	if c.form.drag != nil || c.Blocked() {
		return
	}
	anchor := c
	if !c.worn {
		slot := c.form.State.BagAnchor(c.slot)
		if slot == 0 {
			return
		}
		anchor = c.form.Slots[slot-1]
	}
	it := anchor.item()
	if it.Empty() {
		return
	}
	w, h := 1, 1
	if !c.worn {
		w, h = c.form.State.Items[it.ID].Definition.Footprint()
	}
	a := anchor.Abs()
	bounds := image.Rect(a.X, a.Y, a.X+w*cellStep-(cellStep-cellSize), a.Y+h*cellStep-(cellStep-cellSize))
	c.Env.Screen.FillAlpha(bounds, itemHoverColor, itemHoverAlpha)
	c.form.drawItem(it, a.X, a.Y, !c.worn)
	c.form.drawItemHintBounds(&anchor.Component, it, bounds)
}

// Bag views share the native item breakdown without sharing selection state.
func (f *Form) drawItemHint(control *seui.Component, it game.Item) {
	f.drawItemHintBounds(control, it, control.Rect())
}

func (f *Form) drawItemHintBounds(control *seui.Component, it game.Item, bounds image.Rectangle) {
	item := f.State.Items[it.ID]
	lines := itemInfoLines(item, it.ID)
	if item.Definition.Type == game.VehicleType {
		condition := item.VehicleCondition(it)
		if condition.Fuel > 0 {
			lines = append(lines, fmt.Sprintf(localization.Text("ui.inventory.fuel_d_0ef96842"), uint64(condition.Fuel)*100/uint64(max(1, condition.Maximum))))
		}
	}
	if control.Root().Base().Name == compoundNativeClass {
		lines = append(lines, alchemyBaseLine(item))
	}
	control.SetHint(clientassets.Big5Text(strings.TrimSuffix(strings.TrimPrefix(lines[0], "[ "), " ]")))
	control.GrBasic.Hint()
	rows := wrapItemInfo(lines)
	if f.itemInfo == nil {
		f.itemInfo = seui.NewPanel(f.Env, nil)
		f.itemInfo.Init("panel4", 0, 50, 50, 0, 0, true, 150, itemInfoWidth, 0)
		f.itemInfo.SetMargins(15, 15, 15, 15)
		f.itemInfo.Hover = false
	}
	panel := f.itemInfo
	// Native sizing reserves description space using a narrower width than
	// the painter. Preserve the resulting padding seen in the reference.
	height := (len(lines) + len(clientassets.Big5Text(item.Description))*8/nativeDescriptionSizingWidth + 1) * itemInfoLineHeight
	height = max(height, (len(rows)+1)*itemInfoLineHeight)
	panel.Width, panel.Height = itemInfoWidth, min(height, f.Env.Screen.H)
	panel.Left, panel.Top = bounds.Max.X+2, bounds.Min.Y
	if panel.Left+panel.Width > f.Env.Screen.W {
		panel.Left = bounds.Min.X - panel.Width - 2
	}
	panel.Left = max(0, panel.Left)
	panel.Top = max(0, min(panel.Top, f.Env.Screen.H-panel.Height))
	panel.Paint()
	for i, row := range rows {
		if (i+1)*itemInfoLineHeight > panel.Height-10 {
			break
		}
		control.Env.Text.Draw(panel.Left+10, panel.Top+10+i*itemInfoLineHeight, 0, false, true, control.Env.Screen, row, itemInfoLineHeight, itemInfoTextWidth, itemInfoPaper, itemInfoInk, 2)
	}
}

func init() {
	localization.Register(func() {
		itemTypeNames = map[byte]string{
			1: localization.Text("ui.inventory.saber_5f002f3d"), 2: localization.Text("ui.inventory.sword_d0b9cd97"), 3: localization.Text("ui.inventory.halberd_60c39704"), 4: localization.Text("ui.inventory.bow_a0154612"), 5: localization.Text("ui.inventory.wand_3e7962d2"),
			6: localization.Text("ui.inventory.claw_587cfbbb"), 7: localization.Text("ui.inventory.knuckle_825f9bd6"), 8: localization.Text("ui.inventory.axe_9e28648f"), 9: localization.Text("ui.inventory.club_72e81fea"), 10: localization.Text("ui.inventory.exotic_a280e8b7"),
			11: localization.Text("ui.inventory.hammer_c157cbf6"), 12: localization.Text("ui.inventory.body_6ccaa641"), 13: localization.Text("ui.inventory.head_9bb0aacc"), 14: localization.Text("ui.inventory.hands_6b720477"), 15: localization.Text("ui.inventory.shoes_b5efa9ec"),
			16: localization.Text("ui.inventory.special_eq_ba3b07a5"), 21: localization.Text("ui.inventory.quest_item_0d0ed5d1"), 25: localization.Text("ui.inventory.spec_prop_6ba6c664"),
		}
	})
}
