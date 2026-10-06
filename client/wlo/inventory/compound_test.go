package inventory

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestCompoundResultValidation(t *testing.T) {
	packet := append([]byte{23, 8, 1, 0x59, 0x1b, 1}, make([]byte, 28)...)
	for _, change := range []func([]byte) []byte{
		func(p []byte) []byte { return p[:len(p)-1] },
		func(p []byte) []byte { p[2] = 0; return p },
		func(p []byte) []byte { p[2] = 51; return p },
		func(p []byte) []byte { p[3], p[4] = 0, 0; return p },
		func(p []byte) []byte { p[5] = 0; return p },
		func(p []byte) []byte { p[5] = 255; return p },
	} {
		s := State{Bag: game.Inventory{{}, {ID: 32011, Count: 1}}}
		before := s.Bag
		handled, valid := s.Apply(change(bytes.Clone(packet)))
		if !handled || valid || s.Bag != before {
			t.Fatal("malformed result published state")
		}
	}
	s := State{Bag: game.Inventory{{ID: 32011, Count: 1}}}
	if handled, valid := s.Apply(packet); !handled || valid || s.Bag[0].ID != 32011 {
		t.Fatal("result overwrote existing item")
	}
	s.Bag = game.Inventory{}
	packet[6] = 7
	if handled, valid := s.Apply(packet); !handled || !valid || s.Bag[0].ID != 7001 || s.Bag[0].Count != 1 || s.Bag[0].Metadata[0] != 7 {
		t.Fatal("fresh result/metadata", s.Bag[0])
	}
}

func TestAlchemyMaterialBaseTooltip(t *testing.T) {
	item := assets.NativeItem{}
	item.Record[136], item.Record[138] = 1, 67
	if got := alchemyBaseLine(item); got != "Alch Mat: Flower / Magical Item" {
		t.Fatal(got)
	}
	item.Record[138], item.Record[140] = 49, 67
	if got := alchemyBaseLine(item); got != "Alch Mat: Flower / Fur / Magical Item" {
		t.Fatal(got)
	}
}
