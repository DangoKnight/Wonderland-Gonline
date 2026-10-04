package inventory

import (
	"encoding/binary"
	"fmt"
	"strings"
	"wonderland-go/client/wlo/seui"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/clientassets"
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
	moveQuantityTitle            = "Moving quantity"
)

// FUN_00485a20; authored type names are client presentation data.
var itemTypeNames = map[byte]string{
	1: "Saber", 2: "Sword", 3: "Halberd", 4: "Bow", 5: "Wand",
	6: "Claw", 7: "Knuckle", 8: "Axe", 9: "Club", 10: "Exotic",
	11: "Hammer", 12: "Body", 13: "Head", 14: "Hands", 15: "Shoes",
	16: "Special Eq", 21: "Quest Item", 25: "Spec Prop",
}

func itemInfoLines(item assets.NativeItem, id uint16) []string {
	name := item.Definition.Name
	if name == "" {
		name = fmt.Sprintf("Item %d", id)
	}
	lines := []string{"[ " + name + " ]"}
	// FUN_0028cbec's catalog flag. Socketed/bound item metadata requires
	// additional native rules; do not infer it from inventory reservation locks.
	if binary.LittleEndian.Uint16(item.Record[itemTradeFlagsOffset:])&itemNonTradeableFlag != 0 {
		lines = append(lines, "<Non-tradeable>")
	}
	kind := itemTypeNames[item.Definition.Type]
	if kind == "" {
		kind = "Prop"
	}
	lines = append(lines, "Type: "+kind, fmt.Sprintf("Rank: %d", item.Record[itemRankOffset]))
	if item.Definition.EquipSlot >= 1 && item.Definition.EquipSlot <= EquipmentSlots {
		if level := item.Record[itemEquipLevelOffset]; level != 0 {
			lines = append(lines, fmt.Sprintf("LV: %d", level))
		}
	}
	if item.Description != "" {
		lines = append(lines, item.Description)
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
	it := c.item()
	if it.Empty() || c.form.drag != nil || c.Blocked() {
		return
	}
	f := c.form
	item := f.State.Items[it.ID]
	lines := itemInfoLines(item, it.ID)
	c.SetHint(clientassets.Big5Text(strings.TrimSuffix(strings.TrimPrefix(lines[0], "[ "), " ]")))
	c.GrBasic.Hint()
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
	at := c.Abs()
	panel.Width, panel.Height = itemInfoWidth, min(height, f.Env.Screen.H)
	panel.Left, panel.Top = at.X+c.Width+2, at.Y
	if panel.Left+panel.Width > f.Env.Screen.W {
		panel.Left = at.X - panel.Width - 2
	}
	panel.Left = max(0, panel.Left)
	panel.Top = max(0, min(panel.Top, f.Env.Screen.H-panel.Height))
	panel.Paint()
	for i, row := range rows {
		if (i+1)*itemInfoLineHeight > panel.Height-10 {
			break
		}
		c.Env.Text.Draw(panel.Left+10, panel.Top+10+i*itemInfoLineHeight, 0, false, true, c.Env.Screen, row, itemInfoLineHeight, itemInfoTextWidth, itemInfoPaper, itemInfoInk, 2)
	}
}
