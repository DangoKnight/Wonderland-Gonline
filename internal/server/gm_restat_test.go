package server

import (
	"bytes"
	"context"
	"reflect"
	"strconv"
	"testing"
	"wonderland-go/internal/game"
)

func restatFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	s, players, wires := chatFixture(t)
	c := players[0]
	next := c.character.Clone()
	next.Base = game.Attributes{Strength: 10, Constitution: 20, Intelligence: 30, Wisdom: 40, Agility: 50}
	next.StatPoints = 7
	next.Refill(s.Assets.Items)
	next.HP, next.SP = 1, 1
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	return s, players, wires
}

func TestGMRestatPersistencePacketsAndReplay(t *testing.T) {
	s, p, w := restatFixture(t)
	c := p[0]
	s.SetGMLevel(c.account.ID, 1)
	before := c.character.Clone()
	say(t, s, c, "/restat")
	expected := before.Clone()
	expected.Base = game.Attributes{Strength: 10, Constitution: 10, Intelligence: 10, Wisdom: 10, Agility: 10}
	expected.StatPoints = 107
	expected.Refill(s.Assets.Items)
	if !reflect.DeepEqual(*c.character, expected) {
		t.Fatal("reset changed unrelated state", *c.character)
	}
	packets := w[0].packets(t)
	if len(packets) != 17 || !bytes.Equal(packets[10], []byte{8, 1, 38, 1, 107, 0, 0, 0, 0, 0, 0, 0}) || packets[15][0] != 5 || packets[15][1] != 3 {
		t.Fatal("reset synchronization", packets)
	}
	// This avatar's permanent INT +2 remains after resetting its base to ten.
	if !bytes.Equal(packets[7], []byte{8, 1, 27, 1, 12, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatal("avatar INT synchronization", packets[7])
	}
	assertProgressSaved(t, s, c)
	if w[1].Len() != 0 || w[2].Len() != 0 {
		t.Fatal("reset broadcast to peers")
	}
	say(t, s, c, ":resetstats")
	if c.character.StatPoints != 107 {
		t.Fatal("repeat reset minted points")
	}
	w[0].Reset()
	// Refunded points remain spendable through ordinary stat allocation.
	if err := s.worldCommand(context.Background(), c, []byte{8, 1, 28}); err != nil {
		t.Fatal(err)
	}
	if c.character.Base.Strength != 11 || c.character.StatPoints != 106 {
		t.Fatal("refund not spendable")
	}
	assertProgressSaved(t, s, c)
}

func TestGMRestatTargetAuthorizationAndFailure(t *testing.T) {
	s, p, w := restatFixture(t)
	actor, target := p[2], p[0] // Cross-map target.
	before := target.character.Clone()
	say(t, s, actor, "/restat "+target.character.Name)
	if !reflect.DeepEqual(*target.character, before) {
		t.Fatal("non-GM reset target")
	}
	s.SetGMLevel(actor.account.ID, 1)
	for _, text := range []string{"/restat missing", "/restat one two"} {
		say(t, s, actor, text)
	}
	if !reflect.DeepEqual(*target.character, before) || w[0].Len() != 0 {
		t.Fatal("invalid target reset self or target")
	}
	w[2].Reset()
	say(t, s, actor, "/resetstats "+strconv.FormatUint(uint64(target.character.ID), 10))
	if target.character.StatPoints != 107 || len(w[0].packets(t)) != 16 || len(w[2].packets(t)) != 1 || w[1].Len() != 0 {
		t.Fatal("target reset synchronization")
	}
	assertProgressSaved(t, s, target)
	s.SetGMLevel(actor.account.ID, 0)
	target.character.Base.Strength = 20
	say(t, s, actor, "/restat "+target.character.Name)
	if target.character.Base.Strength != 20 {
		t.Fatal("revoked GM reset")
	}
}

func TestGMRestatInteractionGates(t *testing.T) {
	for _, owner := range []int{0, 1} {
		for _, gate := range []string{"loading", "battle", "trade", "event", "storm", "beach", "minigame"} {
			t.Run(strconv.Itoa(owner)+gate, func(t *testing.T) {
				s, p, w := restatFixture(t)
				actor, target := p[1], p[0]
				s.SetGMLevel(actor.account.ID, 1)
				c := target
				if owner == 1 {
					c = actor
				}
				switch gate {
				case "loading":
					c.ready = false
				case "battle":
					c.battle = &battleRun{}
				case "trade":
					c.trade = &tradeSession{}
				case "event":
					c.event = &eventSession{}
				case "storm":
					c.storm = true
				case "beach":
					c.beach = &beachRun{}
				case "minigame":
					c.event = &eventSession{onMinigame: func(byte) error { return nil }}
				}
				before := target.character.Clone()
				err := s.worldCommand(context.Background(), actor, append([]byte{2, 2}, []byte("/restat "+target.character.Name)...))
				if owner == 1 && gate == "loading" {
					if err == nil {
						t.Fatal("loading actor accepted")
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(*target.character, before) || w[0].Len() != 0 {
					t.Fatal("busy reset changed target")
				}
			})
		}
	}
}

func TestGMRestatSaveAndSnapshotFailures(t *testing.T) {
	for _, failure := range []string{"save", "skill snapshot"} {
		t.Run(failure, func(t *testing.T) {
			s, p, w := restatFixture(t)
			c := p[0]
			s.SetGMLevel(c.account.ID, 1)
			ctx := context.Background()
			if failure == "save" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			} else {
				s.Assets.Skills = nil
			}
			before := c.character.Clone()
			err := s.worldCommand(ctx, c, append([]byte{2, 2}, []byte("/restat")...))
			if err == nil || !reflect.DeepEqual(*c.character, before) || w[0].Len() != 0 {
				t.Fatal("failed reset mutated or published", err)
			}
			assertProgressSaved(t, s, c)
		})
	}
}

func TestGMRestatFailedTargetDelivery(t *testing.T) {
	s, p, w := restatFixture(t)
	actor, target := p[1], p[0]
	s.SetGMLevel(actor.account.ID, 1)
	broken := &failedWorldConn{}
	target.conn = broken
	say(t, s, actor, "/restat "+target.character.Name)
	if !broken.closed || target.character.StatPoints != 107 || len(w[1].packets(t)) != 1 {
		t.Fatal("failed target delivery not isolated")
	}
	assertProgressSaved(t, s, target)
}
