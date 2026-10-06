package inventory

import (
	"reflect"
	"strings"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestInventoryItemInfoNativeFields(t *testing.T) {
	item := assets.NativeItem{Definition: game.ItemDefinition{Name: "Training Ticke", Type: 25, Level: 99}, Description: "Can be used to train on Training Island and receive EXP. Cannot be traded"}
	item.Record[itemRankOffset] = 1
	item.Record[itemTradeFlagsOffset] = itemNonTradeableFlag
	got := itemInfoLines(item, 34253)
	want := []string{"[ Training Ticke ]", "<Non-tradeable>", "Type: Spec Prop", "Rank: 1", item.Description}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	rows := wrapItemInfo(got)
	var description strings.Builder
	for _, row := range rows[4:] {
		if len(row)*8 > itemInfoTextWidth {
			t.Fatal("overflow", string(row))
		}
		description.Write(row)
	}
	if description.String() != item.Description {
		t.Fatal("wrapping changed description", description.String())
	}
	item.Definition.Type = 13
	item.Definition.EquipSlot = 1
	item.Record[itemEquipLevelOffset] = 10
	item.Record[itemTradeFlagsOffset] = 0
	got = itemInfoLines(item, 1)
	if strings.Join(got, "\n") != "[ Training Ticke ]\nType: Head\nRank: 1\nLV: 10\n"+item.Description {
		t.Fatal("native equipment level", got)
	}
}

func TestInventoryItemInfoBig5Wrapping(t *testing.T) {
	rows := wrapItemInfo([]string{strings.Repeat("水", 12)})
	if len(rows) != 2 || len(rows[0]) != 22 || len(rows[1]) != 2 {
		t.Fatal("split a Big5 glyph", rows)
	}
}

func TestInventoryConsumableEffectBreakdown(t *testing.T) {
	item := assets.NativeItem{Definition: game.ItemDefinition{Name: "Spicy Hot Pot", Status: [2]uint16{25, 26}, Values: [2]int32{400, 400}}, Description: "A Chaffy dish with hug u and spice"}
	item.Record[itemRankOffset] = 12
	item.Record[itemTradeFlagsOffset] = itemNonTradeableFlag
	want := []string{"[ Spicy Hot Pot ]", "<Non-tradeable>", "Hp: +300", "Sp: +300", "Type: Prop", "Rank: 12", item.Description}
	if got := itemInfoLines(item, 1); !reflect.DeepEqual(got, want) {
		t.Fatalf("screenshot effect breakdown %v", got)
	}
	item.Definition.Status = [2]uint16{64, 207}
	item.Definition.Values = [2]int32{95, 120}
	if got := itemEffectLines(item.Definition); !reflect.DeepEqual(got, []string{"Amity: -5", "MaxHp: +20"}) {
		t.Fatal(got)
	}
	item.Definition.Values = [2]int32{0, 100}
	if got := itemEffectLines(item.Definition); len(got) != 0 {
		t.Fatal("inert effects shown", got)
	}
}
