package store

import (
	"context"
	"testing"
	"wonderland-go/internal/game"
)

func TestCharacterMetadataV13UpgradeReopenAndTentFloors(t *testing.T) {
	db, ref, _, _, path := manufacturingStoreFixture(t)
	ctx := context.Background()
	before, err := db.Characters(ctx, ref.Account)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{"ALTER TABLE character_state DROP COLUMN nickname", "ALTER TABLE character_state DROP COLUMN job", "ALTER TABLE character_state DROP COLUMN potential", "ALTER TABLE character_pets DROP COLUMN potential", "ALTER TABLE tents DROP COLUMN floor2", "ALTER TABLE tents DROP COLUMN wallpaper2", "DROP TABLE manufacture_jobs", "PRAGMA user_version=13"} {
		if _, err = db.db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	after, err := db.Characters(ctx, ref.Account)
	if err != nil || !CharacterSnapshotsEqual(before[0], after[0]) {
		t.Fatal("upgrade changed existing character", err)
	}
	if err = db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error {
		c.Nickname = "Builder Nick"
		c.Job = game.JobKnight
		c.Reborn = true
		c.Potential = 300
		c.RebornJob = 5
		c.Pets = []game.Pet{{ID: 1, Slot: 1, Potential: 77}}
		c.ReservePets = []game.Pet{{ID: 2, Slot: 2, Potential: 88}}
		c.HotelPets = []game.Pet{{ID: 3, Slot: 3, Potential: 99}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.orm.Model(&Tent{}).Where("owner_id = ?", ref.ID).Updates(map[string]any{"floor2": 123, "wallpaper2": 456}).Error; err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	chars, err := db.Characters(ctx, ref.Account)
	if err != nil {
		t.Fatal(err)
	}
	c := chars[0]
	if c.Nickname != "Builder Nick" || c.Job != 3 || c.Potential != 300 || c.RebornJob != 5 || c.Pets[0].Potential != 77 || c.ReservePets[0].Potential != 88 || c.HotelPets[0].Potential != 99 {
		t.Fatal(c)
	}
	tent, err := db.Tent(ctx, ref.ID)
	if err != nil || tent.Floor2 != 123 || tent.Wallpaper2 != 456 {
		t.Fatal(tent, err)
	}
}
