package server

import (
	"testing"
	"wonderland-gonline/internal/world"
)

func TestInstalledXaolanForestBattleFormationAndBackground(t *testing.T) {
	const northIslandMap = 60001
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, encounter := range []struct{ event, formation uint16 }{{44, 15}, {83, 33}} {
		s, players, wires := worldFixture(t)
		c := players[0]
		c.character.Map = northIslandMap
		c.character.X, c.character.Y = 2622, 755
		s.Assets, s.World = catalog, world.New(catalog)
		ev, ok := s.World.Event(northIslandMap, encounter.event)
		if !ok {
			t.Fatal("native rescue event absent")
		}
		es := &eventSession{mapID: northIslandMap, click: 63, ev: ev}
		ok, err := s.questBattle(c, es, world.Op{Code: world.ActionBattle, D1: 1, D2: encounter.formation})
		if err != nil || !ok || c.battle == nil {
			t.Fatal("rescue battle failed", err)
		}
		want := map[uint32][2]byte{12049: {2, 1}, 12050: {2, 4}, 14155: {1, 2}}
		if len(c.battle.b.Defenders) != len(want) {
			t.Fatal("wrong rescue roster")
		}
		for _, fighter := range c.battle.b.Defenders {
			position, ok := want[fighter.ID]
			if !ok || fighter.X != position[0] || fighter.Y != position[1] {
				t.Fatalf("formation %d: template %d at (%d,%d), want %v", encounter.formation, fighter.ID, fighter.X, fighter.Y, position)
			}
		}
		found := false
		for _, p := range wires[0].packets(t) {
			if len(p) >= 4 && p[0] == 11 && p[1] == 250 {
				found = true
				if p[2] != 81 || p[3] != 0 {
					t.Fatal("rescue did not use the native position selector 81", p[:4])
				}
			}
		}
		if !found {
			t.Fatal("no native battle intro")
		}
		s.abandonBattle(c)
	}
}
