package world

import (
	"image"
	"testing"

	"wonderland-gonline/client/wlo/surface"
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

// TestProps checks the island's props (map 10003): AC22:4's state fixes a
// prop's frame (FUN_0038cf30), props keep no name, the tiger (tall name
// base) draws 0x44 lower, and the coconut's record depth sorts it in
// front of its palm.
func TestProps(t *testing.T) {
	needAssets(t)
	rec, err := MapRecord(assets, 10003)
	if err != nil {
		t.Fatal(err)
	}
	templates, err := NPCTemplates(assets)
	if err != nil {
		t.Fatal(err)
	}
	npcs := MapNPCs(rec, templates, nil)
	chest, coconut, tiger, robinson := npcs[2], npcs[6], npcs[8], npcs[1]
	if !chest.Info.Prop() || !chest.Info.Unnamed() || robinson.Info.Prop() || robinson.Info.Unnamed() {
		t.Fatalf("kinds: chest %d, Robinson %d", chest.Info.Kind, robinson.Info.Kind)
	}
	if chest.Fixed {
		t.Fatalf("chest fixed on %d before AC22:4", chest.Frame)
	}
	if coconut.SortY() != coconut.Y+150 || robinson.SortY() != robinson.Y {
		t.Fatalf("sort keys: coconut %d at %d, Robinson %d at %d", coconut.SortY(), coconut.Y, robinson.SortY(), robinson.Y)
	}
	if tiger.Info.SpriteDrop() != spriteDropTall || robinson.Info.SpriteDrop() != 0 {
		t.Fatalf("drops: tiger %d, Robinson %d", tiger.Info.SpriteDrop(), robinson.Info.SpriteDrop())
	}
	w := &World{NPCs: npcs}
	w.ApplyActorPositions([]byte{
		2, 0, 0, 0, 0x41, 3, 0x5e, 7, actorShown, 0, 0, 0, 0, 0,
		1, 0, 0, 0, 0x60, 4, 0xa5, 8, actorShown, 0, 0, 0, 0, 0,
	})
	if !chest.Fixed || chest.Frame != 0 || robinson.Fixed {
		t.Fatalf("after AC22:4: chest %v %d, Robinson %v", chest.Fixed, chest.Frame, robinson.Fixed)
	}
	w.ApplyActorPositions([]byte{2, 0, frameFree, 0xff, 0x41, 3, 0x5e, 7, actorShown, 0, 0, 0, 0, 0})
	if chest.Fixed {
		t.Fatalf("chest fixed on %d after state 0xff", chest.Frame)
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

// TestPropSound: AC22:1 opening a chest (frame 0 to 1) returns its
// template's sound (wav9900); setting the same frame again does not.
func TestPropSound(t *testing.T) {
	needAssets(t)
	rec, err := MapRecord(assets, 10003)
	if err != nil {
		t.Fatal(err)
	}
	templates, err := NPCTemplates(assets)
	if err != nil {
		t.Fatal(err)
	}
	w := &World{NPCs: MapNPCs(rec, templates, nil)}
	chest := w.NPCs[2]
	if chest.Info.Sound != 9900 {
		t.Fatalf("chest sound %d", chest.Info.Sound)
	}
	w.ApplyActorPositions([]byte{2, 0, 0, 0, 0x41, 3, 0x5e, 7, actorShown, 0, 0, 0, 0, 0})
	if got := w.ApplyActorState([]byte{2, 0, propOpened}); got != 9900 || !chest.Fixed || chest.Frame != propOpened {
		t.Fatalf("opening: sound %d, chest %v %d", got, chest.Fixed, chest.Frame)
	}
	if got := w.ApplyActorState([]byte{2, 0, propOpened}); got != 0 {
		t.Fatalf("reopening sounded %d", got)
	}
}

// TestSoundZones: Newbie Island's 105 zones, the shore's (wav0050,
// radius 4) among them; a point by a zone's centre finds it.
func TestSoundZones(t *testing.T) {
	needAssets(t)
	s, err := LoadScene(assets, 10003)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Zones) != 105 {
		t.Fatalf("%d zones", len(s.Zones))
	}
	z := s.Zones[1]
	if z.Sound != 50 || z.Radius != 4 {
		t.Fatalf("zone 1 %+v", z)
	}
	got, ok := s.SoundZoneAt(z.X*cellSize+5, z.Y*cellSize+5)
	if !ok || got.Sound != 50 {
		t.Fatalf("at zone 1's centre: %+v %v", got, ok)
	}
	if _, ok := s.SoundZoneAt(1, 1); ok {
		t.Fatal("a zone at the map's corner")
	}
}
