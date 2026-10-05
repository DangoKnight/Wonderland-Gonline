package server

import (
	"wonderland-go/internal/battle"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// battleStateCommand decodes the native AC11:2 player PK request before changing
// either party. raw character ID is authoritative; the trailing click ID is NPC-only.
func (s *Server) battleStateCommand(c *Session, p []byte) error {
	if len(p) < 2 {
		return ErrUnsupported
	}
	if c.battle != nil {
		return s.battleCommand(c, p)
	}
	if p[1] != protocol.BattleStateChallenge {
		return nil
	}
	if len(p) != 9 {
		return protocol.ErrMalformed
	}
	if p[2] != protocol.BattleChallengePlayer {
		return nil
	}
	targetID := uint32(p[3]) | uint32(p[4])<<8 | uint32(p[5])<<16 | uint32(p[6])<<24
	target := s.friendSessions[targetID]
	if target == nil || target == c || target.character == nil || !s.samePlayerScene(target, c) || c.tentOwner != 0 ||
		!c.character.Preferences().PKAllowed || !target.character.Preferences().PKAllowed ||
		!gmIdle(c) || !gmIdle(target) || (c.party != nil && c.party == target.party) {
		return c.send(headBanner("The player is unavailable for PK."))
	}
	return s.startPvP(c, target)
}

// startPvP initializes two parties through the same turn/persistence pipeline as PvE.
// Caller holds worldMu, so no participant can join two battles during setup.
func (s *Server) startPvP(attacker, defender *Session) error {
	run := &battleRun{}
	teams := [][]*Session{s.battleTeam(attacker), s.battleTeam(defender)}
	seen := map[*Session]bool{}
	for _, team := range teams {
		for _, c := range team {
			if seen[c] || !gmIdle(c) || !c.character.Preferences().PKAllowed {
				return attacker.send(headBanner("A party member is unavailable for PK."))
			}
			seen[c] = true
		}
	}
	var fighters [2][]*battle.Fighter
	for side, team := range teams {
		for i, c := range team {
			m := &battleMember{c: c, self: battle.PlayerFighter(c.character, s.Assets.Items, i)}
			battle.Place(m.self, battle.Side(side+1), i)
			fighters[side] = append(fighters[side], m.self)
			if pet := c.character.BattlePet(); pet != nil {
				copy := *pet
				copy.Skills = append([]game.PetSkill(nil), pet.Skills...)
				copy.Normalize(s.Assets.Items, false)
				copy.Name = s.companionName(copy.ID, copy.Name)
				m.pet = battle.PetFighter(copy, c.character.ID, s.Assets.Items, i)
				battle.Place(m.pet, battle.Side(side+1), i)
				fighters[side] = append(fighters[side], m.pet)
			}
			run.members = append(run.members, m)
		}
	}
	run.b = battle.NewPvP(fighters[0], fighters[1])
	for _, m := range run.members {
		m.c.battle = run
	}
	background := uint16(1)
	if attacker.character.Map < game.MapID10000 {
		background = attacker.character.Map
	}
	for _, m := range run.members {
		if err := s.battleIntro(run, m, background); err != nil {
			run.b.Finished = true
			for _, member := range run.members {
				member.c.battle = nil
				member.c.conn.Close()
			}
			return err
		}
	}
	s.armTurnTimer(run)
	return nil
}
