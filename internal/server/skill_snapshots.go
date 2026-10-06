package server

import "wonderland-gonline/internal/game"

// Native AC5:3 overlays skill records (FUN_004381c4). Zero removed grades and
// EXP in the wire snapshot only; removed records must not survive persistence.
func skillSnapshotWithRemovals(previous, next game.Character) game.Character {
	snapshot := next.Clone()
	retained := map[uint16]bool{}
	clientID := func(id uint16) uint16 {
		if id == game.StarterStunt(next.Body, next.Head) {
			return game.StarterStuntClientID
		}
		return id
	}
	for _, skill := range next.Skills {
		retained[clientID(skill.ID)] = true
	}
	for _, skill := range previous.Skills {
		id := clientID(skill.ID)
		if !retained[id] {
			snapshot.Skills = append(snapshot.Skills, game.LearnedSkill{ID: skill.ID})
			retained[id] = true
		}
	}
	return snapshot
}
