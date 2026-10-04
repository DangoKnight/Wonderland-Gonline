package server

import (
	"context"
	"encoding/binary"
	"testing"
	"wonderland-go/internal/game"
)

func TestCompiledElementalGrowthClinicAndPersistence(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	c.character.Base = game.Attributes{Strength: 5, Constitution: 5, Intelligence: 5, Wisdom: 5, Agility: 5}
	c.character.Element = game.Earth
	ctx := context.Background()
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	c.character.HP = 10
	if err := s.openService(c, 6); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, []byte{31, 1}); err != nil {
		t.Fatal(err)
	}
	if c.character.HP != 201 || c.character.SP != 121 || c.character.MaxHP != 201 || c.character.MaxSP != 121 {
		t.Fatal("clinic ignored compiled growth", c.character)
	}
	found := false
	for _, p := range wires[0].packets(t) {
		if len(p) == 12 && p[0] == 8 && p[2] == 41 {
			found = true
			if binary.LittleEndian.Uint32(p[4:8]) != 10 {
				t.Fatal("native ATK contribution", p)
			}
		}
	}
	if !found {
		t.Fatal("missing compiled stat update")
	}
	assertProgressSaved(t, s, c)
}

func TestCompiledElementalGrowthLoginPersistsWithoutHealing(t *testing.T) {
	s, players, _ := worldFixture(t)
	if err := s.Store.UpdateCharacter(context.Background(), players[0].account.ID, players[0].character.ID, func(stored *game.Character) error {
		stored.Element = game.Water
		stored.Base.Wisdom = 5
		stored.HP, stored.SP = 100, 50
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	c := &Session{account: players[0].account, conn: &captureConn{}, info: SessionInfo{ID: 99}}
	if err := s.dispatch(context.Background(), c, []byte{63, 2, 1}); err != nil {
		t.Fatal(err)
	}
	if c.character == nil || c.character.MaxHP != 181 || c.character.MaxSP != 121 || c.character.HP != 100 || c.character.SP != 50 {
		t.Fatalf("login did not apply growth without healing: %+v", c.character)
	}
	assertProgressSaved(t, s, c)
}
