package server

import (
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/world"
)

func TestBattleBackgroundUsesModularPolicy(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	policy := assets.StoryRule{ID: "operator_room", Kind: assets.StoryBattleBackground, Scope: assets.StoryScope{Maps: []uint16{12002}, Events: []uint16{5}, Branches: []byte{3}}, Value: 140}
	s.Assets.Story.Rules = []assets.StoryRule{policy}
	s.World = world.New(s.Assets)
	c.character.Map = 12002
	run := &battleRun{event: &eventSession{mapID: 12002, ev: &assets.Event{ClickID: 5, Branches: []assets.Branch{{Index: 3}, {Index: 6}}}}}
	if got := s.battleBackground(c, run); got != 140 {
		t.Fatal("authored backdrop ignored", got)
	}
	run.event.branch = 1
	if got := s.battleBackground(c, run); got != 1 {
		t.Fatal("branch scope leaked", got)
	}
	run.event.branch = 0
	run.event.mapID = 12004
	if got := s.battleBackground(c, run); got != 1 {
		t.Fatal("map scope leaked", got)
	}
	if got := s.battleBackground(c, nil); got != 1 {
		t.Fatal("event scope applied to non-event battle", got)
	}
}

func TestInstalledXaolanBattleBackdropPacket(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	s, players, wires := worldFixture(t)
	c := players[0]
	c.character.Map = 12002
	s.Assets, s.World = catalog, world.New(catalog)
	event, ok := s.World.Event(12002, 5)
	if !ok {
		t.Fatal("missing Xaolan event")
	}
	es := &eventSession{mapID: 12002, click: 2, ev: event}
	ok, err = s.questBattle(c, es, world.Op{Code: world.ActionBattle, D1: 1, D2: 1})
	if err != nil || !ok || c.battle == nil {
		t.Fatal("Xaolan battle failed to start", err)
	}
	defer s.abandonBattle(c)
	found := false
	for _, packet := range wires[0].packets(t) {
		if len(packet) >= 4 && packet[0] == 11 && packet[1] == 250 {
			found = true
			if packet[2] != 140 || packet[3] != 0 {
				t.Fatal("wrong house backdrop on native wire", packet[:4])
			}
		}
	}
	if !found {
		t.Fatal("no formation packet")
	}
}
