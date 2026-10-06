package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func assertProgressSaved(t *testing.T, s *Server, c *Session) {
	t.Helper()
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := json.Marshal(c.character)
	stored, _ := json.Marshal(chars[0])
	if !bytes.Equal(current, stored) {
		t.Fatal("session and durable progression differ")
	}
}

func TestGMLevelPointsAttributesAndEXP(t *testing.T) {
	s, players, wires := chatFixture(t)
	c := players[0]
	s.SetGMLevel(c.account.ID, 1)
	say(t, s, c, ":level 3")
	if c.character.Level != 3 || c.character.EXP != 49 || c.character.StatPoints != 6 || c.character.HP != c.character.MaxHP || c.character.SP != c.character.MaxSP {
		t.Fatal("level progression incorrect")
	}
	packets := wires[0].packets(t)
	if len(packets) != 16 || !bytes.Equal(packets[0], []byte{8, 1, 36, 1, 49, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatal("level EXP synchronization", packets)
	}
	assertProgressSaved(t, s, c)
	for _, alias := range []string{"points", "sp", "statpoint", "statpoints"} {
		say(t, s, c, "/"+alias+" 2")
		got := wires[0].packets(t)
		if len(got) != 1 || got[0][2] != 38 {
			t.Fatal("stat-point response", got)
		}
	}
	if c.character.StatPoints != 14 {
		t.Fatal("wrong points total")
	}
	say(t, s, c, ":points 65535")
	wires[0].Reset()
	if c.character.StatPoints != math.MaxUint16 {
		t.Fatal("points did not saturate")
	}
	say(t, s, c, ":stat 10 20 30 40 50")
	if c.character.Base != (game.Attributes{Strength: 10, Constitution: 20, Intelligence: 30, Wisdom: 40, Agility: 50}) {
		t.Fatal("attributes not assigned in native order")
	}
	got := wires[0].packets(t)
	// This avatar adds two INT points; command values are stored base values.
	if len(got) != 15 || !bytes.Equal(got[7], []byte{8, 1, 27, 1, 32, 0, 0, 0, 0, 0, 0, 0}) || c.character.HP != c.character.MaxHP {
		t.Fatal("avatar bonuses or refill incorrect", got)
	}
	assertProgressSaved(t, s, c)
	points := c.character.StatPoints
	say(t, s, c, "/exp 14")
	got = wires[0].packets(t)
	if c.character.Level != 2 || c.character.EXP != 14 || c.character.StatPoints != points || len(got) != 16 {
		t.Fatal("direct EXP assignment granted points")
	}
	assertProgressSaved(t, s, c)
	say(t, s, c, "/exp -20")
	wires[0].Reset()
	if c.character.Level != 1 || c.character.EXP != 0 || c.character.HP > c.character.MaxHP {
		t.Fatal("negative EXP or vitals invalid")
	}
	assertProgressSaved(t, s, c)
	say(t, s, c, "/lvl 255")
	wires[0].Reset()
	if c.character.Level != 199 {
		t.Fatal("command level cap not preserved")
	}
	assertProgressSaved(t, s, c)
}

func TestGMSkillOverrideAndCatalogValidation(t *testing.T) {
	s, players, wires := chatFixture(t)
	c := players[0]
	s.SetGMLevel(c.account.ID, 1)
	s.Assets.Skills[15101] = assets.Skill{ID: 15101}
	say(t, s, c, ":skill 15101 4")
	got := wires[0].packets(t)
	if len(got) != 4 || !bytes.Equal(got[1], []byte{5, 16, 0, 253, 58, 4}) {
		t.Fatal("skill receipt", got)
	}
	assertProgressSaved(t, s, c)
	// Repeated explicit updates reset training; quest learning remains non-degrading.
	for i := range c.character.Skills {
		if c.character.Skills[i].ID == 15101 {
			c.character.Skills[i].EXP = 399
		}
	}
	say(t, s, c, "/skill 15101 1")
	got = wires[0].packets(t)
	if len(got) != 3 {
		t.Fatal("grade decrease sent an increment", got)
	}
	count := 0
	for _, skill := range c.character.Skills {
		if skill.ID == 15101 {
			count++
			if skill.Grade != 1 || skill.EXP != 0 {
				t.Fatal("proficiency not reset")
			}
		}
	}
	if count != 1 {
		t.Fatal("duplicate skill")
	}
	assertProgressSaved(t, s, c)
}

func TestGMProgressRejectsInvalidInputWithoutMutation(t *testing.T) {
	s, players, wires := chatFixture(t)
	c := players[0]
	s.SetGMLevel(c.account.ID, 1)
	before, _ := json.Marshal(c.character)
	for _, text := range []string{":level", ":level -1", ":level 256", ":level nope", ":points -1", ":points 65536", ":stat 1 2 3 4", ":stats 1 2 3 4 -1", ":stats 1 2 3 4 65536", ":exp nope", ":exp 4294967296", ":skill 15101", ":skill 11166 0", ":skill 11166 11", ":skill 11166 bad", ":skill 65536 1"} {
		say(t, s, c, text)
		after, _ := json.Marshal(c.character)
		if !bytes.Equal(before, after) || wires[0].Len() != 0 {
			t.Fatal("invalid command changed state", text)
		}
	}
	assertProgressSaved(t, s, c)
}

func TestGMProgressPermissionAndFailedSaves(t *testing.T) {
	for _, text := range []string{":level 4", ":points 20", ":stats 5 6 7 8 9", ":exp 100", ":skill 15101 3"} {
		t.Run(strings.Fields(text)[0], func(t *testing.T) {
			s, players, wires := chatFixture(t)
			c := players[0]
			s.Assets.Skills[15101] = assets.Skill{ID: 15101}
			before, _ := json.Marshal(c.character)
			say(t, s, c, text)
			after, _ := json.Marshal(c.character)
			if !bytes.Equal(before, after) || wires[0].Len() != 0 {
				t.Fatal("non-GM progression accepted")
			}
			s.SetGMLevel(c.account.ID, 1)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := s.dispatch(ctx, c, travelPacket(text)); !errors.Is(err, context.Canceled) {
				t.Fatal("save failure not surfaced", err)
			}
			after, _ = json.Marshal(c.character)
			if !bytes.Equal(before, after) || wires[0].Len() != 0 {
				t.Fatal("failed save published or changed state")
			}
			assertProgressSaved(t, s, c)
		})
	}
}

func TestGMProgressOwnershipGates(t *testing.T) {
	for _, gate := range []string{"battle", "trade", "event", "storm", "beach", "loading"} {
		t.Run(gate, func(t *testing.T) {
			s, players, wires := chatFixture(t)
			c := players[0]
			s.SetGMLevel(c.account.ID, 1)
			s.Assets.Skills[15101] = assets.Skill{ID: 15101}
			switch gate {
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
			case "loading":
				c.ready = false
				c.warped = true
			}
			before, _ := json.Marshal(c.character)
			for _, text := range []string{":level 5", ":points 10", ":stats 5 6 7 8 9", ":exp 100", ":skill 15101 3"} {
				say(t, s, c, text)
			}
			after, _ := json.Marshal(c.character)
			if !bytes.Equal(before, after) || wires[0].Len() != 0 {
				t.Fatal("progression bypassed ownership", gate)
			}
		})
	}
}

func TestGMProgressPersistsBeforeReplyFailure(t *testing.T) {
	s, players, _ := chatFixture(t)
	c := players[0]
	s.SetGMLevel(c.account.ID, 1)
	c.conn = &failedWorldConn{}
	if err := s.dispatch(context.Background(), c, travelPacket(":points 9")); err == nil {
		t.Fatal("reply failure not surfaced")
	}
	if c.character.StatPoints != 9 {
		t.Fatal("committed points rolled back after reply failure")
	}
	assertProgressSaved(t, s, c)
}

func TestGMSkillDefaultsToGradeOne(t *testing.T) {
	s, players, wires := chatFixture(t)
	c := players[0]
	s.SetGMLevel(c.account.ID, 1)
	s.Assets.Skills[15101] = assets.Skill{ID: 15101}
	say(t, s, c, "/skill 15101")
	got := wires[0].packets(t)
	if len(got) != 4 || !bytes.Equal(got[1], []byte{5, 16, 0, 253, 58, 1}) {
		t.Fatal("default skill grade reply", got)
	}
	assertProgressSaved(t, s, c)
}
