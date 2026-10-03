package world

import (
	"image"
	"testing"

	"wonderland-go/client/wlo/surface"
)

// TestStandFacing checks FUN_00484a50's facing conversion followed by the
// standing action: facing 6 stands facing the camera (11), as players do.
func TestStandFacing(t *testing.T) {
	for r, want := range map[byte]int{1: 8, 2: 15, 4: 13, 6: 11, 8: 9, 0: 8, 9: 8} {
		if got := standFacing(r); got != want {
			t.Errorf("facing %d: action %d, want %d", r, got, want)
		}
	}
}

type paintCount struct{ n *int }

func (p paintCount) Draw(*surface.Surface, int, int, int) { *p.n++ }
func (paintCount) FirstAnchorY() (int, bool)              { return 0, false }
func (paintCount) Bounds(int, int, int) image.Rectangle   { return image.Rectangle{} }

// TestShipDeckNPCs: map 10017 has eleven NPCs, all shown at load; the
// girl (click 1, template 14091) uses look 1010.
func TestShipDeckNPCs(t *testing.T) {
	needAssets(t)
	rec, err := MapRecord(assets, 10017)
	if err != nil {
		t.Fatal(err)
	}
	templates, err := NPCTemplates(assets)
	if err != nil {
		t.Fatal(err)
	}
	if tpl := templates[14091]; tpl.Look != 1010 || tpl.Colors[0] != 444334444 {
		t.Fatalf("template 14091: %+v", tpl)
	}
	painted := 0
	npcs := MapNPCs(rec, templates, func(NPCTemplate) NPCPainter { return paintCount{&painted} })
	if len(npcs) != 11 {
		t.Fatalf("%d NPCs", len(npcs))
	}
	girl := npcs[1]
	if girl.Template != 14091 || girl.X != 117 || girl.Y != 1303 || girl.Action != 11 || !girl.Shown || girl.Painter == nil {
		t.Fatalf("girl %+v", girl)
	}

	w := &World{NPCs: npcs}
	// AC22:4 after its subcommand: click 1 to (200, 300) hidden, then an
	// unknown click 99 that must be ignored.
	w.ApplyActorPositions([]byte{
		1, 0, 0xff, 0xff, 200, 0, 0x2c, 1, actorHidden, 0x18, 0xfc, 0xe7, 0x03, 0,
		99, 0, 0, 0, 1, 0, 1, 0, actorShown, 0, 0, 0, 0, 0,
	})
	if girl.X != 200 || girl.Y != 300 || girl.Shown {
		t.Fatalf("after AC22:4: %+v", girl)
	}
}

// TestNameHeight checks FUN_004265a4's +0x114 and the name lift.
func TestNameHeight(t *testing.T) {
	for _, tc := range []struct {
		t      NPCTemplate
		anchor int
		height int
		top    int
	}{
		{NPCTemplate{}, 20, 0x6a - 20, -(0x6a - 20) - nameLiftTall},
		{NPCTemplate{}, 0x50, 0x1a, -0x1a - nameLiftLow},
		{NPCTemplate{}, 0x70, 0, -nameLiftLow},
		{NPCTemplate{HeightPreset: 1}, 0x80, 0x26, -0x26 - nameLiftLow},
		{NPCTemplate{HeightScale: 1}, 0x50, 0x34, -0x34 - nameLiftTall},
		{NPCTemplate{HeightScale: 75}, 0, 0, -nameLiftLow},
	} {
		h := nameHeight(tc.t, tc.anchor)
		if h != tc.height || nameTop(h) != tc.top {
			t.Errorf("%+v anchor %d: height %d top %d, want %d %d", tc.t, tc.anchor, h, nameTop(h), tc.height, tc.top)
		}
	}
}
