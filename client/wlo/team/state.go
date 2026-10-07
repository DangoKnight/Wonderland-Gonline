// Package team ports TSe_TeamForm and the native AC13/AC8:3 party state.
package team

import (
	"encoding/binary"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

const (
	MaximumMembers    = 4
	teammateStatBytes = 16
	rosterHeaderBytes = 7
)

type Member struct {
	ID    uint32
	Stats world.Stats
}
type State struct {
	Self, Leader        uint32
	Members             []uint32
	Vitals              map[uint32]*Member
	BattlePet, MountPet uint16
}

func (s *State) Reset(self uint32) { *s = State{Self: self, Vitals: map[uint32]*Member{}} }
func (s *State) InParty() bool     { return len(s.Members) > 1 }
func (s *State) Others() []uint32 {
	var ids []uint32
	for _, id := range s.Members {
		if id != s.Self {
			ids = append(ids, id)
		}
	}
	return ids
}

// FUN_00407ccc: leader U32, follower count U8, follower IDs U32.
func (s *State) ApplyRoster(p []byte) bool {
	if len(p) < rosterHeaderBytes || p[0] != protocol.CommandTeam || p[1] != protocol.TeamRoster || p[6] >= MaximumMembers || len(p) != rosterHeaderBytes+int(p[6])*4 {
		return false
	}
	ids := []uint32{binary.LittleEndian.Uint32(p[2:])}
	if ids[0] == 0 {
		return false
	}
	seen := map[uint32]bool{ids[0]: true}
	for at := rosterHeaderBytes; at < len(p); at += 4 {
		id := binary.LittleEndian.Uint32(p[at:])
		if id == 0 || seen[id] {
			return false
		}
		seen[id] = true
		ids = append(ids, id)
	}
	// AC13:6 can also describe other teams in a scene. Never adopt their roster.
	if !seen[s.Self] {
		return true
	}
	s.Leader, s.Members = ids[0], ids
	for id := range s.Vitals {
		if !seen[id] {
			delete(s.Vitals, id)
		}
	}
	return true
}

// FUN_00451718 -> FUN_0043b7bc: separate stat/sign bytes, U32 value/target.
// In particular 0x23 is level and 0x19 is current HP, not maximum HP.
func (s *State) ApplyStat(p []byte) bool {
	if len(p) != teammateStatBytes || p[0] != protocol.CommandStats || p[1] != protocol.StatsWireCode3 || p[7] < protocol.StatValuePositive || p[7] > protocol.StatValueNegative || binary.LittleEndian.Uint32(p[12:]) != 0 {
		return false
	}
	id := binary.LittleEndian.Uint32(p[2:])
	if id == 0 || id == s.Self {
		return false
	}
	v := binary.LittleEndian.Uint32(p[8:])
	if p[7] == protocol.StatValueNegative {
		v = uint32(-int32(v))
	}
	if s.Vitals == nil {
		s.Vitals = map[uint32]*Member{}
	}
	m := s.Vitals[id]
	if m == nil {
		m = &Member{ID: id}
		s.Vitals[id] = m
	}
	m.Stats.Apply(p[6], v)
	return true
}
