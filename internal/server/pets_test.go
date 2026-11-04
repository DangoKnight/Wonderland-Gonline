package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/world"
)

func TestCompanionAltarRebirthCommitAndReplay(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	c.character.Pets[1].Amity = 60
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	before := c.character.Clone()
	if err := s.openService(c, world.ServiceCompanionRebirth); err != nil {
		t.Fatal(err)
	}
	if got := wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], []byte{44, 2}) {
		t.Fatal(got)
	}
	if err := s.dispatch(context.Background(), c, []byte{44, 1, 2}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) < 3 || !bytes.Equal(got[0], []byte{44, 1, 0}) {
		t.Fatal(got)
	}
	p := c.character.Pets[1]
	if !p.Reborn || p.Level != 1 || p.Exp != 0 || p.StatPoints != before.Pets[1].StatPoints || p.Amity != 60 || !reflect.DeepEqual(p.Skills, before.Pets[1].Skills) {
		t.Fatal(p)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || !reflect.DeepEqual(saved[0].Pets, c.character.Pets) {
		t.Fatal("durability", err)
	}
	if err := s.dispatch(context.Background(), c, []byte{44, 1, 2}); err != nil {
		t.Fatal(err)
	}
	if got := wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], []byte{44, 1, 255}) {
		t.Fatal("replay", got)
	}
}

func TestCompanionAltarRebirthRejectionsAndRollback(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	for _, tc := range []struct {
		mutate func(*game.Pet)
		code   byte
	}{
		{func(p *game.Pet) { p.Level = 99 }, 3},
		{func(p *game.Pet) { p.Reborn = true }, 6},
		{func(p *game.Pet) { p.Amity = 59 }, 7},
		{func(p *game.Pet) { p.Equipment[0] = game.Item{ID: 123, Count: 1} }, 8},
	} {
		original := c.character.Clone()
		tc.mutate(&c.character.Pets[1])
		before := c.character.Clone()
		c.view.CompanionRebirthOpen = true
		if err := s.dispatch(context.Background(), c, []byte{44, 1, 2}); err != nil {
			t.Fatal(err)
		}
		if got := wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], []byte{44, 1, tc.code}) || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("rejection", got)
		}
		*c.character = original
	}
	before := c.character.Clone()
	s.Store.Close()
	if err := s.dispatch(context.Background(), c, []byte{44, 1, 2}); err == nil || wires[0].Len() != 0 || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("failed save published", err)
	}
}

func mountFixture(t *testing.T) (*Server, *Session, []*captureConn) {
	s, players, wires := petFixture(t)
	c := players[0]
	next := c.character.Clone()
	next.Pets = []game.Pet{{ID: 14156, Slot: 2, Name: "Xaolan", Amity: 80}, {ID: 12032, Slot: 3, Name: "Robinson", Amity: 80}}
	c.pets.register(14156)
	c.pets.register(12032)
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	return s, c, wires
}

func TestMountPacketsPersistenceAndRest(t *testing.T) {
	s, c, wires := mountFixture(t)
	ctx := context.Background()
	command := protocol.Builder{15, 11, 2}.U32(12178)
	if err := s.worldCommand(ctx, c, command); err != nil {
		t.Fatal(err)
	}
	want := serverwire.MountPacket(c.character.ID, 12178, 3)
	if len(want) != 37 || !bytes.Equal(want[:11], protocol.Builder{15, 16, 3}.U32(c.character.ID).U32(12178)) {
		t.Fatal("mount wire layout", want)
	}
	for _, b := range want[11:] {
		if b != 0 {
			t.Fatal("reserved bytes")
		}
	}
	if !contains(wires[0].packets(t), want) || !contains(wires[1].packets(t), want) {
		t.Fatal("mount not replicated")
	}
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || stored[0].ActiveMount != 12178 {
		t.Fatal("mount not durable", err)
	}
	if !contains(s.peerPetPackets(c.character), serverwire.MountPacket(c.character.ID, 12178, 1)) {
		t.Fatal("peer snapshot mount missing")
	}
	if !contains(s.rosterPackets(c.character, newPetRoster()), want) {
		t.Fatal("owner snapshot mount missing")
	}
	if err = s.worldCommand(ctx, c, command); err != nil || wires[0].Len() != 0 {
		t.Fatal("mount replay", err)
	}
	if err = s.worldCommand(ctx, c, protocol.Builder{15, 12, 1}.U32(14156)); err != nil || c.character.ActiveMount != 12178 || wires[0].Len() != 0 {
		t.Fatal("stale rest unmounted another pet", err)
	}
	if err = s.worldCommand(ctx, c, protocol.Builder{15, 12, 2}.U32(12032)); err != nil || c.character.ActiveMount != 0 {
		t.Fatal("rest failed", err)
	}
	if !contains(wires[0].packets(t), protocol.Builder{15, 17}.U32(c.character.ID)) {
		t.Fatal("rest wire")
	}
	stored, err = s.Store.Characters(ctx, c.account.ID)
	if err != nil || stored[0].ActiveMount != 0 {
		t.Fatal("rest not durable", err)
	}
}

func TestMountRefusalsAndSaveFailure(t *testing.T) {
	s, c, wires := mountFixture(t)
	ctx := context.Background()
	for _, command := range [][]byte{protocol.Builder{15, 11, 1}.U32(12032), protocol.Builder{15, 11, 2}.U32(99999)} {
		if err := s.worldCommand(ctx, c, command); err != nil || c.character.ActiveMount != 0 || wires[0].Len() != 0 || wires[1].Len() != 0 {
			t.Fatal("invalid mount accepted", err)
		}
	}
	command := protocol.Builder{15, 11, 2}.U32(12032)
	for i := 2; i < len(command); i++ {
		if err := s.mountCommand(ctx, c, command[:i]); err == nil {
			t.Fatal("truncation accepted", i)
		}
	}
	s.Store.Close()
	if err := s.worldCommand(ctx, c, command); err == nil || c.character.ActiveMount != 0 || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("failed mount save published state", err)
	}
}

func TestMountedPetHotelAndDismissCleanup(t *testing.T) {
	for _, hotel := range []bool{false, true} {
		s, c, wires := mountFixture(t)
		ctx := context.Background()
		if err := s.worldCommand(ctx, c, protocol.Builder{15, 11, 2}.U32(12032)); err != nil {
			t.Fatal(err)
		}
		for _, w := range wires {
			w.Reset()
		}
		if hotel {
			if err := s.hotelCommand(ctx, c, []byte{31, 3, 2}); err != nil {
				t.Fatal(err)
			}
		} else {
			if _, err := s.dismiss(ctx, c, 12032); err != nil {
				t.Fatal(err)
			}
		}
		if c.character.ActiveMount != 0 || !contains(wires[0].packets(t), protocol.Builder{15, 17}.U32(c.character.ID)) || !contains(wires[1].packets(t), protocol.Builder{15, 17}.U32(c.character.ID)) {
			t.Fatal("removed mount remained visible")
		}
	}
}

func TestMountRestoredOnWorldEntry(t *testing.T) {
	s, c, wires := mountFixture(t)
	ctx := context.Background()
	if err := s.worldCommand(ctx, c, protocol.Builder{15, 11, 2}.U32(12032)); err != nil {
		t.Fatal(err)
	}
	id := c.character.ID
	c.character = nil
	for _, w := range wires {
		w.Reset()
	}
	if err := s.dispatch(ctx, c, []byte{63, 2, 1}); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveMount != 12178 || !contains(wires[0].packets(t), serverwire.MountPacket(id, 12178, 3)) {
		t.Fatal("mount not restored on login")
	}
}

func TestPetControlsReachWorldThroughDispatcher(t *testing.T) {
	s, c, wires := mountFixture(t)
	ctx := context.Background()
	if err := s.dispatch(ctx, c, protocol.Builder{15, 11, 1}.U32(14156)); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveMount != 14156 || !contains(wires[0].packets(t), serverwire.MountPacket(c.character.ID, 14156, 2)) {
		t.Fatal("dispatcher did not route companion mount")
	}
	if err := s.dispatch(ctx, c, append([]byte{15, 6, 1}, []byte("New Name")...)); err != nil {
		t.Fatal(err)
	}
	if c.character.Pets[0].Name != "New Name" {
		t.Fatal("dispatcher did not route pet rename")
	}
	if err := s.dispatch(ctx, c, []byte{15, 2, 1}); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.character.Pet(14156); ok || c.character.ActiveMount != 0 {
		t.Fatal("dispatcher did not route release")
	}
}

func TestRenamePetRawASCIIAndReplication(t *testing.T) {
	s, c, wires := mountFixture(t)
	ctx := context.Background()
	c.character.ActivePet = 14156
	c.character.Pets[0].Battle = true
	command := append([]byte{15, 6, 1}, []byte("  My Companion Name That Is Long  \x00\x00")...)
	if err := s.worldCommand(ctx, c, command); err != nil {
		t.Fatal(err)
	}
	name := "My Companion Nam"
	if c.character.Pets[0].Name != name || !contains(wires[0].packets(t), serverwire.PetNamePacket(c.character.ID, 1, name)) || !contains(wires[1].packets(t), serverwire.PetMapPacket(c.character.ID, 14156, name)) {
		t.Fatal("rename wire or trimming", c.character.Pets[0].Name)
	}
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || stored[0].Pets[0].Name != name {
		t.Fatal("rename not durable", err)
	}
	if err = s.worldCommand(ctx, c, []byte{15, 6, 2, 'A', 255}); err != nil || c.character.Pets[1].Name != "A?" {
		t.Fatal("ASCII replacement", err)
	}
}

func TestReleaseMountedBattlePetIsPermanent(t *testing.T) {
	s, c, wires := mountFixture(t)
	ctx := context.Background()
	c.character.ActivePet = 12178
	c.character.Pets[1].Battle = true
	if err := s.worldCommand(ctx, c, protocol.Builder{15, 11, 2}.U32(12032)); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	if err := s.worldCommand(ctx, c, []byte{15, 2, 2}); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || len(stored[0].Pets) != 1 || len(stored[0].ReservePets) != 0 || stored[0].ActiveMount != 0 || stored[0].ActivePet != 0 || c.pets.slot(12032) != 0 {
		t.Fatal("release not durable/permanent", err)
	}
	for _, wire := range wires[:2] {
		packets := wire.packets(t)
		if !contains(packets, protocol.Builder{15, 17}.U32(c.character.ID)) || !contains(packets, protocol.Builder{15, 2}.U32(c.character.ID).U8(2)) {
			t.Fatal("released pet not removed from client", packets)
		}
	}
}

func TestReleaseBattlePetPreservesAnotherMount(t *testing.T) {
	s, c, wires := mountFixture(t)
	ctx := context.Background()
	c.character.ActivePet = 14156
	c.character.Pets[0].Battle = true
	if err := s.worldCommand(ctx, c, protocol.Builder{15, 11, 2}.U32(12032)); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	if err := s.worldCommand(ctx, c, []byte{15, 2, 1}); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveMount != 12178 {
		t.Fatal("another mount removed")
	}
	for _, wire := range wires[:2] {
		packets := wire.packets(t)
		if contains(packets, protocol.Builder{15, 17}.U32(c.character.ID)) || contains(packets, protocol.Builder{5, 8}.U32(c.character.ID).U32(0)) {
			t.Fatal("another mount's appearance erased")
		}
	}
}

func TestPetControlRefusalsAndSaveFailure(t *testing.T) {
	s, c, wires := mountFixture(t)
	ctx := context.Background()
	for _, command := range [][]byte{{15, 2, 0}, {15, 2, 4}, {15, 6, 0, 'A'}, {15, 6, 4, 'A'}, {15, 6, 1, ' ', ' '}, {15, 6, 1, 'A', 0, 'B'}} {
		if err := s.worldCommand(ctx, c, command); err != nil || len(c.character.Pets) != 2 || c.character.Pets[0].Name != "Xaolan" || wires[0].Len() != 0 || wires[1].Len() != 0 {
			t.Fatal("invalid pet control mutated state", command, err)
		}
	}
	for _, command := range [][]byte{{15, 2}, {15, 2, 1, 0}, {15, 6}, {15, 6, 1}} {
		if err := s.worldCommand(ctx, c, command); err == nil {
			t.Fatal("malformed pet control accepted", command)
		}
	}
	s.Store.Close()
	for _, command := range [][]byte{{15, 6, 1, 'A'}, {15, 2, 1}} {
		if err := s.worldCommand(ctx, c, command); err == nil || len(c.character.Pets) != 2 || c.pets.slot(14156) != 1 || c.character.Pets[0].Name != "Xaolan" || wires[0].Len() != 0 || wires[1].Len() != 0 {
			t.Fatal("failed pet control save published state", err)
		}
	}
}

func TestPetEquipmentAndAllocationPersistence(t *testing.T) {
	s, players, wires := petFixture(t)
	c, wire := players[0], wires[0]
	ctx := context.Background()
	next := c.character.Clone()
	pet := game.NewPet(14156, "Xaolan", 3, s.petTemplate(14156), s.Assets.Items)
	pet.StatPoints = 3
	pet.Equipment[2] = game.Item{ID: 10063, Count: 1, Damage: 7}
	next.Pets = []game.Pet{pet}
	next.Bag[8] = game.Item{ID: 10064, Count: 1, Damage: 4, Metadata: [26]byte{18: 2}}
	s.Assets.Items[10064] = game.ItemDefinition{ID: 10064, EquipSlot: 3, Status: [2]uint16{210, 207}, Values: [2]int32{120, 110}}
	s.Assets.Items[10063] = game.ItemDefinition{ID: 10063, EquipSlot: 3}
	c.pets.register(14156) // client slot 1 differs from persistent party slot 3.
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	do := func(p ...byte) [][]byte {
		t.Helper()
		if err := s.worldCommand(ctx, c, p); err != nil {
			t.Fatal(err)
		}
		return wire.packets(t)
	}
	if got := do(23, 17, 1, 9); !contains(got, []byte{23, 23, 1, 9}) {
		t.Fatal(got)
	}
	if c.character.Pets[0].Equipment[2].ID != 10064 || c.character.Bag[8].Damage != 7 {
		t.Fatal("swap lost metadata")
	}
	before := c.character.Clone()
	if got := do(23, 18, 1, 3, 9); len(got) != 0 || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("occupied destination accepted")
	}
	if got := do(23, 18, 1, 3, 10); !contains(got, []byte{23, 22, 1, 3, 10}) || c.character.Bag[9].Forge() != 2 {
		t.Fatal("remove", got)
	}
	do(8, 2, 1, 28, 2)
	if c.character.Pets[0].StatPoints != 1 || c.character.Pets[0].Base.Strength != pet.Base.Strength+2 {
		t.Fatal("wrong pet allocated")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || !reflect.DeepEqual(chars[0].Pets, c.character.Pets) || chars[0].Bag[9] != c.character.Bag[9] {
		t.Fatal("state did not persist", err)
	}
}

func feedingFixture(t *testing.T) (*Server, *Session, []*captureConn) {
	t.Helper()
	s, c, wires := rebirthFixture(t)
	s.Assets.Items[50000] = game.ItemDefinition{ID: 50000, Type: 23, Status: [2]uint16{64, 64}, Values: [2]int32{107, 103}}
	next := c.character.Clone()
	next.Pets[1].Amity = 95
	next.Bag[8] = game.Item{ID: 50000, Count: 5, Damage: 7, Metadata: [26]byte{18: 2}}
	next.Bag[16] = game.Item{ID: 50000, Count: 3}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	return s, c, wires
}

func TestPetFeedingPacketsConsumptionAndPersistence(t *testing.T) {
	s, c, wires := feedingFixture(t)
	before := c.character.Clone()
	// Native request is a WORD item ID, not either the bag or party slot.
	if err := s.dispatch(context.Background(), c, []byte{67, 1, 0x50, 0xc3}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) != 5 || !bytes.Equal(got[0], []byte{23, 9, 9, 1}) || !bytes.Equal(got[len(got)-1], []byte{67, 1, 0x50, 0xc3, 1}) || !contains(got, protocol.Builder{8, 2, 4, 2, 0, 64, 1}.U32(100).U32(0)) || !contains(got, []byte{23, 15}) {
		t.Fatal("feeding wire packets", got)
	}
	if c.character.Pets[1].Amity != 100 || c.character.Bag[8].Count != 4 || c.character.Bag[8].Metadata != before.Bag[8].Metadata || c.character.Bag[8].Damage != 7 || c.character.Bag[16] != before.Bag[16] || !reflect.DeepEqual(c.character.Pets[0], before.Pets[0]) {
		t.Fatal("wrong food/pet consumed", c.character.Pets, c.character.Bag)
	}
	if wires[1].Len() != 0 {
		t.Fatal("feeding emitted peer packets")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || !reflect.DeepEqual(saved[0].Pets, c.character.Pets) || saved[0].Bag != c.character.Bag {
		t.Fatal("feeding not durable", err)
	}
	// Feeding at the cap fails without wasting another food unit.
	after := c.character.Clone()
	if err := s.dispatch(context.Background(), c, []byte{67, 1, 0x50, 0xc3}); err != nil {
		t.Fatal(err)
	}
	got = wires[0].packets(t)
	if len(got) != 2 || !bytes.Equal(got[1], []byte{67, 1, 0x50, 0xc3, 0}) || !reflect.DeepEqual(after, *c.character) {
		t.Fatal("cap consumed food", got)
	}
}

func TestPetFeedingInvalidTargetsFoodAndSaveFailure(t *testing.T) {
	s, c, wires := feedingFixture(t)
	before := c.character.Clone()
	for _, p := range [][]byte{{67}, {67, 1}, {67, 1, 0x50}, {67, 1, 0x50, 0xc3, 0}, {67, 2, 0x50, 0xc3}} {
		if err := s.dispatch(context.Background(), c, p); err == nil || wires[0].Len() != 0 || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("malformed feeding changed state", p, err)
		}
	}
	for _, change := range []func(){
		func() { c.character.Pets[1].Battle = false },
		func() { c.pets.slots = map[uint32]byte{} },
		func() { c.character.Bag[8], c.character.Bag[16] = game.Item{}, game.Item{} },
		func() { s.Assets.Items[50000] = game.ItemDefinition{ID: 50000} },
	} {
		change()
		snapshot := c.character.Clone()
		if err := s.dispatch(context.Background(), c, []byte{67, 1, 0x50, 0xc3}); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		if len(got) != 2 || !bytes.Equal(got[1], []byte{67, 1, 0x50, 0xc3, 0}) || !reflect.DeepEqual(snapshot, *c.character) {
			t.Fatal("invalid feeding succeeded", got)
		}
		*c.character = before.Clone()
		c.pets = newPetRoster()
		for _, p := range c.character.Pets {
			c.pets.register(p.ID)
		}
		s.Assets.Items[50000] = game.ItemDefinition{ID: 50000, Type: 23, Status: [2]uint16{64, 64}, Values: [2]int32{107, 103}}
	}
	for _, food := range []uint16{0, 49999} {
		if err := s.dispatch(context.Background(), c, protocol.Builder{67, 1}.U16(food)); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		if len(got) != 2 || !bytes.Equal(got[1], protocol.Builder{67, 1}.U16(food).U8(0)) || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("missing food mutated state", got)
		}
	}
	s.Store.Close()
	if err := s.dispatch(context.Background(), c, []byte{67, 1, 0x50, 0xc3}); err == nil || !reflect.DeepEqual(before, *c.character) || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("failed save consumed/published food", err)
	}
}

func TestPetFeedingWorldGatesAndSharedInputBounds(t *testing.T) {
	s, c, wires := feedingFixture(t)
	before := c.character.Clone()
	// Shared feeding is safe even when an internal caller supplies bad inputs.
	for _, request := range [][3]byte{{0, 1, 2}, {51, 1, 2}, {9, 0, 2}, {9, 1, 0}, {9, 1, 255}} {
		if ok, err := s.feedPet(context.Background(), c, request[0], request[1], request[2]); ok || err != nil || !reflect.DeepEqual(before, *c.character) || wires[0].Len() != 0 {
			t.Fatal("bad shared feed inputs", request, ok, err)
		}
	}
	c.ready = false
	if err := s.dispatch(context.Background(), c, []byte{67, 1, 0x50, 0xc3}); !errors.Is(err, protocol.ErrMalformed) {
		t.Fatal("feeding during loading", err)
	}
	c.warped = true
	if err := s.dispatch(context.Background(), c, []byte{67, 1, 0x50, 0xc3}); err != nil {
		t.Fatal(err)
	}
	c.ready = true
	for _, set := range []func(){
		func() { c.battle = &battleRun{} }, func() { c.event = &eventSession{} },
		func() { c.storm = true }, func() { c.beach = &beachRun{} }, func() { c.trade = &tradeSession{} },
	} {
		set()
		if err := s.dispatch(context.Background(), c, []byte{67, 1, 0x50, 0xc3}); err != nil || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("locked feeding consumed item", err)
		}
		for _, packet := range wires[0].packets(t) {
			if len(packet) > 0 && packet[0] == 67 {
				t.Fatal("locked feeding emitted receipt", packet)
			}
		}
		c.battle, c.event, c.trade = nil, nil, nil
		c.storm = false
		c.beach = nil
	}
}

func TestPetGrowthStartupFormulaReachesEXPGrants(t *testing.T) {
	s, _, _ := worldFixture(t)
	s.Assets.Items[42] = game.ItemDefinition{ID: 42, Status: [2]uint16{214, 0}, Values: [2]int32{140, 0}}
	for _, tc := range []struct {
		formula game.PetGrowthFormula
		total   int
	}{{game.PetGrowthBaseStats, 25}, {game.PetGrowthCombatStats, 108}} {
		cfg := config.Default()
		cfg.PetGrowthFormula = tc.formula
		selected := New(cfg, s.Store, s.Assets, s.Log)
		p := game.Pet{ID: 17100, Level: 1, Base: game.Attributes{Strength: 5, Constitution: 5, Intelligence: 5, Wisdom: 5, Agility: 5}}
		p.Equipment[0] = game.Item{ID: 42, Count: 1}
		selected.worldMu.Lock()
		levels := selected.gainPetExp(&p, 14, func(total int) int {
			if total != tc.total {
				t.Errorf("%s weights=%d, want %d", tc.formula, total, tc.total)
			}
			return total - 1
		})
		selected.worldMu.Unlock()
		if levels != 1 || p.Base.Agility != 6 {
			t.Fatal(p, levels)
		}
	}
}

func TestPetGrowthConfigurationSaveRequiresRestart(t *testing.T) {
	s, _, _ := worldFixture(t)
	cfg := s.Config
	path := filepath.Join(t.TempDir(), "config.json")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s.SetConfigurationPath(path)
	snapshot, err := s.StartupConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	next := snapshot.Configuration
	next.PetGrowthFormula = game.PetGrowthCombatStats
	if err = s.SaveStartupConfiguration(snapshot.Version, next); err != nil {
		t.Fatal(err)
	}
	if s.Config.PetGrowthFormula != game.PetGrowthBaseStats || s.petGrowthFormula != game.PetGrowthBaseStats {
		t.Fatal("configuration save changed running formula")
	}
	loaded, err := config.Load(path)
	if err != nil || loaded.PetGrowthFormula != game.PetGrowthCombatStats {
		t.Fatal("formula not saved", err)
	}
	settings := s.RuntimeSettings()
	settings.ExpRate = 2
	if err = s.UpdateRuntimeSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	if err = s.LoadRuntimeSettings(context.Background()); err != nil || s.Config.PetGrowthFormula != game.PetGrowthBaseStats {
		t.Fatal("live settings changed formula", err)
	}
	raw, err := json.Marshal(s.RuntimeSettings())
	if err != nil || strings.Contains(string(raw), "pet_growth_formula") {
		t.Fatal("formula exposed as live setting", err)
	}
	if err = s.Store.SaveSettings(context.Background(), map[string]string{"pet_growth_formula": "combat_stats"}); err == nil {
		t.Fatal("formula accepted as live database setting")
	}
}

func rebirthFixture(t *testing.T) (*Server, *Session, []*captureConn) {
	t.Helper()
	s, c, wires := mountFixture(t)
	next := c.character.Clone()
	pet := &next.Pets[1] // Persistent slot 3 is client slot 2; Robinson alias 12178.
	pet.Level, pet.Amity, pet.Exp, pet.StatPoints = 100, 100, 123, 7
	pet.Base = game.Attributes{Strength: 40, Constitution: 60, Intelligence: 30, Wisdom: 50, Agility: 70}
	pet.Skills = []game.PetSkill{{ID: 11001, Grade: 5, Exp: 73}}
	pet.Battle = true
	pet.Normalize(s.Assets.Items, false)
	next.ActivePet = 12178
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	return s, c, wires
}

func TestPetRebirthPacketsPersistenceAndReconnect(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	before := c.character.Clone()
	if err := s.dispatch(context.Background(), c, []byte{69, 1, 2}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) < 3 || !bytes.Equal(got[0], []byte{69, 1, 2, 1}) || !bytes.Equal(got[1][:2], []byte{15, 8}) || !contains(got, protocol.Builder{8, 2, 4, 2, 0, 35, 1}.U32(1).U32(0)) || !contains(got, protocol.Builder{8, 2, 4, 2, 0, 38, 1}.U32(57).U32(0)) || !contains(got, protocol.Builder{8, 2, 4, 2, 0, 36, 1}.U32(6).U32(0)) {
		t.Fatal("rebirth wire refresh", got)
	}
	if !bytes.Equal(got[1][len(got[1])-11:], []byte{0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatal("reborn flag missing from roster", got[1])
	}
	pet := c.character.Pets[1]
	if !pet.Reborn || pet.Level != 1 || pet.Exp != 0 || pet.StatPoints != 57 || pet.HP != pet.MaxHP || pet.SP != pet.MaxSP || pet.Slot != 3 || c.character.ActivePet != 12178 || !pet.Battle || !reflect.DeepEqual(c.character.Pets[0], before.Pets[0]) {
		t.Fatal("wrong pet/state", c.character.Pets)
	}
	if wires[1].Len() != 0 {
		t.Fatal("unnecessary peer packet")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(saved) != 1 || !reflect.DeepEqual(saved[0].Pets, c.character.Pets) {
		t.Fatal("rebirth not saved", err)
	}
	roster := newPetRoster()
	restored := saved[0].Clone()
	for _, p := range restored.Pets {
		roster.register(p.ID)
	}
	if !bytes.Equal(s.petListPacket(&restored, roster), got[1]) {
		t.Fatal("reconnect rebirth list differs")
	}
	after := c.character.Clone()
	if err := s.dispatch(context.Background(), c, []byte{69, 1, 2}); err != nil {
		t.Fatal(err)
	}
	reply := wires[0].packets(t)
	if len(reply) == 0 || !bytes.Equal(reply[0], []byte{69, 1, 2, 0}) || !reflect.DeepEqual(after, *c.character) {
		t.Fatal("repeat ascension granted bonus", reply)
	}
}

func TestPetRebirthRejectsInvalidRequestsAndSaveFailure(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	for _, p := range [][]byte{{69}, {69, 1}, {69, 1, 2, 0}, {69, 2, 2}} {
		before := c.character.Clone()
		if err := s.dispatch(context.Background(), c, p); err == nil || !reflect.DeepEqual(before, *c.character) || wires[0].Len() != 0 {
			t.Fatal("bad request mutated state", p, err)
		}
	}
	for _, slot := range []byte{0, 4, 255} {
		if err := s.dispatch(context.Background(), c, []byte{69, 1, slot}); err != nil {
			t.Fatal(err)
		}
		if got := wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], []byte{69, 1, slot, 0}) {
			t.Fatal("unknown client slot", got)
		}
	}
	for _, change := range []func(*game.Pet){
		func(p *game.Pet) { p.Level = 99 }, func(p *game.Pet) { p.Amity = 99 },
		func(p *game.Pet) { p.Reborn = true }, func(p *game.Pet) { p.StatPoints = math.MaxUint16 },
	} {
		original := c.character.Clone()
		change(&c.character.Pets[1])
		before := c.character.Clone()
		if err := s.dispatch(context.Background(), c, []byte{69, 1, 2}); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		if len(got) == 0 || !bytes.Equal(got[0], []byte{69, 1, 2, 0}) || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("ineligible pet ascended", got)
		}
		*c.character = original
	}
	before := c.character.Clone()
	s.Store.Close()
	if err := s.dispatch(context.Background(), c, []byte{69, 1, 2}); err == nil || !reflect.DeepEqual(before, *c.character) || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("failed save published rebirth", err)
	}
}

func TestPetRebirthRespectsWorldInteractionGates(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	before := c.character.Clone()
	c.ready = false
	if err := s.dispatch(context.Background(), c, []byte{69, 1, 2}); !errors.Is(err, protocol.ErrMalformed) {
		t.Fatal("loading rebirth accepted", err)
	}
	c.warped = true
	if err := s.dispatch(context.Background(), c, []byte{69, 1, 2}); err != nil {
		t.Fatal(err)
	}
	c.ready = true
	for _, set := range []func(){
		func() { c.battle = &battleRun{} }, func() { c.event = &eventSession{} },
		func() { c.storm = true }, func() { c.beach = &beachRun{} }, func() { c.trade = &tradeSession{} },
	} {
		set()
		if err := s.dispatch(context.Background(), c, []byte{69, 1, 2}); err != nil || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("locked interaction rebirth", err)
		}
		for _, packet := range wires[0].packets(t) {
			if len(packet) > 0 && packet[0] == 69 {
				t.Fatal("locked interaction emitted rebirth result", packet)
			}
		}
		c.battle, c.event, c.trade = nil, nil, nil
		c.storm = false
		c.beach = nil
	}
}

func TestPetTrainingAllocationPacketsAndPersistence(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	ctx := context.Background()
	next := c.character.Clone()
	next.Pets[1].HP, next.Pets[1].SP = 17, 11
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before := c.character.Clone()
	statIDs := []byte{28, 29, 27, 33, 30}
	baseValues := []uint32{40, 60, 30, 50, 70}
	for selector := byte(1); selector <= 5; selector++ {
		if err := s.dispatch(ctx, c, []byte{68, 2, 2, selector}); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		if len(got) < 2 || !bytes.Equal(got[len(got)-1], []byte{68, 2, 2, selector, 1}) || !contains(got, protocol.Builder{8, 2, 4, 2, 0, statIDs[selector-1], 1}.U32(baseValues[selector-1]+1).U32(0)) || !contains(got, protocol.Builder{8, 2, 4, 2, 0, 38, 1}.U32(uint32(7-selector)).U32(0)) {
			t.Fatal("training wire reply", selector, got)
		}
	}
	pet := c.character.Pets[1]
	if pet.Base != (game.Attributes{Strength: 41, Constitution: 61, Intelligence: 31, Wisdom: 51, Agility: 71}) || pet.StatPoints != 2 || pet.HP != 17 || pet.SP != 11 || pet.MaxHP <= before.Pets[1].MaxHP || pet.MaxSP <= before.Pets[1].MaxSP || pet.Slot != 3 || pet.Level != 100 || pet.Exp != 123 || pet.Amity != 100 || pet.Reborn || !reflect.DeepEqual(pet.Skills, before.Pets[1].Skills) || !reflect.DeepEqual(c.character.Pets[0], before.Pets[0]) {
		t.Fatal("wrong pet training state", pet)
	}
	if wires[1].Len() != 0 {
		t.Fatal("training sent peer packets")
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || len(saved) != 1 || !reflect.DeepEqual(saved[0].Pets, c.character.Pets) {
		t.Fatal("training not durable", err)
	}
	roster := newPetRoster()
	for _, p := range saved[0].Pets {
		roster.register(p.ID)
	}
	if !bytes.Equal(s.petListPacket(&saved[0], roster), s.petListPacket(c.character, c.pets)) {
		t.Fatal("reconnected training differs")
	}
	// Existing AC8 allocation spends the same budget; AC68 cannot grant extra points.
	for range 2 {
		if err := s.dispatch(ctx, c, []byte{8, 2, 2, 28, 1}); err != nil {
			t.Fatal(err)
		}
		wires[0].Reset()
	}
	exhausted := c.character.Clone()
	if err := s.dispatch(ctx, c, []byte{68, 2, 2, 1}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) != 1 || !bytes.Equal(got[0], []byte{68, 2, 2, 1, 0}) || !reflect.DeepEqual(exhausted, *c.character) {
		t.Fatal("spent exhausted budget", got)
	}
	// The newly ported rebirth bonus feeds that same point budget.
	if err := s.dispatch(ctx, c, []byte{69, 1, 2}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.dispatch(ctx, c, []byte{68, 2, 2, 3}); err != nil {
		t.Fatal(err)
	}
	got = wires[0].packets(t)
	if !c.character.Pets[1].Reborn || c.character.Pets[1].StatPoints != 49 || c.character.Pets[1].Base.Intelligence != 32 || !contains(got, protocol.Builder{8, 2, 4, 2, 0, 38, 1}.U32(49).U32(0)) {
		t.Fatal("rebirth points unavailable for training", got)
	}
	saved, err = s.Store.Characters(ctx, c.account.ID)
	if err != nil || !reflect.DeepEqual(saved[0].Pets, c.character.Pets) {
		t.Fatal("trained rebirth not durable", err)
	}
}

func TestPetTrainingRejectionsAndSaveFailure(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	before := c.character.Clone()
	for _, p := range [][]byte{{68}, {68, 2}, {68, 2, 2}, {68, 2, 2, 1, 0}, {68, 3, 2, 1}} {
		if err := s.dispatch(context.Background(), c, p); err == nil || wires[0].Len() != 0 || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("malformed training mutated state", p, err)
		}
	}
	for _, p := range [][]byte{{68, 2, 0, 1}, {68, 2, 4, 1}, {68, 2, 255, 1}, {68, 2, 2, 0}, {68, 2, 2, 6}, {68, 2, 2, 255}} {
		if err := s.dispatch(context.Background(), c, p); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		if len(got) != 1 || !bytes.Equal(got[0], append(append([]byte(nil), p...), 0)) || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("invalid selector or slot", p, got)
		}
	}
	if err := s.dispatch(context.Background(), c, []byte{68, 1, 2, 0}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) != 2 || !bytes.Equal(got[0], []byte{68, 1, 2, 0, 0}) || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("unfunded potential training accepted", got)
	}
	c.character.Pets[1].Base.Strength = math.MaxUint16
	overflow := c.character.Clone()
	if err := s.dispatch(context.Background(), c, []byte{68, 2, 2, 1}); err != nil {
		t.Fatal(err)
	}
	got = wires[0].packets(t)
	if len(got) != 1 || !bytes.Equal(got[0], []byte{68, 2, 2, 1, 0}) || !reflect.DeepEqual(overflow, *c.character) {
		t.Fatal("training wrapped attribute", got)
	}
	*c.character = before
	s.Store.Close()
	if err := s.dispatch(context.Background(), c, []byte{68, 2, 2, 1}); err == nil || !reflect.DeepEqual(before, *c.character) || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("failed save published training", err)
	}
}

func TestPetTrainingRespectsWorldInteractionGates(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	before := c.character.Clone()
	c.ready = false
	if err := s.dispatch(context.Background(), c, []byte{68, 2, 2, 1}); !errors.Is(err, protocol.ErrMalformed) {
		t.Fatal("training while loading", err)
	}
	c.warped = true
	if err := s.dispatch(context.Background(), c, []byte{68, 2, 2, 1}); err != nil {
		t.Fatal(err)
	}
	c.ready = true
	for _, set := range []func(){
		func() { c.battle = &battleRun{} }, func() { c.event = &eventSession{} },
		func() { c.storm = true }, func() { c.beach = &beachRun{} }, func() { c.trade = &tradeSession{} },
	} {
		set()
		for _, command := range [][]byte{{68, 2, 2, 1}, {68, 1, 2, 0}} {
			if err := s.dispatch(context.Background(), c, command); err != nil || !reflect.DeepEqual(before, *c.character) {
				t.Fatal("locked training mutated state", err)
			}
			for _, packet := range wires[0].packets(t) {
				if len(packet) > 0 && packet[0] == 68 {
					t.Fatal("locked training emitted result", packet)
				}
			}
		}
		c.battle, c.event, c.trade = nil, nil, nil
		c.storm = false
		c.beach = nil
	}
}

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
	if recruit[0] != 15 || recruit[1] != 1 || recruit[6] != 0x4c || recruit[7] != 0x37 || recruit[11] != 4 || recruit[19] != 5 {
		t.Fatalf("AC15:1 %v", recruit)
	}
	pet := c.character.Pets[0]
	if pet.Level != 1 || pet.Amity != 60 || pet.Base.Agility != 5 || len(pet.Skills) != 1 || pet.HP != pet.MaxHP {
		t.Fatal("new pet", pet)
	}
	// A party-only branch continues into the next eligible one: the companion's question.
	if !contains(got, serverwire.EventFrame(6, 3, 5, 0, 0, 77, 1, 2)) {
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
	initialConstitution := p.Base.Constitution
	if p.ClientTotalExp() != 6 {
		t.Fatal(p.ClientTotalExp())
	}
	if gained := p.GainExp(14+35, tpl, true, func(total int) int {
		if total != 13 {
			t.Fatalf("growth total = %d, want 13", total)
		}
		return 1 // STR occupies roll 0; CON occupies rolls 1 through 9.
	}); gained != 2 || p.Level != 3 || p.Base.Constitution != initialConstitution+2 {
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
