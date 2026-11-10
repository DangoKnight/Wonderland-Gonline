package server

import (
	"testing"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/world"
)

// The native battle op's tail dword carries a battle-mode flag in its top
// byte (0x07/0x08 on the Cave 2F guardians). Only the low 24 bits may act as
// an HP override; the flag byte must not leak into the enemy HP.
func TestQuestBattleIgnoresBattleModeTailInHPOverride(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
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
	ok, err = s.questBattle(c, es, op)
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
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
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
	ok, err = s.questBattle(c, es, op)
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
