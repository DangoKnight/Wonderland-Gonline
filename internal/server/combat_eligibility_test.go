package server

import (
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/battle"
)

func TestBattleDialogueLocksAndScriptedTransition(t *testing.T) {
	s, p, _ := partyFixture(t)
	group := &party{members: []*Session{p[0], p[1]}}
	p[0].party, p[1].party = group, group
	leaderEvent := &eventSession{}
	p[0].event = leaderEvent
	if battleInteractionAvailable(p[1], nil) {
		t.Fatal("follower bypassed leader event")
	}
	if err := s.startBattle(p[1], &battleRun{}, []battle.Enemy{{Template: 10500, Level: 5, HP: 100}}); err != nil || p[1].battle != nil || p[0].battle != nil {
		t.Fatal("ambient battle interrupted leader dialogue", err)
	}
	p[0].event = nil
	p[1].event = &eventSession{}
	if err := s.startBattle(p[1], &battleRun{}, []battle.Enemy{{Template: 10500, Level: 5, HP: 100}}); err != nil || p[1].battle != nil {
		t.Fatal("battle interrupted own dialogue", err)
	}
	p[1].event = nil
	p[0].event = leaderEvent
	// A quest can explicitly transition out of its own dialogue into combat.
	run := &battleRun{event: leaderEvent}
	if err := s.startBattle(p[0], run, []battle.Enemy{{Template: 10500, Level: 5, HP: 100}}); err != nil {
		t.Fatal(err)
	}
	if run.b == nil || len(run.members) != 2 || p[1].battle != run {
		t.Fatal("own scripted transition lost eligible party")
	}
	run.timer.Stop()
}

func TestEnemyDropsDialogueAndBusyLeaderTargets(t *testing.T) {
	for _, leaderBusy := range []bool{false, true} {
		s, c, _ := boundedEncounterFixture(t)
		now := time.Now()
		s.simulateWorldTick(now, false)
		if s.actorPursuits[30001][4].target != c.character.ID {
			t.Fatal("fixture did not acquire target")
		}
		if leaderBusy {
			leader := &Session{character: c.character, ready: true, event: &eventSession{}}
			c.party = &party{members: []*Session{leader, c}}
		} else {
			c.event = &eventSession{}
		}
		s.simulateWorldTick(now.Add(actorTargetScanInterval), false)
		if s.actorPursuits[30001][4].target != 0 || c.battle != nil {
			t.Fatal("enemy retained locked dialogue/team target")
		}
		n, _ := s.World.NPC(30001, 4)
		if err := s.proximityEncounter(c, n); err != nil || c.battle != nil {
			t.Fatal("proximity bypassed dialogue lock", err)
		}
		if handled, err := s.wildClick(c, n); err != nil || !handled || c.battle != nil {
			t.Fatal("wild click bypassed dialogue lock", handled, err)
		}
	}
}

func TestPKFollowerIncludesWholeTeamOrRejectsBusyMember(t *testing.T) {
	for _, busyMember := range []int{-1, 1, 2} {
		s, p, _ := partyFixture(t)
		group := &party{members: []*Session{p[1], p[2]}}
		p[1].party, p[2].party = group, group
		for _, c := range p {
			settings := c.character.Preferences()
			settings.PKAllowed = true
			c.character.Settings = &settings
		}
		if busyMember >= 0 {
			p[busyMember].event = &eventSession{}
		}
		if err := s.worldCommand(context.Background(), p[0], pkRequest(p[2].character.ID)); err != nil {
			t.Fatal(err)
		}
		if busyMember >= 0 {
			for _, c := range p {
				if c.battle != nil {
					t.Fatal("PK formed a partial party around dialogue", busyMember)
				}
			}
			continue
		}
		run := p[0].battle
		if run == nil || !run.b.PvP || len(run.members) != 3 {
			t.Fatal("PK omitted target's team")
		}
		run.timer.Stop()
		if p[1].battle != run || p[2].battle != run || run.members[1].c != p[1] {
			t.Fatal("PK did not preserve defender team order")
		}
	}
}

func TestBattleAdmissionBlockedByDialogue(t *testing.T) {
	s, p, _, run := admissionFixture(t)
	event := &eventSession{}
	p[1].event = event
	for _, watch := range []bool{false, true} {
		if err := s.worldCommand(context.Background(), p[1], entryRequest(p[0], watch)); err != nil {
			t.Fatal(err)
		}
		if p[1].battle != nil || p[1].event != event || len(run.members) != 1 {
			t.Fatal("join/watch interrupted dialogue")
		}
	}
	p[1].event = nil
	group := &party{members: []*Session{p[2], p[1]}}
	p[1].party, p[2].party = group, group
	p[2].event = event
	if battleInteractionAvailable(p[1], nil) {
		t.Fatal("following member ignored leader's event")
	}
	if err := s.worldCommand(context.Background(), p[1], entryRequest(p[0], false)); err != nil || p[1].battle != nil {
		t.Fatal("follower joined during leader dialogue", err)
	}
}
