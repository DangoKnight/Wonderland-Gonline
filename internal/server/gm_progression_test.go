package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
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

func TestRebornStoryCheckpointPreservesTrialsAndCompletionTime(t *testing.T) {
	for _, state := range []game.QuestState{game.NotStarted, game.InProgress, game.Completed} {
		t.Run(strconv.Itoa(int(state)), func(t *testing.T) {
			s, c, _, _ := mallFixture(t)
			c.character.SetLevel(100)
			c.character.Refill(s.Assets.Items)
			old := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			if state != game.NotStarted {
				c.character.Quests[15090] = game.Quest{ID: 15090, State: state, Step: 12}
				if state == game.Completed {
					q := c.character.Quests[15090]
					q.CompletedAt = &old
					c.character.Quests[15090] = q
				}
			}
			if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
				t.Fatal(err)
			}
			if err := s.performReborn(context.Background(), c, game.JobKiller, false); err != nil {
				t.Fatal(err)
			}
			trials, done := c.character.Quests[15090], c.character.Quests[15091]
			want := 12
			if state == game.NotStarted {
				want = 1
			}
			if !c.character.Reborn || trials.State != game.Completed || trials.Step != want || done.State != game.InProgress || done.Step != 1 {
				t.Fatal("rebirth checkpoint", trials, done)
			}
			if state == game.Completed && (trials.CompletedAt == nil || !trials.CompletedAt.Equal(old)) {
				t.Fatal("old completion timestamp replaced")
			}
			assertProgressSaved(t, s, c)
		})
	}
}

func TestRebornStoryCheckpointFailureRollsBackRebirth(t *testing.T) {
	s, c, _, path := mallFixture(t)
	ctx := context.Background()
	c.character.SetLevel(100)
	c.character.Refill(s.Assets.Items)
	c.character.Quests[15090] = game.Quest{ID: 15090, State: game.InProgress, Step: 12}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	before := c.character.Clone()
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec("CREATE TRIGGER reject_reborn_checkpoint BEFORE INSERT ON character_quests WHEN NEW.quest_id=15091 BEGIN SELECT RAISE(ABORT,'checkpoint failed'); END"); err != nil {
		t.Fatal(err)
	}
	if err := s.performReborn(ctx, c, game.JobKiller, false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*c.character, before) {
		t.Fatal("failed checkpoint changed the live character")
	}
	assertProgressSaved(t, s, c)
	if _, err := raw.Exec("DROP TRIGGER reject_reborn_checkpoint"); err != nil {
		t.Fatal(err)
	}
	if err := s.performReborn(ctx, c, game.JobKiller, false); err != nil || !c.character.Reborn || c.character.Quests[15091].Step != 1 {
		t.Fatal("checkpoint retry", err)
	}
}

func TestRebornStoryLoginRepairUsesFreshSQLAndSkipsCompleted(t *testing.T) {
	s, c, _, path := mallFixture(t)
	ctx := context.Background()
	c.character.RebornAs(game.JobKiller, true)
	c.character.Refill(s.Assets.Items)
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	stale := c.character.Clone()
	if err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(current *game.Character) error {
		current.Gold = 777
		current.Quests[15090] = game.Quest{ID: 15090, State: game.InProgress, Step: 12}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	next, err := s.reconcileRebornCheckpoints(ctx, c.account.ID, stale)
	if err != nil || next.Gold != 777 || next.Quests[15090].Step != 12 || next.Quests[15091].Step != 1 {
		t.Fatal("backfill used stale state", next.Gold, err)
	}
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec("CREATE TRIGGER reject_repeated_repair BEFORE UPDATE ON character_state BEGIN SELECT RAISE(ABORT,'unnecessary repair'); END"); err != nil {
		t.Fatal(err)
	}
	again, err := s.reconcileRebornCheckpoints(ctx, c.account.ID, next)
	if err != nil || !reflect.DeepEqual(again, next) {
		t.Fatal("completed checkpoint was rewritten", err)
	}
}

func TestRebornStoryCheckpointIsContentDriven(t *testing.T) {
	s, c, _, _ := mallFixture(t)
	rule := assets.StoryRule{ID: "custom_rebirth", Kind: assets.StoryRebornCheckpoint, Scope: assets.StoryScope{AllowMissing: true}, Gates: []assets.StoryGate{{Source: "mark_exists", ID: 100001, Op: "eq", Value: 0}}, Marks: []assets.StoryMark{{ID: 100000, Mode: "complete", PreserveStep: true, Value: 1}, {ID: 100001, Mode: "ensure_active", Value: 1}}}
	s.World.ReloadStory(assets.StoryDefinitions{Rules: []assets.StoryRule{rule}})
	if err := s.performReborn(context.Background(), c, game.JobKiller, true); err != nil {
		t.Fatal(err)
	}
	if c.character.Quests[100001].Step != 1 || c.character.Quests[100000].State != game.Completed || c.character.Quests[15091].ID != 0 {
		t.Fatal("checkpoint IDs remained hardcoded")
	}
	s.World.ReloadStory(assets.StoryDefinitions{})
	delete(c.character.Quests, 100001)
	next, err := s.reconcileRebornCheckpoints(context.Background(), c.account.ID, c.character.Clone())
	if err != nil || next.Quests[100001].ID != 0 || next.Quests[15091].ID != 0 {
		t.Fatal("cleared definitions restored a checkpoint", err)
	}
}

func TestRebornStoryCheckpointRepairsDuringCharacterSelection(t *testing.T) {
	s, c, _, _ := mallFixture(t)
	ctx := context.Background()
	c.character.RebornAs(game.JobKiller, true)
	c.character.Refill(s.Assets.Items)
	c.character.Quests[15090] = game.Quest{ID: 15090, State: game.InProgress, Step: 12}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	login := &Session{conn: &captureConn{}, info: SessionInfo{ID: 77}, account: c.account}
	if err := s.login(ctx, login, []byte{63, 2, 1}); err != nil {
		t.Fatal(err)
	}
	if login.character == nil || login.character.Quests[15091].Step != 1 || login.character.Quests[15090].State != game.Completed {
		t.Fatal("login snapshot preceded checkpoint repair")
	}
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || stored[0].Quests[15091].Step != 1 {
		t.Fatal("login checkpoint not durable", err)
	}
	// The existing completion mark is authoritative even if a later content
	// edit leaves the trial mark in progress.
	if err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(current *game.Character) error {
		current.Quests[15090] = game.Quest{ID: 15090, State: game.InProgress, Step: 7}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	latest, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	latest[0], err = s.reconcileRebornCheckpoints(ctx, c.account.ID, latest[0])
	if err != nil || latest[0].Quests[15090].Step != 7 || latest[0].Quests[15090].State != game.InProgress {
		t.Fatal("existing completion did not suppress backfill", err)
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
	if len(packets) != 17 || !bytes.Equal(packets[0], []byte{8, 1, 36, 1, 49, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatal("level EXP synchronization", packets)
	}
	if !contains(packets, []byte{8, 1, 35, 1, 3, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatal("native level stat missing", packets)
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
	if len(got) != 16 || !bytes.Equal(got[7], []byte{8, 1, 27, 1, 32, 0, 0, 0, 0, 0, 0, 0}) || c.character.HP != c.character.MaxHP {
		t.Fatal("avatar bonuses or refill incorrect", got)
	}
	assertProgressSaved(t, s, c)
	points := c.character.StatPoints
	say(t, s, c, "/exp 14")
	got = wires[0].packets(t)
	if c.character.Level != 2 || c.character.EXP != 14 || c.character.StatPoints != points || len(got) != 17 {
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

func TestGMRebornOwnershipJobSkillResetAndReplay(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	s.Assets.Items[25231] = game.ItemDefinition{ID: 25231, Type: 16, EquipSlot: 6}
	s.Assets.RebornClasses = []assets.RebornClass{{Job: game.JobKiller, Name: "Killer", CapeID: 25231, Enabled: true}}
	c.character.SetLevel(100)
	c.character.Refill(s.Assets.Items)
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	// The owning chat dispatcher checks GM authority before invoking the helper.
	c.gmLevel.Store(1)
	if err := s.gmReborn(ctx, c, "reborn", []string{"Killer"}); err != nil {
		t.Fatal(err)
	}
	if !c.character.Reborn || c.character.Job != 1 || c.character.RebornJob != 1 || c.character.Level != 1 || c.character.EXP != 0 || c.character.Bag[0].ID != 0 {
		t.Fatal(c.character)
	}
	if !contains(wire.packets(t), []byte{5, 5, 17, 39, 0, 0, 0, 0}) {
		t.Fatal("rebirth shape reset missing")
	}
	before := c.character.Clone()
	if err := s.gmReborn(ctx, c, "reborn", []string{"Killer"}); err != nil || c.character.Bag != before.Bag || c.character.Job != before.Job {
		t.Fatal("reborn replay", err)
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].Job != 1 || saved[0].EXP != 0 {
		t.Fatal(saved, err)
	}
}

func TestGMRebornLowLevelGateAndFullBagAllowed(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(strconv.FormatBool(full), func(t *testing.T) {
			s, c, _, _ := mallFixture(t)
			ctx := context.Background()
			s.Assets.Items[25231] = game.ItemDefinition{ID: 25231, Type: 16, EquipSlot: 6}
			s.Assets.RebornClasses = []assets.RebornClass{{Job: game.JobKiller, Name: "Killer", CapeID: 25231, Enabled: true}}
			if full {
				c.character.SetLevel(100)
				for i := range c.character.Bag {
					c.character.Bag[i] = game.Item{ID: 32176, Count: 50}
				}
			}
			if err := s.commit(ctx, c, c.character.Clone()); err != nil {
				t.Fatal(err)
			}
			before := c.character.Clone()
			if err := s.gmReborn(ctx, c, "reborn", []string{"Killer"}); err != nil {
				t.Fatal(err)
			}
			if c.character.Reborn != full || c.character.Bag != before.Bag || (!full && (c.character.Job != 0 || c.character.Level != before.Level || c.character.EXP != before.EXP)) {
				t.Fatal("rejected rebirth changed character")
			}
			got, err := s.Store.Characters(ctx, c.account.ID)
			if err != nil || got[0].Reborn != full || got[0].Bag != before.Bag {
				t.Fatal("rejected rebirth changed SQL", err)
			}
		})
	}
}

func gmRepairFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	s, players, wires := repairFixture(t)
	for _, c := range players {
		s.sessions[c.info.ID] = c
		s.accounts[c.account.ID] = c.info.ID
	}
	return s, players, wires
}

func TestGMRepairPacketsAndPersistence(t *testing.T) {
	s, players, wires := gmRepairFixture(t)
	c := players[0]
	s.SetGMLevel(c.account.ID, 1)
	next := c.character.Clone()
	next.Equipment = game.Equipment{}
	next.Equipment[0] = game.Item{ID: 100, Count: 1, Damage: 99, Metadata: [26]byte{8}}
	next.Equipment[0].Metadata[game.ForgeMetadataOffset] = 3
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	before := next.Clone()
	say(t, s, c, "/repair")
	expected := before.Clone()
	expected.Bag[4].Damage, expected.Bag[8].Damage, expected.Equipment[0].Damage = 0, 0, 0
	if !reflect.DeepEqual(*c.character, expected) {
		t.Fatal("repair changed more than durability")
	}
	packets := wires[0].packets(t)
	if len(packets) != 5 || !bytes.Equal(packets[0], []byte{23, 9, 5, 2}) || !bytes.Equal(packets[1], []byte{23, 9, 9, 2}) {
		t.Fatal(packets)
	}
	// Reapply raw native additive inventory records to the pre-repair client state.
	client := before.Bag
	for _, p := range packets[:3] {
		switch p[1] {
		case 9:
			if err := client.Remove(p[2], p[3]); err != nil {
				t.Fatal(err)
			}
		case 5:
			for i := 2; i < len(p); i += 31 {
				r := p[i : i+31]
				if !client[r[0]-1].Empty() {
					t.Fatal("repaired stack overlaid old record")
				}
				item := game.Item{ID: uint16(r[1]) | uint16(r[2])<<8, Count: r[3], Damage: r[4]}
				copy(item.Metadata[:], r[5:])
				client[r[0]-1] = item
			}
		}
	}
	if client != expected.Bag {
		t.Fatal("native bag diverged")
	}
	gear := append([]byte{23, 11, 100, 0, 0}, make([]byte, 18)...)
	gear[17] = 3
	if !bytes.Equal(packets[3], gear) {
		t.Fatal("equipment durability/forge layout", packets[3])
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || !reflect.DeepEqual(saved[0], expected) {
		t.Fatal("repair not durable", err)
	}
	if wires[1].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("private repair leaked")
	}
	say(t, s, c, ":fixall")
	if packets := wires[0].packets(t); len(packets) != 1 || packets[0][0] != 2 {
		t.Fatal("healthy replay rewrote inventory", packets)
	}
}

func TestGMRepairTargetsAndAuthorization(t *testing.T) {
	s, players, wires := gmRepairFixture(t)
	actor, target := players[1], players[0]
	before := target.character.Clone()
	say(t, s, actor, "/repair "+target.character.Name)
	if !reflect.DeepEqual(*target.character, before) || wires[0].Len() != 0 {
		t.Fatal("non-GM repaired target")
	}
	s.SetGMLevel(actor.account.ID, 1)
	say(t, s, actor, "/repair missing")
	say(t, s, actor, "/repair one two")
	if !reflect.DeepEqual(*target.character, before) || wires[0].Len() != 0 {
		t.Fatal("bad target changed state")
	}
	wires[1].Reset()
	say(t, s, actor, "/fixall "+strconv.FormatUint(uint64(target.character.ID), 10))
	if target.character.Bag[4].Damage != 0 || target.character.Gold != before.Gold || wires[0].Len() == 0 || len(wires[1].packets(t)) != 1 {
		t.Fatal("target repair failed")
	}
	if wires[2].Len() != 0 {
		t.Fatal("target repair broadcast")
	}
	s.SetGMLevel(actor.account.ID, 0)
	target.character.Bag[4].Damage = 1
	say(t, s, actor, "/repair "+target.character.Name)
	if target.character.Bag[4].Damage != 1 {
		t.Fatal("revoked GM repaired")
	}
}

func TestGMRepairInteractionGates(t *testing.T) {
	for _, owner := range []int{0, 1} {
		for _, gate := range []string{"loading", "battle", "event", "storm", "beach", "trade", "minigame"} {
			t.Run(strconv.Itoa(owner)+gate, func(t *testing.T) {
				s, players, wires := gmRepairFixture(t)
				actor, target := players[1], players[0]
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
				case "event":
					c.event = &eventSession{}
				case "storm":
					c.storm = true
				case "beach":
					c.beach = &beachRun{}
				case "trade":
					c.trade = &tradeSession{}
				case "minigame":
					c.event = &eventSession{onMinigame: func(byte) error { return nil }}
				}
				before := target.character.Clone()
				err := s.worldCommand(context.Background(), actor, append([]byte{2, 2}, []byte("/repair "+target.character.Name)...))
				if owner == 1 && gate == "loading" {
					if err == nil {
						t.Fatal("loading actor accepted")
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(*target.character, before) || wires[0].Len() != 0 {
					t.Fatal("busy repair changed target")
				}
			})
		}
	}
}

func TestGMRepairFailedSave(t *testing.T) {
	s, players, wires := gmRepairFixture(t)
	c := players[0]
	s.SetGMLevel(c.account.ID, 1)
	before := c.character.Clone()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.worldCommand(ctx, c, append([]byte{2, 2}, []byte("/repair")...)); err == nil {
		t.Fatal("missing save failure")
	}
	if !reflect.DeepEqual(*c.character, before) || wires[0].Len() != 0 {
		t.Fatal("failed save mutated or published")
	}
}

func TestGMRepairFailedTargetDelivery(t *testing.T) {
	s, players, wires := gmRepairFixture(t)
	actor, target := players[1], players[0]
	s.SetGMLevel(actor.account.ID, 1)
	broken := &failedWorldConn{}
	target.conn = broken
	say(t, s, actor, "/repair "+target.character.Name)
	if !broken.closed || target.character.Bag[4].Damage != 0 {
		t.Fatal("committed repair lost or failed target not disconnected")
	}
	saved, err := s.Store.Characters(context.Background(), target.account.ID)
	if err != nil || saved[0].Bag != target.character.Bag {
		t.Fatal("repair not saved", err)
	}
	if len(wires[1].packets(t)) != 1 {
		t.Fatal("GM not told to reconnect target")
	}
}

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
	if len(packets) != 18 || !bytes.Equal(packets[11], []byte{8, 1, 38, 1, 107, 0, 0, 0, 0, 0, 0, 0}) || packets[16][0] != 5 || packets[16][1] != 3 {
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
	if target.character.StatPoints != 107 || len(w[0].packets(t)) != 17 || len(w[2].packets(t)) != 1 || w[1].Len() != 0 {
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
