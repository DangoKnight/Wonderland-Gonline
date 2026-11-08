package server

import (
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/world"
)

func TestPositionBackdropsAndExplicitStoryOverride(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	c.ready = true
	c.character.Map, c.character.X, c.character.Y = 60001, 2202, 1395
	s.Assets.BattleBackdrops = map[uint16]assets.BattleBackdrop{60001: {Default: 80, Regions: []assets.BattleBackdropRegion{{Selector: 91, Left: 1899, Top: 1165, Right: 2844, Bottom: 1442}}}}
	if got := s.battleBackground(c, nil); got != 91 {
		t.Fatal(got)
	}
	c.character.X, c.character.Y = 300, 300
	if got := s.battleBackground(c, nil); got != 80 {
		t.Fatal(got)
	}
	// Explicit operator policy remains a supported override of native geography.
	s.Assets.Story = assets.DefaultStoryDefinitions()
	s.Assets.Story.Rules = append(s.Assets.Story.Rules, assets.StoryRule{ID: "operator_backdrop", Kind: assets.StoryBattleBackground, Scope: assets.StoryScope{Maps: []uint16{60001}}, Value: 50})
	s.World = world.New(s.Assets)
	if got := s.battleBackground(c, nil); got != 50 {
		t.Fatal(got)
	}
}

func TestInstalledNorthIslandSeaflowerBackdropPackets(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	const northIslandCoastMap = 60000
	for _, spawn := range []struct {
		actor, event uint16
		template     uint32
	}{
		{26, 36, 17416}, {27, 38, 17106}, {28, 40, 17418},
		{29, 37, 17417}, {30, 41, 17107}, {31, 42, 17100}, {32, 43, 17101},
	} {
		s, players, wires := worldFixture(t)
		c := players[0]
		c.ready = true
		c.character.Map = northIslandCoastMap
		c.character.X, c.character.Y = 1694, 963 // The player's shore position, independent of NPC movement.
		s.Assets, s.World = catalog, world.New(catalog)
		foundSpawn := false
		for _, actor := range catalog.Maps[northIslandCoastMap].NPCs {
			if actor.ClickID == spawn.actor && actor.Template == spawn.template {
				foundSpawn = true
			}
		}
		if !foundSpawn {
			t.Fatal("native seaflower spawn missing", spawn)
		}
		ev, ok := s.World.Event(northIslandCoastMap, spawn.event)
		if !ok {
			t.Fatal("native coastal encounter missing", spawn.event)
		}
		es := &eventSession{mapID: northIslandCoastMap, click: spawn.actor, ev: ev}
		ok, err := s.questBattle(c, es, world.DecodeOp(ev.Branches[0].Operations[0]))
		if err != nil || !ok || c.battle == nil {
			t.Fatal("coastal battle failed", spawn, err)
		}
		foundPacket := false
		for _, p := range wires[0].packets(t) {
			if len(p) >= 4 && p[0] == 11 && p[1] == 250 {
				foundPacket = true
				if p[2] != 50 || p[3] != 0 {
					t.Fatal("seaflower fight did not select Beach", spawn, p[:4])
				}
			}
		}
		s.abandonBattle(c)
		if !foundPacket {
			t.Fatal("no coastal formation packet", spawn)
		}
	}
}

func TestInstalledLakesideMushroomBackdropPacket(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	s, players, wires := worldFixture(t)
	c := players[0]
	c.ready = true
	const northIslandMap = 60001
	c.character.Map = northIslandMap
	c.character.X, c.character.Y = 2202, 1395
	s.Assets, s.World = catalog, world.New(catalog)
	ev, ok := s.World.Event(northIslandMap, 65)
	if !ok {
		t.Fatal("missing lakeside mushroom event")
	}
	es := &eventSession{mapID: northIslandMap, click: 40, ev: ev}
	ok, err = s.questBattle(c, es, world.DecodeOp(ev.Branches[0].Operations[0]))
	if err != nil || !ok || c.battle == nil {
		t.Fatal("mushroom battle failed", err)
	}
	defer s.abandonBattle(c)
	for _, p := range wires[0].packets(t) {
		if len(p) >= 4 && p[0] == 11 && p[1] == 250 {
			if p[2] != 91 || p[3] != 0 {
				t.Fatal("incorrect lake formation header", p[:4])
			}
			return
		}
	}
	t.Fatal("no native formation packet")
}
