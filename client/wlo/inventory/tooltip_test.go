package inventory

import (
	"reflect"
	"strings"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
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
