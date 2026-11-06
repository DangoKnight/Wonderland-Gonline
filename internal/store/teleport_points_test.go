package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"wonderland-gonline/internal/game"
)

func TestTeleportPointsUpgradeOwnershipAndRestart(t *testing.T) {
	s, a, ref, path := mallStoreFixture(t)
	if err := s.UpdateCharacter(context.Background(), a.ID, ref.ID, func(c *game.Character) error { c.Map = 12345; c.Bag[0] = game.Item{ID: 34163, Count: 1}; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("DROP TABLE character_teleport_points"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("PRAGMA user_version=17"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	loc := game.Location{Map: 12345, X: 200, Y: 300}
	scrolls := map[uint16]byte{34163: 1}
	if err := s.RecordTeleportPoint(context.Background(), ref, 1, 1, loc, scrolls); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordTeleportPoint(context.Background(), ref, 2, 1, loc, scrolls); !errors.Is(err, game.ErrInvalidItem) {
		t.Fatal("scroll capacity", err)
	}
	if err := s.RecordTeleportPoint(context.Background(), CharacterRef{Account: a.ID + 1, ID: ref.ID}, 1, 1, loc, scrolls); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("ownership", err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	points, err := s.TeleportPoints(context.Background(), ref)
	if err != nil || len(points) != 1 || points[1] != loc {
		t.Fatal("restart", points, err)
	}
	chars, err := s.Characters(context.Background(), a.ID)
	if err != nil || chars[0].Bag[0].Count != 1 {
		t.Fatal("record consumed scroll", err)
	}
}
