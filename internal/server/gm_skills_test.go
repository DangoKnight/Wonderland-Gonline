package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"reflect"
	"strconv"
	"testing"

	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

func clearSkillsFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	s, players, wires := chatFixture(t)
	s.Assets.Skills[15101] = assets.Skill{TableOrder: 4}
	s.Assets.Skills[11114] = assets.Skill{TableOrder: 5}
	s.Assets.Skills[12000] = assets.Skill{TableOrder: 6}
	c := players[0]
	next := c.character.Clone()
	next.Base.Strength = 28
	next.Skills = []game.LearnedSkill{{ID: 11075, Grade: 7, EXP: 123}, {ID: 11016, Grade: 8, EXP: 55}, {ID: 12000, Grade: 10, EXP: 9}}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	for _, wire := range wires {
		wire.Reset()
	}
	return s, players, wires
}

func TestGMClearSkillsPersistenceAndNativeReplay(t *testing.T) {
	s, p, w := clearSkillsFixture(t)
	c := p[0]
	s.SetGMLevel(c.account.ID, 1)
	before := c.character.Clone()
	say(t, s, c, "/clearskills")
	expected := before.Clone()
	expected.Skills = []game.LearnedSkill{{ID: 11075, Grade: 1}, {ID: 11016, Grade: 1}, {ID: 11166, Grade: 1}, {ID: 11056, Grade: 1}, {ID: 15101, Grade: 1}, {ID: 11114, Grade: 1}}
	if !reflect.DeepEqual(*c.character, expected) {
		t.Fatal("reset state", *c.character)
	}
	packets := w[0].packets(t)
	// Independent AC5:3 table records: stunt alias order 188, baseline and
	// stat-qualified skills at grade one, removed quest skill at grade zero.
	records := []byte{188, 0, 1, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 0, 2, 0, 1, 0, 0, 0, 0, 3, 0, 1, 0, 0, 0, 0, 4, 0, 1, 0, 0, 0, 0, 5, 0, 1, 0, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0}
	if len(packets) != 3 || !bytes.Equal(packets[0][:2], []byte{5, 3}) || !bytes.Equal(packets[0][62:64], []byte{7, 0}) || !bytes.Equal(packets[0][64:113], records) || !bytes.Equal(packets[1], []byte{5, 4}) || !bytes.HasPrefix(packets[2], []byte{2, 16}) {
		t.Fatal("native synchronization", packets)
	}
	// Replay the native indexed overwrite: old grade-ten skill disappears.
	table := map[uint16]byte{6: 10, 188: 7, 1: 8}
	for at := 64; at < 113; at += 7 {
		table[binary.LittleEndian.Uint16(packets[0][at:])] = packets[0][at+2]
	}
	if table[6] != 0 || table[188] != 1 || table[5] != 1 {
		t.Fatal("stale native skills", table)
	}
	assertProgressSaved(t, s, c)
	if w[1].Len() != 0 || w[2].Len() != 0 {
		t.Fatal("reset leaked to peers")
	}
	say(t, s, c, ":resetskills")
	if !reflect.DeepEqual(*c.character, expected) {
		t.Fatal("repeat changed unrelated state")
	}
	assertProgressSaved(t, s, c)
}

func TestGMClearSkillsTargetsAndAuthorization(t *testing.T) {
	s, p, w := clearSkillsFixture(t)
	actor, target := p[2], p[0]
	before, actorBefore := target.character.Clone(), actor.character.Clone()
	say(t, s, actor, "/clearskills "+target.character.Name)
	if !reflect.DeepEqual(*target.character, before) {
		t.Fatal("non-GM reset")
	}
	s.SetGMLevel(actor.account.ID, 1)
	for _, command := range []string{"/clearskills missing", "/clearskills one two"} {
		say(t, s, actor, command)
	}
	if !reflect.DeepEqual(*target.character, before) || !reflect.DeepEqual(*actor.character, actorBefore) || w[0].Len() != 0 {
		t.Fatal("invalid target reset a character")
	}
	w[2].Reset()
	say(t, s, actor, ":clearskills "+strconv.FormatUint(uint64(target.character.ID), 10))
	if len(target.character.Skills) != 6 || len(w[0].packets(t)) != 2 || len(w[2].packets(t)) != 1 || w[1].Len() != 0 {
		t.Fatal("cross-map target synchronization")
	}
	assertProgressSaved(t, s, target)
	s.SetGMLevel(actor.account.ID, 0)
	target.character.Skills[0].Grade = 9
	say(t, s, actor, "/resetskills "+target.character.Name)
	if target.character.Skills[0].Grade != 9 {
		t.Fatal("revoked GM reset")
	}
}

func TestGMClearSkillsInteractionGates(t *testing.T) {
	for _, owner := range []int{0, 1} {
		for _, gate := range []string{"loading", "trade", "battle", "event", "storm", "beach"} {
			t.Run(strconv.Itoa(owner)+gate, func(t *testing.T) {
				s, p, w := clearSkillsFixture(t)
				actor, target := p[1], p[0]
				s.SetGMLevel(actor.account.ID, 1)
				busy := target
				if owner == 1 {
					busy = actor
				}
				switch gate {
				case "loading":
					busy.ready = false
				case "trade":
					busy.trade = &tradeSession{}
				case "battle":
					busy.battle = &battleRun{}
				case "event":
					busy.event = &eventSession{}
				case "storm":
					busy.storm = true
				case "beach":
					busy.beach = &beachRun{}
				}
				before := target.character.Clone()
				err := s.worldCommand(context.Background(), actor, append([]byte{2, 2}, []byte("/clearskills "+target.character.Name)...))
				if !(owner == 1 && gate == "loading") && err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(*target.character, before) || w[0].Len() != 0 {
					t.Fatal("busy character reset")
				}
			})
		}
	}
}

func TestGMClearSkillsFailureRecovery(t *testing.T) {
	for _, failure := range []string{"save", "starter snapshot", "removed snapshot", "target delivery"} {
		t.Run(failure, func(t *testing.T) {
			s, p, w := clearSkillsFixture(t)
			actor, target := p[1], p[0]
			s.SetGMLevel(actor.account.ID, 1)
			before := target.character.Clone()
			ctx := context.Background()
			var broken *failedWorldConn
			switch failure {
			case "save":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "starter snapshot":
				delete(s.Assets.Skills, 15003)
			case "removed snapshot":
				delete(s.Assets.Skills, 12000)
			case "target delivery":
				broken = &failedWorldConn{}
				target.conn = broken
			}
			err := s.worldCommand(ctx, actor, append([]byte{2, 2}, []byte("/clearskills "+target.character.Name)...))
			if broken != nil {
				if err != nil || !broken.closed || target.character.Skills[0].Grade != 1 || len(w[1].packets(t)) != 1 {
					t.Fatal("delivery isolation", err)
				}
			} else if err == nil || !reflect.DeepEqual(*target.character, before) || w[0].Len() != 0 || w[1].Len() != 0 {
				t.Fatal("failed reset mutated/published", err)
			}
			assertProgressSaved(t, s, target)
		})
	}
}

func TestGMClearSkillsInstalledCatalog(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, element := range []byte{1, 2, 3, 4} {
		t.Run(strconv.Itoa(int(element)), func(t *testing.T) {
			s, p, w := chatFixture(t)
			s.Assets = catalog
			c := p[0]
			next := c.character.Clone()
			next.Element = element
			next.Base = game.Attributes{Strength: 100, Constitution: 100, Intelligence: 100, Wisdom: 100, Agility: 100}
			next.Skills = game.StarterSkills(next.Body, next.Head, element)
			for i := range next.Skills {
				next.Skills[i].Grade = 10
				next.Skills[i].EXP = 77
			}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			w[0].Reset()
			s.SetGMLevel(c.account.ID, 1)
			say(t, s, c, "/clearskills")
			if len(c.character.Skills) <= 4 {
				t.Fatal("qualified skills missing")
			}
			for _, skill := range c.character.Skills {
				if skill.Grade != 1 || skill.EXP != 0 {
					t.Fatal("training survived", skill)
				}
			}
			packets := w[0].packets(t)
			if len(packets) != 3 {
				t.Fatal("reset synchronization", packets)
			}
			if got := int(binary.LittleEndian.Uint16(packets[0][62:64])); got != len(c.character.Skills) {
				t.Fatal("SQL snapshot count", got)
			}
			assertProgressSaved(t, s, c)
		})
	}
}

func TestGMClearSkillsWaterDoesNotRestoreUnqualifiedIcicle(t *testing.T) {
	s, players, wires := clearSkillsFixture(t)
	c := players[0]
	next := c.character.Clone()
	next.Element, next.Base = game.Water, game.Attributes{Strength: 1}
	next.Skills = []game.LearnedSkill{{ID: 11075, Grade: 1}, {ID: 11001, Grade: 1}}
	for _, id := range []uint16{15091, 15097, 15100, 11001} {
		s.Assets.Skills[id] = assets.Skill{ID: id, TableOrder: id}
	}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	s.SetGMLevel(c.account.ID, 1)
	wires[0].Reset()
	say(t, s, c, "/clearskills")
	want := []game.LearnedSkill{{ID: 11075, Grade: 1}, {ID: 15091, Grade: 1}, {ID: 15097, Grade: 1}, {ID: 15100, Grade: 1}}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || !reflect.DeepEqual(c.character.Skills, want) || !reflect.DeepEqual(stored[0].Skills, want) {
		t.Fatal("reset retained unqualified Icicle", c.character.Skills, stored, err)
	}
}
