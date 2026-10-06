package server

import (
	"bytes"
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

// petFixture: on map 10017 actor 5 is story companion 14156 (Npc.dat type 4). Its
// event recruits it when absent, raises amity when present, and dismisses it on
// answer 31.
func petFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	s, players, wires := worldFixture(t)
	ev := assets.Event{ClickID: 1, Branches: []assets.Branch{
		{Index: 1, Condition: evCond(2, 2, 2, 14156, 0, 0), Operations: []assets.Operation{evOp(1, 3, 1, 14156, 0, 0)}},
		{Index: 2, Condition: evCond(2, 2, 1, 14156, 0, 0), Operations: []assets.Operation{evOp(1, 2, 5, 6, 77, 0)}},
		{Index: 3, Condition: evCond(7, 77, 30, 0, 0, 0), Operations: []assets.Operation{evOp(1, 3, 5, 14156, 0, 5<<8)}},
		{Index: 4, Condition: evCond(7, 77, 31, 0, 0, 0), Operations: []assets.Operation{evOp(1, 3, 2, 14156, 0, 0)}},
	}}
	// Amity +5 is the signed byte in dialog4's high byte.
	ev.Branches[2].Operations[0].Data[8] = 5
	s.Assets.Maps[10017] = assets.Map{ID: 10017, NPCs: []assets.MapNPC{{ClickID: 5, Flags: 1, X: 1050, Y: 1080, Template: 14156, Events: []byte{1}}}, Events: []assets.Event{ev}}
	s.Assets.NPCs = map[uint16]assets.NPC{14156: {ID: 14156, Name: "Xaolan", Type: 4, Stats: [5]uint16{8, 6, 9, 7, 10}, Skills: [3]uint16{11001}}}
	s.Assets.Skills[11001] = assets.Skill{ID: 11001, Name: "Fire Light"}
	s.World = world.New(s.Assets)
	for i := range players[:2] {
		if err := s.worldCommand(context.Background(), players[i], []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
	}
	for _, w := range wires {
		w.Reset()
	}
	return s, players, wires
}

func TestRecruitSelectAndDismissCompanion(t *testing.T) {
	s, players, wires := petFixture(t)
	ctx := context.Background()
	c, wire := players[0], wires[0]
	do := func(p ...byte) [][]byte {
		t.Helper()
		if err := s.worldCommand(ctx, c, p); err != nil {
			t.Fatal(err)
		}
		return wire.packets(t)
	}
	click := func() [][]byte { c.resumeAt = time.Time{}; return do(20, 1, 5, 0) }
	got := click()
	if !contains(got, protocol.Builder{22, 4}.Bytes(world.NPCRecord(5, 0, 1050, 1080, true))) || len(c.character.Pets) != 1 || c.pets.slot(14156) != 1 {
		t.Fatal("recruit", got)
	}
	recruit := got[2]
	if recruit[0] != 15 || recruit[1] != 1 || recruit[6] != 0x4c || recruit[7] != 0x37 || recruit[11] != 8 || recruit[19] != 10 {
		t.Fatalf("AC15:1 %v", recruit)
	}
	pet := c.character.Pets[0]
	if pet.Level != 1 || pet.Amity != 60 || pet.Base.Agility != 10 || len(pet.Skills) != 1 || pet.HP != pet.MaxHP {
		t.Fatal("new pet", pet)
	}
	// A party-only branch continues into the next eligible one: the companion's question.
	if !contains(got, eventFrame(6, 3, 5, 0, 0, 77, 1, 2)) {
		t.Fatal("follow-up question", got)
	}
	do(20, 9, 40)
	// The recruited companion's actor is now hidden, so the event cannot run from it.
	if got = click(); len(got) != 2 || !bytes.Equal(got[1], []byte{20, 8}) {
		t.Fatal("recruited actor still interactive", got)
	}
	// Selecting it for battle shows it to map peers.
	got = do(19, 1, 1)
	if !bytes.Equal(got[0], protocol.Builder{19, 1}.U32(14156)) || c.character.ActivePet != 14156 || !c.character.Pets[0].Battle {
		t.Fatal("battle pet", got)
	}
	if peer := wires[1].packets(t); len(peer) != 1 || peer[0][0] != 15 || peer[0][1] != 4 {
		t.Fatal("peer pet appearance", peer)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || len(chars[0].Pets) != 1 || chars[0].ActivePet != 14156 {
		t.Fatal("pets not persisted", err)
	}
	// A new login lists the party once and selects the battle pet.
	view, roster := world.NewView(), newPetRoster()
	entry, err := s.worldEntryPackets(chars[0], view, roster)
	if err != nil || !contains(entry, protocol.Builder{19, 1}.U32(14156)) || roster.slot(14156) != 1 {
		t.Fatal("login roster", err)
	}
	listed := 0
	for _, p := range entry {
		if p[0] == 15 && p[1] == 8 {
			listed++
		}
	}
	if listed != 1 {
		t.Fatal("pet list sent", listed)
	}
	// Dismissing a story companion keeps it in reserve.
	es := &eventSession{mapID: 10017, click: 5}
	es.ev, _ = s.World.Event(10017, 1)
	c.event = es
	if ok, err := s.dismiss(ctx, c, 14156); err != nil || !ok || len(c.character.Pets) != 0 || len(c.character.ReservePets) != 1 || c.character.ActivePet != 0 {
		t.Fatal("dismiss", err)
	}
	if got = wire.packets(t); !contains(got, protocol.Builder{15, 2}.U32(c.character.ID).U8(1)) || !contains(got, []byte{19, 2}) {
		t.Fatal("dismiss packets", got)
	}
	c.event = nil
	// Rejoining restores the reserved companion with its progress.
	c.character.ReservePets[0].Level = 7
	if ok, err := s.recruit(ctx, c, 14156); err != nil || !ok || c.character.Pets[0].Level != 7 || len(c.character.ReservePets) != 0 {
		t.Fatal("rejoin", err)
	}
}

func TestPetGrowthAndConditions(t *testing.T) {
	tpl := game.PetTemplate{Stats: game.Attributes{Strength: 1, Constitution: 9, Intelligence: 1, Wisdom: 1, Agility: 1}}
	p := game.NewPet(17100, "Boar", 1, tpl, nil)
	if p.ClientTotalExp() != 6 {
		t.Fatal(p.ClientTotalExp())
	}
	if gained := p.GainExp(14+35, tpl, true, func(total int) int {
		if total != 13 {
			t.Fatalf("growth total = %d, want 13", total)
		}
		return 1 // STR occupies roll 0; CON occupies rolls 1 through 9.
	}); gained != 2 || p.Level != 3 || p.Base.Constitution != 11 {
		t.Fatal("growth", gained, p.Level, p.Base)
	}
	if p.ClientTotalExp() != 6+14+35 {
		t.Fatal("client EXP", p.ClientTotalExp())
	}
}

func TestBattlePetEarnsExpAndEats(t *testing.T) {
	s, c, _ := battleFixture(t, 1)
	ctx := context.Background()
	s.worldMu.Lock()
	s.abandonBattle(c)
	s.worldMu.Unlock()
	c.event = nil
	s.Assets.NPCs[14156] = assets.NPC{ID: 14156, Name: "Xaolan", Type: 4, Stats: [5]uint16{5, 5, 5, 5, 5}}
	s.Assets.Items[29001] = game.ItemDefinition{ID: 29001, Type: 23, Status: [2]uint16{64, 0}, Values: [2]int32{110, 0}}
	s.World = world.New(s.Assets)
	if ok, err := s.recruit(ctx, c, 14156); err != nil || !ok {
		t.Fatal(err)
	}
	if err := s.worldCommand(ctx, c, []byte{19, 1, 1}); err != nil {
		t.Fatal(err)
	}
	// A one-HP wild monster: player and pet each send a command.
	s.worldMu.Lock()
	err := s.startBattle(c, &battleRun{}, []battle.Enemy{{Template: 10500, Name: "Slime", Level: 5, HP: 1}})
	s.worldMu.Unlock()
	if err != nil || c.battle.members[0].pet == nil {
		t.Fatal("battle pet missing", err)
	}
	if err := s.worldCommand(ctx, c, []byte{50, 4, 3, 2, 0, 0}); err != nil || c.battle.b.Processing {
		t.Fatal("round ran before the player's command", err)
	}
	if err := s.worldCommand(ctx, c, []byte{50, 1, 4, 2, 2, 2}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	if pet := c.character.Pets[0]; pet.Level != 3 || pet.Exp != 75-14-35 {
		t.Fatal("pet EXP", pet.Level, pet.Exp)
	}
	// Pet food raises amity by its value above 100, using only what reaches 100.
	c.character.Pets[0].Amity = 85
	c.character.Bag[8] = game.Item{ID: 29001, Count: 5}
	if err := s.worldCommand(ctx, c, []byte{23, 15, 9, 5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	if c.character.Pets[0].Amity != 100 || c.character.Bag[8].Count != 3 {
		t.Fatal("feeding", c.character.Pets[0].Amity, c.character.Bag[8].Count)
	}
}
