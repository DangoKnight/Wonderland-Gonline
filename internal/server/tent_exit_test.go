package server

import (
	"context"
	"testing"

	"wonderland-gonline/internal/game"
)

func TestTentExitDestinationPrefersVisitorAndValidatesFallback(t *testing.T) {
	s, players, _ := tentFixture(t)
	host, guest := players[0], players[1]
	host.openTent = &openTent{Map: 12000, X: 10, Y: 20}
	guest.tentOwner = host.character.ID
	for _, tc := range []struct {
		name  string
		point *game.Location
		want  game.Location
	}{
		{"visitor", &game.Location{Map: 12000, X: 30, Y: 40}, game.Location{Map: 12000, X: 30, Y: 40}},
		{"missing", nil, game.Location{Map: 12000, X: 10, Y: 20}},
		{"interior", &game.Location{Map: 63507, X: 460, Y: 700}, game.Location{Map: 12000, X: 10, Y: 20}},
		{"unknown_map", &game.Location{Map: 65535}, game.Location{Map: 12000, X: 10, Y: 20}},
		// Coordinates alone do not identify an interior: a legitimate outdoor
		// return at the same numbers must remain usable.
		{"overworld_at_spawn_coordinates", &game.Location{Map: 12000, X: 460, Y: 700}, game.Location{Map: 12000, X: 460, Y: 700}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			guest.character.TentReturn = tc.point
			if dst, ok := s.tentExitDestination(guest); !ok || dst != tc.want {
				t.Fatal(dst, ok)
			}
		})
	}
	guest.character.TentReturn = nil
	host.openTent.Map = 63507
	if _, ok := s.tentExitDestination(guest); ok {
		t.Fatal("fallback to interior accepted")
	}
	host.openTent = nil
	if err := s.exitTent(context.Background(), guest); err == nil {
		t.Fatal("missing return point silently accepted")
	}
}

func TestTentExitFallbackCommitsBeforeLeavingPrivateScene(t *testing.T) {
	s, players, _ := tentFixture(t)
	host, guest := players[0], players[1]
	tradeDo(t, s, host, []byte{23, 96, 1})
	want := game.Location{Map: host.openTent.Map, X: host.openTent.X, Y: host.openTent.Y}
	enterTestTent(t, s, host, host.character.ID)
	enterTestTent(t, s, guest, host.character.ID)
	guest.character.TentReturn = nil
	tradeDo(t, s, guest, []byte{65, 3})
	if guest.tentOwner != 0 || guest.character.Map != want.Map || guest.character.X != want.X || guest.character.Y != want.Y || guest.character.TentReturn != nil {
		t.Fatal("fallback did not leave the private scene", guest.character)
	}
	rows, err := s.Store.Characters(context.Background(), guest.account.ID)
	if err != nil || rows[0].Map != want.Map || rows[0].X != want.X || rows[0].Y != want.Y || rows[0].TentReturn != nil {
		t.Fatal("fallback was not durable", rows, err)
	}
}
