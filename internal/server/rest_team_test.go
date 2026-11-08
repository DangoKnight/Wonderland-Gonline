package server

import (
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func restingTeamFixture(t *testing.T) (*Server, []*Session) {
	t.Helper()
	s, players, _ := partyFixture(t)
	p := &party{members: players}
	for _, c := range players {
		c.party = p
		next := c.character.Clone()
		next.HP, next.SP = 1, 0
		pet := game.NewPet(14162, "Roca", 1, s.petTemplate(14162), s.Assets.Items)
		pet.HP, pet.SP, pet.Battle = 1, 0, true
		next.Pets, next.ActivePet = []game.Pet{pet}, pet.ID
		if err := s.commit(context.Background(), c, next); err != nil {
			t.Fatal(err)
		}
		c.pets = newPetRoster()
		c.pets.register(pet.ID)
	}
	return s, players
}

func TestTeamRestingUsesOnlyLeaderPose(t *testing.T) {
	s, p := restingTeamFixture(t)
	t0 := time.Unix(100, 0)
	p[1].emote = protocol.PoseSitFirst
	p[2].emote = protocol.PoseLieFirst
	s.restCharacters(t0)
	s.restCharacters(t0.Add(restRecoveryInterval))
	for _, c := range p {
		if c.character.HP != 1 || c.character.Pets[0].HP != 1 {
			t.Fatal("followers rested without leader")
		}
	}
	p[0].emote = protocol.PoseSitFirst
	p[1].emote, p[2].emote = 0, 0
	s.restCharacters(t0.Add(2 * restRecoveryInterval))
	s.restCharacters(t0.Add(3 * restRecoveryInterval))
	for _, c := range p {
		if c.character.HP <= 1 || c.character.SP == 0 || c.character.Pets[0].HP <= 1 || c.character.Pets[0].SP == 0 {
			t.Fatal("leader did not heal standing member and active pet")
		}
	}
	// A follower sitting down must not reset its leader-governed interval.
	if err := s.worldCommand(context.Background(), p[1], []byte{32, 2, 46}); err != nil {
		t.Fatal(err)
	}
	hp := p[1].character.HP
	s.restCharacters(t0.Add(4 * restRecoveryInterval))
	if p[1].character.HP <= hp {
		t.Fatal("follower pose reset team recovery")
	}
	// Stop and restart between ticks must still require a full new interval.
	for _, pose := range []byte{0, 46} {
		if err := s.worldCommand(context.Background(), p[0], []byte{32, 2, pose}); err != nil {
			t.Fatal(err)
		}
	}
	hp = p[1].character.HP
	s.restCharacters(t0.Add(5 * restRecoveryInterval))
	s.restCharacters(t0.Add(5*restRecoveryInterval + time.Second))
	if p[1].character.HP != hp {
		t.Fatal("leader pose cycle retained old recovery clock")
	}
}

func TestTeamRestingLeadershipAndDisband(t *testing.T) {
	s, p := restingTeamFixture(t)
	t0 := time.Unix(100, 0)
	p[0].emote = 46
	s.restCharacters(t0)
	team := p[0].party
	team.members[0], team.members[1] = team.members[1], team.members[0]
	s.restCharacters(t0.Add(restRecoveryInterval))
	for _, c := range p {
		if c.character.HP != 1 {
			t.Fatal("old leader still controlled recovery")
		}
	}
	leader := team.leader()
	leader.emote = 46
	s.restCharacters(t0.Add(2 * restRecoveryInterval))
	s.restCharacters(t0.Add(3 * restRecoveryInterval))
	for _, c := range p {
		if c.character.HP <= 1 {
			t.Fatal("new leader did not enable recovery")
		}
	}
	// After disbanding, each player's own pose controls their recovery again.
	for _, c := range p {
		c.party = nil
	}
	s.restCharacters(t0.Add(4 * restRecoveryInterval))
	hp := p[2].character.HP
	s.restCharacters(t0.Add(5 * restRecoveryInterval))
	if p[2].character.HP != hp {
		t.Fatal("standing former follower continued recovering")
	}
}

func TestTeamRestingBlocksUnavailableMembersAndLeader(t *testing.T) {
	s, p := restingTeamFixture(t)
	p[0].emote = 46
	p[1].event = &eventSession{}
	p[2].ready = false
	t0 := time.Unix(100, 0)
	s.restCharacters(t0)
	s.restCharacters(t0.Add(restRecoveryInterval))
	if p[0].character.HP <= 1 || p[1].character.HP != 1 || p[2].character.HP != 1 {
		t.Fatal("unavailable members recovered")
	}
	p[1].event = nil
	p[2].ready = true
	p[0].battle = &battleRun{}
	hp := p[0].character.HP
	s.restCharacters(t0.Add(2 * restRecoveryInterval))
	s.restCharacters(t0.Add(3 * restRecoveryInterval))
	if p[0].character.HP != hp || p[1].character.HP != 1 || p[2].character.HP != 1 {
		t.Fatal("busy leader permitted recovery")
	}
}
