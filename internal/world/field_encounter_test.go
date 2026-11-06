package world

import (
	"fmt"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
)

func TestFieldEncounterEventKinds(t *testing.T) {
	for _, tt := range []struct {
		name       string
		flag, kind byte
		operation  [21]byte
		want       bool
	}{
		{"mushroom kind 8", 5, 8, [21]byte{4, 1, 0, 29}, true},
		{"ordinary kind 10", 5, 10, [21]byte{4, 1, 0, 37}, true},
		{"scripted actor", 4, 8, [21]byte{4, 1, 0, 29}, false},
		{"other event kind", 5, 1, [21]byte{4, 1, 0, 29}, false},
		{"dialogue only", 5, 8, [21]byte{1, 1}, false},
		{"other battle operand", 5, 8, [21]byte{4, 2}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// The kind-8 header and operation are independent bytes from the
			// North Island mushroom encounter, not built from production codes.
			w := New(&assets.Catalog{Maps: map[uint16]assets.Map{60001: {
				ID:     60001,
				NPCs:   []assets.MapNPC{{ClickID: 25, Template: 17049, Flags: tt.flag, X: 2200, Y: 1400, Events: []byte{42}}},
				Events: []assets.Event{{ClickID: 42, Kind: tt.kind, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{{Index: 1, Data: tt.operation}}}}}},
			}}})
			if got := w.RoamingBattle(60001, 25); got != tt.want {
				t.Fatalf("roaming battle = %v, want %v", got, tt.want)
			}
			if tt.want {
				n, _ := w.NPC(60001, 25)
				if !w.Wild(60001, n) || w.SafeTown(60001) {
					t.Fatal("field encounter excluded from wild map classification")
				}
			}
		})
	}
}

func TestMobileFieldBattlesIgnoreEventHeader(t *testing.T) {
	// Include observed headers 1, 2 and Chicken Farm's 11, plus an arbitrary
	// header to prove this is movement/operation classification, not a whitelist.
	for _, kind := range []byte{1, 2, 8, 10, 11, 99} {
		for _, movement := range []byte{2, 3, 4, 5} {
			t.Run(fmt.Sprintf("kind%d/walk%d", kind, movement), func(t *testing.T) {
				steps := []assets.WalkStep{{X: 1300, Y: 1000}, {X: 1000, Y: 1000}}
				if movement == 3 {
					steps = []assets.WalkStep{{X: 0xffffff6a, Y: 0xffffff6a}, {X: 150, Y: 150}}
				}
				w := New(&assets.Catalog{Maps: map[uint16]assets.Map{60001: {
					ID:     60001,
					NPCs:   []assets.MapNPC{{ClickID: 25, Template: 40016, Flags: 5, WalkBehavior: movement, WalkSteps: steps, X: 1000, Y: 1000, Events: []byte{42}}},
					Events: []assets.Event{{ClickID: 42, Kind: kind, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{{Index: 1, Data: [21]byte{4, 1, 0, 29}}}}}}},
				}}})
				if !w.RoamingBattle(60001, 25) {
					t.Fatal("mobile enemy excluded by event kind or template range")
				}
				target := ActorPosition{X: 1050, Y: 1000}
				if area, ok := w.ActorTargetArea(60001, 25); !ok || !area.Contains(target.X, target.Y) {
					t.Fatal("native enemy blocked by the map's town heuristic")
				}
				moves := w.AdvanceActorsToward(60001, time.Unix(1000, 0), nil, map[uint16]ActorPosition{25: target}, func(int) int { return 0 })
				if len(moves) != 1 || moves[0].X != target.X || moves[0].Y != target.Y {
					t.Fatal("mobile field enemy did not pursue target", moves)
				}
			})
		}
	}
}

func TestMobileFieldBattleRequiresCombatAndUsesEventFallback(t *testing.T) {
	for _, tt := range []struct {
		name string
		flag byte
		op   [21]byte
		want bool
	}{
		{"matching event ID fallback", 5, [21]byte{4, 1}, true},
		{"alternate battle action", 5, [21]byte{6, 1}, true},
		{"noncombat walker", 5, [21]byte{1, 1}, false},
		{"quest actor", 4, [21]byte{4, 1}, false},
		{"other battle operation", 5, [21]byte{4, 2}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := New(&assets.Catalog{Maps: map[uint16]assets.Map{11083: {
				ID:     11083,
				NPCs:   []assets.MapNPC{{ClickID: 1, Template: 17042, Flags: tt.flag, WalkBehavior: 5, X: 1000, Y: 1000}},
				Events: []assets.Event{{ClickID: 1, Kind: 11, Branches: []assets.Branch{{Operations: []assets.Operation{{Index: 1, Data: tt.op}}}}}},
			}}})
			if got := w.RoamingBattle(11083, 1); got != tt.want {
				t.Fatalf("field battle=%v want %v", got, tt.want)
			}
		})
	}
}

func TestFieldBattleIndexFollowsEventReload(t *testing.T) {
	m := assets.Map{ID: 11083, NPCs: []assets.MapNPC{{ClickID: 1, Template: 17042, Flags: 5, WalkBehavior: 5, X: 1000, Y: 1000, Events: []byte{1}}}}
	w := New(&assets.Catalog{Maps: map[uint16]assets.Map{11083: m}})
	old, _ := w.Map(11083)
	if w.RoamingBattle(11083, 1) {
		t.Fatal("actor without a combat event was indexed")
	}
	m.Events = []assets.Event{{ClickID: 1, Kind: 11, Branches: []assets.Branch{{Operations: []assets.Operation{{Index: 1, Data: [21]byte{4, 1}}}}}}}
	w.ReloadEvents(map[uint16]assets.Map{11083: m})
	if !w.RoamingBattle(11083, 1) || old.fieldBattles[1] {
		t.Fatal("event reload did not publish an independent updated combat index")
	}
	m.Events = nil
	w.ReloadEvents(map[uint16]assets.Map{11083: m})
	if w.RoamingBattle(11083, 1) {
		t.Fatal("removed encounter remained indexed")
	}
}
