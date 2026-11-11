package server

import (
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/assetsql"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/world"
)

// The native battle op's tail dword carries a battle-mode flag in its top
// byte (0x07/0x08 on the Cave 2F guardians). Only the low 24 bits may act as
// an HP override; the flag byte must not leak into the enemy HP.
func TestQuestBattleIgnoresBattleModeTailInHPOverride(t *testing.T) {
	catalog := battleHPRepositoryCatalog(t)
	s, players, _ := worldFixture(t)
	c := players[0]
	c.ready = true
	c.character.Map = 12006
	s.Assets, s.World = catalog, world.New(catalog)
	event, ok := s.World.Event(12006, 4)
	if !ok {
		t.Fatal("missing Cave 2F guardian event")
	}
	op := world.DecodeOp(event.Branches[0].Operations[0])
	if op.DW1 != 0x07000000 {
		t.Fatalf("fixture op lost its mode tail: DW1=%#x", op.DW1)
	}
	es := &eventSession{mapID: 12006, click: 3, ev: event}
	ok, err := s.questBattle(c, es, op)
	if err != nil || !ok || c.battle == nil {
		t.Fatal("guardian battle failed to start", err)
	}
	defer s.abandonBattle(c)
	for _, f := range c.battle.b.Defenders {
		if f.MaxHP >= 0x07000000 {
			t.Fatalf("mode tail leaked into HP: template %d MaxHP %d", f.Template, f.MaxHP)
		}
		if want := storedNPCHP(catalog, f.Template); f.MaxHP != want {
			t.Fatalf("template %d HP %d, want stored NPC HP %d", f.Template, f.MaxHP, want)
		}
	}
}

// A genuinely authored HP override (positive low 24 bits) still wins over
// the stored NPC HP.
func TestQuestBattleHonorsAuthoredHPOverride(t *testing.T) {
	catalog := battleHPRepositoryCatalog(t)
	s, players, _ := worldFixture(t)
	c := players[0]
	c.ready = true
	c.character.Map = 12006
	s.Assets, s.World = catalog, world.New(catalog)
	event, ok := s.World.Event(12006, 4)
	if !ok {
		t.Fatal("missing Cave 2F guardian event")
	}
	op := world.DecodeOp(event.Branches[0].Operations[0])
	op.DW1 = 4321
	es := &eventSession{mapID: 12006, click: 3, ev: event}
	ok, err := s.questBattle(c, es, op)
	if err != nil || !ok || c.battle == nil {
		t.Fatal("guardian battle failed to start", err)
	}
	defer s.abandonBattle(c)
	if got := c.battle.b.Defenders[0].MaxHP; got != 4321 {
		t.Fatalf("authored HP override ignored: MaxHP %d, want 4321", got)
	}
}

func storedNPCHP(catalog *assets.Catalog, template uint32) int {
	if npc, ok := catalog.NPCs[uint16(template)]; ok {
		return int(npc.HP)
	}
	return 0
}

// The repository seed makes these regressions run on a fresh server checkout.
func battleHPRepositoryCatalog(t *testing.T) *assets.Catalog {
	t.Helper()
	path := filepath.Join(t.TempDir(), "assets.db")
	if _, err := assetsql.BuildRepositoryDatabase(path); err != nil {
		t.Fatal(err)
	}
	catalog, err := assetsql.LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestQuestBattleCombinedModeAndHPOverride(t *testing.T) {
	catalog := battleHPRepositoryCatalog(t)
	w := world.New(catalog)
	event, ok := w.Event(12006, 4)
	if !ok {
		t.Fatal("missing Cave 2F guardian event")
	}
	for _, test := range []struct {
		name string
		tail [4]byte
		want int
	}{
		{"zero uses template", [4]byte{0, 0, 0, 0}, 0},
		{"mode seven", [4]byte{0, 0, 0, 7}, 0},
		{"mode eight", [4]byte{0, 0, 0, 8}, 0},
		{"authored with mode seven", [4]byte{0xe1, 0x10, 0, 7}, 4321},
		{"authored with mode eight", [4]byte{0xe1, 0x10, 0, 8}, 4321},
		{"all lower bits retained", [4]byte{0xff, 0xff, 0xff, 8}, 16777215},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, players, _ := worldFixture(t)
			c := players[0]
			c.ready = true
			c.character.Map = 12006
			s.Assets, s.World = catalog, w
			// These independent raw bytes occupy EVE offsets 9..12. Decode the real
			// operation so a layout regression cannot be hidden by setting Op fields.
			raw := event.Branches[0].Operations[0]
			copy(raw.Data[9:13], test.tail[:])
			op := world.DecodeOp(raw)
			ok, err := s.questBattle(c, &eventSession{mapID: 12006, click: 3, ev: event}, op)
			if err != nil || !ok || c.battle == nil {
				t.Fatal("battle failed", err)
			}
			defer s.abandonBattle(c)
			if len(c.battle.b.Defenders) == 0 {
				t.Fatal("empty enemy formation")
			}
			for _, f := range c.battle.b.Defenders {
				want := test.want
				if want == 0 {
					want = storedNPCHP(catalog, f.Template)
				}
				if want <= 0 || f.MaxHP != want || f.HP != want {
					t.Fatalf("template %d HP=%d/%d want %d", f.Template, f.HP, f.MaxHP, want)
				}
			}
		})
	}
}
