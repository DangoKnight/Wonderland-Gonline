package world

import (
	"testing"
	native "wonderland-gonline/internal/assets"
)

func TestKelanExchangerLegacyVisibility(t *testing.T) {
	for _, mapID := range []uint16{12000, 13000} {
		rec := native.Map{ID: mapID, NPCs: []native.MapNPC{
			{ClickID: 40, Template: 14157, Flags: 5, X: 1278, Y: 828, Sign: 7},
			{ClickID: 38, Template: 14134, Flags: 5, X: 1220, Y: 810},
		}}
		w := &World{NPCs: MapNPCs(rec, nil, nil)}
		n := w.NPCs[40]
		want := mapID != 12000
		if !n.Shown || n.Visible() != want || !w.NPCs[38].Visible() {
			t.Fatal("native suppression or neighboring actor visibility", mapID, n)
		}
		// Server visibility does not override FUN_004235d8's suppression.
		w.ApplyActorPositions([]byte{40, 0, 255, 0, 254, 4, 60, 3, 1, 0, 0, 0, 0, 0})
		if n.Visible() != want || (len(w.MiniMapSigns()) != 0) != want {
			t.Fatal("show packet revived suppressed exchanger", mapID, n)
		}
	}
}

func TestKelanExchangerAuthoredFlags(t *testing.T) {
	needAssets(t)
	rec, err := MapRecord(assets, 12000)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range rec.NPCs {
		if n.ClickID == 40 {
			if n.Template != 14157 || n.Flags != 5 {
				t.Fatal(n)
			}
			if MapNPCs(rec, nil, nil)[40].Visible() {
				t.Fatal("Kelan exchanger visible")
			}
			return
		}
	}
	t.Fatal("missing reference exchanger")
}
