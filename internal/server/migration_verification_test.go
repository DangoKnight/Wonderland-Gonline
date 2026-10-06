package server

import (
	"bytes"
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

func TestMigrationBankDefaultBalanceIsPrivateAndReadOnly(t *testing.T) {
	s, c, wires := bankFixture(t)
	before := c.character.Clone()
	for sub := 0; sub <= math.MaxUint8; sub++ {
		if sub >= 8 && sub <= 12 {
			continue
		}
		// Unknown operations retain the reference default, including ignored data.
		if err := s.dispatch(context.Background(), c, []byte{45, byte(sub), 255, 255, 255, 255}); err != nil {
			t.Fatal(sub, err)
		}
		got := wires[0].packets(t)
		if len(got) != 1 || !bytes.Equal(got[0], []byte{45, 8, 100, 0, 0, 0, 244, 1, 0, 0}) {
			t.Fatal(sub, got)
		}
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || !reflect.DeepEqual(before, *c.character) || stored[0].BankGold != 100 || stored[0].Gold != 500 {
		t.Fatal("default bank query mutated balances", err)
	}
	for _, wire := range wires[1:] {
		if wire.Len() != 0 {
			t.Fatal("bank query leaked")
		}
	}
}

func TestMigrationGuildJournalWithoutGuildAndRepeatedRequest(t *testing.T) {
	s, c, wires := bankFixture(t)
	s.Assets.Marks = map[uint16]uint16{900: 0, 901: 17}
	next := c.character.Clone()
	next.Quests = map[uint32]game.Quest{900: {ID: 900, State: game.InProgress, Step: 2}, 901: {ID: 901, State: game.InProgress, Step: 1}}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	c.view.Marks = map[uint16]bool{}
	before := c.character.Clone()
	for attempt := 0; attempt < 2; attempt++ {
		if err := s.dispatch(context.Background(), c, []byte{39, 1}); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		wantFlags := []byte{24, 7}
		for index := 1; index <= 250; index++ {
			bits := byte(0)
			if index == 3 {
				bits = 1
			}
			wantFlags = append(wantFlags, byte(index), bits)
		}
		if !contains(got, []byte{24, 6, 1, 132, 3, 2}) || !contains(got, wantFlags) || len(got) != 2+attempt {
			t.Fatal("journal wire layout", got)
		}
		if attempt == 1 && !bytes.Equal(got[0], []byte{24, 4, 132, 3}) {
			t.Fatal("old journal entry not cleared", got)
		}
	}
	if err := s.dispatch(context.Background(), c, []byte{39, 1, 0}); !errors.Is(err, protocol.ErrMalformed) || wires[0].Len() != 0 {
		t.Fatal("malformed journal request", err)
	}
	if guild, err := s.Store.GuildForCharacter(context.Background(), c.character.ID); err != nil || guild != nil || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("journal changed gameplay state", err)
	}
	for _, wire := range wires[1:] {
		if wire.Len() != 0 {
			t.Fatal("journal leaked")
		}
	}
}

func TestMigrationAllocationBatchPersistenceAndFailure(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	ctx := context.Background()
	next := c.character.Clone()
	next.HP, next.SP = 17, 11
	next.StatPoints = 4
	next.Pets[1].StatPoints = 4
	next.Pets[1].Base.Strength = math.MaxUint16
	next.Pets[1].HP, next.Pets[1].SP = 17, 11
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	for _, wire := range wires {
		wire.Reset()
	}
	before := c.character.Clone()
	// Client pet slot 2 resolves persistent slot 3. Overflow leaves points for INT.
	if err := s.dispatch(ctx, c, []byte{8, 2, 2, 2, 28, 1, 0, 0, 0, 27, 4, 0}); err != nil {
		t.Fatal(err)
	}
	pet := c.character.Pets[1]
	if pet.Base.Strength != math.MaxUint16 || pet.Base.Intelligence != before.Pets[1].Base.Intelligence+4 || pet.StatPoints != 0 || pet.HP != 17 || pet.SP != 11 || !reflect.DeepEqual(c.character.Pets[0], before.Pets[0]) {
		t.Fatal("pet batch outcome", pet)
	}
	if !contains(wires[0].packets(t), []byte{8, 2, 4, 2, 0, 38, 1, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatal("pet points snapshot missing")
	}
	// Source skips unaffordable entries and continues applying the same batch.
	if err := s.dispatch(ctx, c, []byte{8, 1, 0, 3, 28, 3, 0, 0, 0, 29, 2, 0, 0, 0, 30, 1}); err != nil {
		t.Fatal(err)
	}
	if c.character.StatPoints != 0 || c.character.Base.Strength != before.Base.Strength+3 || c.character.Base.Constitution != before.Base.Constitution || c.character.Base.Agility != before.Base.Agility+1 || c.character.HP != 17 || c.character.SP != 11 {
		t.Fatal("character batch outcome", c.character)
	}
	wires[0].Reset()
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || !reflect.DeepEqual(stored[0].Pets, c.character.Pets) || stored[0].Base != c.character.Base {
		t.Fatal("allocation not durable", err)
	}
	before = c.character.Clone()
	// Exhausted budgets cannot be spent again by replaying either request.
	for _, packet := range [][]byte{{8, 28, 1}, {8, 2, 2, 27, 1}} {
		if err := s.dispatch(ctx, c, packet); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(before, *c.character) || wires[0].Len() != 0 {
		t.Fatal("allocation replay consumed exhausted budget")
	}
	next = c.character.Clone()
	next.StatPoints = 1
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before = c.character.Clone()
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.dispatch(canceled, c, []byte{8, 28, 1}); err == nil || !reflect.DeepEqual(before, *c.character) || wires[0].Len() != 0 {
		t.Fatal("failed allocation published", err)
	}
}

func TestMigrationPetDesertionThresholdAndUnrelatedMount(t *testing.T) {
	for _, tc := range []struct {
		name    string
		amity   byte
		deaths  int
		deserts bool
		outcome battle.Outcome
	}{
		{"threshold retained", 21, 1, false, battle.Defeat},
		{"threshold deserts", 20, 1, true, battle.Defeat},
		{"repeated knockouts", 21, 2, true, battle.Fled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, wire := battleFixture(t, 1000)
			pet := game.Pet{ID: 10501, Slot: 1, Amity: tc.amity, HP: 10, MaxHP: 10, Battle: true}
			mount := game.Pet{ID: 10502, Slot: 2, Amity: 80, HP: 10, MaxHP: 10}
			next := c.character.Clone()
			next.Pets = []game.Pet{pet, mount}
			next.ActivePet = pet.ID
			next.ActiveMount = mount.ID
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			c.pets.register(pet.ID)
			c.pets.register(mount.ID)
			run := c.battle
			run.members[0].pet = battle.PetFighter(pet, c.character.ID, nil, 0)
			run.members[0].pet.Deaths = tc.deaths
			run.members[0].pet.HP = 0
			wire.Reset()
			s.endBattle(run, tc.outcome)
			settle(t, s, c)
			stored, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || stored[0].ActiveMount != mount.ID || !reflect.DeepEqual(stored[0].Pets, c.character.Pets) {
				t.Fatal("pet outcome not durable or removed unrelated mount", err)
			}
			got := wire.packets(t)
			removed := protocol.Builder{15, 2}.U32(c.character.ID).U8(1)
			if tc.deserts {
				if len(stored[0].Pets) != 1 || stored[0].ActivePet != 0 || c.pets.slot(pet.ID) != 0 || !contains(got, removed) || !contains(got, []byte{19, 2}) {
					t.Fatal("desertion not published after commit", got)
				}
			} else if len(stored[0].Pets) != 2 || stored[0].Pets[0].Amity != 20 || contains(got, removed) {
				t.Fatal("threshold pet deserted", got)
			}
			s.endBattle(run, tc.outcome)
			if wire.Len() != 0 {
				t.Fatal("duplicate settlement published pet outcome")
			}
		})
	}
}

func TestMigrationManufacturingFirstRecipeWins(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	s.Assets.Economy.Manufacturing = []assets.ManufacturingRecipe{
		{Workbench: "Forge", Inputs: [2]assets.ManufacturingInput{{ItemID: 100, Count: 1}, {ItemID: 200, Count: 1}}, Output: assets.ManufacturingInput{ItemID: 300, Count: 1}},
		{Workbench: "Forge", Inputs: [2]assets.ManufacturingInput{{ItemID: 100, Count: 1}, {ItemID: 200, Count: 1}}, Output: assets.ManufacturingInput{ItemID: 200, Count: 1}},
	}
	if err := s.dispatch(context.Background(), c, []byte{59, 1, 100, 0, 1, 200, 0, 1}); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[0].ID != 300 || !contains(wires[0].packets(t), []byte{59, 1, 1}) {
		t.Fatal("later recipe shadowed source first match")
	}
}

// AC01 and AC22 echo generic subcommands, but their payloads differ. There
// is no generic response for an entirely unregistered command.
func TestMigrationGenericAcknowledgements(t *testing.T) {
	s, c, wires := bankFixture(t)
	s.Started = time.Now().Add(-time.Second)
	before := c.character.Clone()
	for sub := 0; sub <= math.MaxUint8; sub++ {
		if err := s.dispatch(context.Background(), c, []byte{1, byte(sub)}); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		if len(got) != 1 {
			t.Fatal(got)
		}
		if sub == 1 {
			if len(got[0]) != 7 || !bytes.Equal(got[0][:3], []byte{1, 1, 1}) || protocol.NewReader(got[0][3:]).U32() < 1000 {
				t.Fatal("uptime handshake", got)
			}
		} else if !bytes.Equal(got[0], []byte{1, byte(sub), 1}) {
			t.Fatal("generic handshake", got)
		}
		if err := s.dispatch(context.Background(), c, []byte{22, byte(sub), 52, 18}); err != nil {
			t.Fatal(err)
		}
		got = wires[0].packets(t)
		if len(got) != 1 || !bytes.Equal(got[0], []byte{22, byte(sub), 52, 18, 1}) {
			t.Fatal("generic entity acknowledgement", got)
		}
	}
	for _, packet := range [][]byte{{3, 1}, {255, 1}} {
		if err := s.dispatch(context.Background(), c, packet); !errors.Is(err, ErrUnsupported) || wires[0].Len() != 0 {
			t.Fatal("unsupported command fabricated an acknowledgement", err)
		}
	}
	if !reflect.DeepEqual(before, *c.character) {
		t.Fatal("acknowledgement changed character")
	}
	for _, wire := range wires[1:] {
		if wire.Len() != 0 {
			t.Fatal("private acknowledgement leaked")
		}
	}
}

func TestMigrationVoucherRollbackAndSparseRoster(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	ctx := context.Background()
	s.Assets.PetVouchers = map[uint16]uint16{30068: 14719}
	s.Assets.Items[30068] = game.ItemDefinition{ID: 30068, Type: 23}
	s.Assets.NPCs[14719] = assets.NPC{ID: 14719, Name: "Voucher pet", Stats: [5]uint16{7, 8, 9, 10, 11}}
	// Rebuild the immutable fixture world to include the authored NPC template.
	s.World = world.New(s.Assets)
	next := c.character.Clone()
	next.Bag[8] = game.Item{ID: 30068, Count: 2}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	for _, wire := range wires {
		wire.Reset()
	}
	before := c.character.Clone()
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.dispatch(canceled, c, []byte{23, 96, 9}); err == nil || !reflect.DeepEqual(before, *c.character) || c.pets.slot(14719) != 0 || wires[0].Len() != 0 {
		t.Fatal("failed voucher consumed item or reserved roster slot", err)
	}
	if err := s.dispatch(ctx, c, []byte{23, 96, 9}); err != nil {
		t.Fatal(err)
	}
	index, owned := c.character.Pet(14719)
	if !owned || c.character.Pets[index].Slot != 1 || c.pets.slot(14719) != 3 || c.character.Bag[8].Count != 1 {
		t.Fatal("voucher did not resolve sparse roster", c.character.Pets)
	}
	pet := c.character.Pets[index]
	if pet.Level != 1 || pet.Amity != 60 || pet.Base != (game.Attributes{Strength: 7, Constitution: 8, Intelligence: 9, Wisdom: 10, Agility: 11}) || pet.HP != pet.MaxHP || pet.SP != pet.MaxSP {
		t.Fatal("voucher initialization differs", pet)
	}
	if !contains(wires[0].packets(t), []byte{8, 2, 4, 3, 0, 38, 1, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatal("voucher progression addressed persistent slot instead of client slot")
	}
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || !reflect.DeepEqual(stored[0].Pets, c.character.Pets) || stored[0].Bag != c.character.Bag {
		t.Fatal("voucher not durable", err)
	}
	before = c.character.Clone()
	if err := s.dispatch(ctx, c, []byte{23, 96, 9}); err != nil || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("duplicate voucher redeemed", err)
	}
	for _, wire := range wires[1:] {
		if wire.Len() != 0 {
			t.Fatal("voucher roster leaked to other players")
		}
	}
}
