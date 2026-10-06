package store

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"path/filepath"
	"testing"
	"time"
	"wonderland-gonline/internal/game"
)

func TestStructuredCharacterMigrationPreservesState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Register(context.Background(), "legacy", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	c := game.Character{ID: a.CharacterID(1), Slot: 1, Name: "Legacy", Level: 10, HP: 200, MaxHP: 400, SP: 100, MaxSP: 300, Gold: 900, Reborn: true, Base: game.Attributes{Strength: 23, Constitution: 17, Intelligence: 13, Wisdom: 31, Agility: 9}, Settings: &game.ClientSettings{}, RecordPoint: &game.Location{Map: 12000, X: 33, Y: 44}, LuckyDraw: game.LuckyDrawState{Day: "2026-10-03", Used: 2}, ActivePet: 123, ActiveMount: 124, ActiveVehicle: 125, VehicleSlot: 8, DiscoveredMonsters: []uint16{9, 9, 2}}
	now := time.Date(2026, 10, 3, 1, 2, 3, 123456789, time.UTC)
	c.Quests = map[uint32]game.Quest{17: {ID: 18, State: game.Completed, Step: 7, Kills: 11, StartedAt: now, CompletedAt: &now}}
	c.EventTimers = map[uint16]time.Time{13: now}
	c.ChestRespawns = map[uint32]time.Time{987654: now}
	c.MutedUntil = now
	c.Bag[49] = game.Item{ID: 100, Count: 7, Damage: 3}
	c.Bag[49].Metadata[0] = 11
	c.Storage[20] = c.Bag[49]
	c.Equipment[5] = c.Bag[49]
	c.Skills = []game.LearnedSkill{{ID: 7, Grade: 2, EXP: 30}, {ID: 7, Grade: 3, EXP: 40}}
	p := game.Pet{ID: 123, Name: "Pet", Level: 5, HP: -2, MaxHP: 100, SP: 25, MaxSP: 40, Base: c.Base, StatPoints: 9, Amity: 81, Reborn: true, Job: 2, Skills: []game.PetSkill{{ID: 8, Grade: 4, Exp: 19}}}
	p.Equipment[3] = c.Bag[49]
	c.Pets = []game.Pet{p, p}
	c.ReservePets = []game.Pet{p}
	c.HotelPets = []game.Pet{p}
	raw, _ := json.Marshal(c)
	if err = s.orm.Create(&characterRow{ID: c.ID, AccountID: a.ID, Slot: c.Slot, Name: c.Name, State: raw}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("PRAGMA user_version=7"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Characters(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || CharacterVersion(got[0]) != CharacterVersion(c) {
		t.Fatalf("state changed: %#v", got)
	}
	// SQL is authoritative; reopening must not replay the retained JSON snapshot.
	if err = s.UpdateCharacter(context.Background(), a.ID, c.ID, func(c *game.Character) error {
		c.Gold = 0
		c.Pets = nil
		c.Skills = nil
		c.EventTimers = nil
		c.Bag = game.Inventory{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var retained characterRow
	if err = s.orm.First(&retained, c.ID).Error; err != nil {
		t.Fatal(err)
	}
	if string(retained.State) != string(raw) {
		t.Fatal("migration snapshot was overwritten")
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err = s.Characters(context.Background(), a.ID)
	if err != nil || got[0].Gold != 0 || len(got[0].Pets) != 0 || len(got[0].Skills) != 0 || len(got[0].EventTimers) != 0 || got[0].Bag[49].ID != 0 {
		t.Fatalf("stale state restored: %#v %v", got, err)
	}
	if err = s.transaction(context.Background(), func(tx *gorm.DB) error { return tx.Delete(&characterRow{}, c.ID).Error }); err != nil {
		t.Fatal(err)
	}
	for _, model := range characterTables() {
		var n int64
		if err = s.orm.Model(model).Count(&n).Error; err != nil || n != 0 {
			t.Fatalf("orphan in %T: %d %v", model, n, err)
		}
	}
}

func TestStructuredCharacterMigrationRollsBackInvalidLegacyState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Register(context.Background(), "legacy", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.orm.Create(&characterRow{ID: a.CharacterID(1), AccountID: a.ID, Slot: 1, Name: "Bad", State: []byte("invalid")}).Error; err != nil {
		t.Fatal(err)
	}
	// Simulate a real pre-v8 schema, entirely within a disposable database.
	for i := len(characterTables()) - 1; i >= 0; i-- {
		if err = s.orm.Migrator().DropTable(characterTables()[i]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.db.Exec("PRAGMA user_version=7"); err != nil {
		t.Fatal(err)
	}
	if err = s.migrate(); err == nil {
		t.Fatal("accepted invalid legacy JSON")
	}
	var version int
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 7 {
		t.Fatalf("version %d %v", version, err)
	}
	if s.orm.Migrator().HasTable(&characterStateRow{}) {
		t.Fatal("partial migration schema committed")
	}
	var row characterRow
	if err = s.orm.First(&row, a.CharacterID(1)).Error; err != nil || string(row.State) != "invalid" {
		t.Fatal("original row changed", err)
	}
	s.Close()
}
