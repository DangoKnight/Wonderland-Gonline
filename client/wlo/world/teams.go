package world

import (
	"encoding/binary"
	"image"
	"slices"
	"time"

	"wonderland-gonline/internal/protocol"
)

const (
	// FUN_004245bc: five positions, shifted after >19 pixels on either axis.
	teamTrailSamples = 5
	teamTrailSpacing = 20
)

type teamTrail struct {
	points      [teamTrailSamples]image.Point
	last        image.Point
	initialized bool
	relocated   bool
	joining     map[uint32]bool // New followers awaiting their native initial snap.
}

// TeamMembers includes the leader, followed by the native formation order.
func (w *World) TeamMembers(id uint32) []uint32 {
	for _, members := range w.Teams {
		if slices.Contains(members, id) {
			return slices.Clone(members)
		}
	}
	return nil
}

func (w *World) inTeam(id uint32) bool {
	for _, members := range w.Teams {
		if len(members) > 1 && slices.Contains(members, id) {
			return true
		}
	}
	return false
}

func (w *World) FollowingTeam() bool {
	members := w.TeamMembers(w.Player.ID)
	return len(members) > 1 && members[0] != w.Player.ID && w.Peers[members[0]] != nil
}

// SetTeamRoster retains relationships even before remote appearances arrive.
func (w *World) SetTeamRoster(members []uint32) bool {
	if len(members) == 0 || len(members) > protocol.TeamMaximumMembers {
		return false
	}
	seen := map[uint32]bool{}
	for _, id := range members {
		if id == 0 || seen[id] {
			return false
		}
		seen[id] = true
	}
	if w.Teams == nil {
		w.Teams = map[uint32][]uint32{}
	}
	if w.teamTrails == nil {
		w.teamTrails = map[uint32]*teamTrail{}
	}
	if slices.Equal(w.Teams[members[0]], members) {
		return true
	}
	trail := w.teamTrails[members[0]]
	if trail == nil {
		trail = &teamTrail{}
	}
	if trail.joining == nil {
		trail.joining = map[uint32]bool{}
	}
	for _, id := range members[1:] {
		if len(w.TeamMembers(id)) < 2 {
			trail.joining[id] = true
		}
	}
	for id := range trail.joining {
		if !seen[id] {
			delete(trail.joining, id)
		}
	}
	for leader, old := range w.Teams {
		if slices.ContainsFunc(old, func(id uint32) bool { return seen[id] }) {
			for _, id := range old {
				w.resetTeamCompanion(id)
			}
			for _, id := range old[1:] {
				w.stopTeamFollower(id)
			}
			delete(w.Teams, leader)
			delete(w.teamTrails, leader)
		}
	}
	if len(members) > 1 {
		for _, id := range members {
			w.resetTeamCompanion(id)
		}
		w.Teams[members[0]] = slices.Clone(members)
		w.teamTrails[members[0]] = trail
		w.placeJoiningFollowers(members, trail)
	}
	if slices.Contains(members, w.Player.ID) {
		w.StopWalk()
		w.Marker.Hide()
	}
	return true
}

// ApplyTeam handles public formation independently of the local Team window.
func (w *World) ApplyTeam(p []byte) bool {
	if len(p) < 2 || p[0] != protocol.CommandTeam {
		return false
	}
	switch p[1] {
	case protocol.TeamFormation:
		if len(p) != protocol.TeamFormationBytes {
			return false
		}
		leader, follower := binary.LittleEndian.Uint32(p[2:]), binary.LittleEndian.Uint32(p[6:])
		// Some compatible servers also emit this alongside AC15:4 pet info.
		// Native player lookup does not make those NPCs into party members.
		if pet := w.Companions[leader]; pet != nil && pet.ID == follower && follower != w.Player.ID && w.Peers[follower] == nil {
			return true
		}
		members := slices.Clone(w.Teams[leader])
		if len(members) == 0 {
			members = []uint32{leader}
		}
		if slices.Contains(members, follower) {
			return follower != 0 && follower != leader
		}
		return w.SetTeamRoster(append(members, follower))
	case protocol.TeamRoster:
		if len(p) < protocol.TeamRosterHeaderBytes || int(p[6]) >= protocol.TeamMaximumMembers || len(p) != protocol.TeamRosterHeaderBytes+4*int(p[6]) {
			return false
		}
		members := []uint32{binary.LittleEndian.Uint32(p[2:])}
		for at := protocol.TeamRosterHeaderBytes; at < len(p); at += 4 {
			members = append(members, binary.LittleEndian.Uint32(p[at:]))
		}
		return w.SetTeamRoster(members)
	case protocol.TeamLeave:
		if len(p) != protocol.TeamIDPacketBytes {
			return false
		}
		id := binary.LittleEndian.Uint32(p[2:])
		members := w.TeamMembers(id)
		if len(members) == 0 {
			return id != 0
		}
		delete(w.Teams, members[0])
		delete(w.teamTrails, members[0])
		for _, member := range members {
			w.resetTeamCompanion(member)
		}
		for _, follower := range members[1:] {
			w.stopTeamFollower(follower)
		}
		if id != members[0] {
			members = slices.DeleteFunc(members, func(member uint32) bool { return member == id })
			w.SetTeamRoster(members)
		}
		if id == w.Player.ID || slices.Contains(members, w.Player.ID) {
			w.StopWalk()
		}
		return true
	case protocol.TeamLeadershipNotice:
		if len(p) != protocol.TeamLeaderChangeNoticeBytes || p[2] != protocol.TeamLeadershipChanged {
			return false
		}
		next, old := binary.LittleEndian.Uint32(p[3:]), binary.LittleEndian.Uint32(p[7:])
		members := slices.Clone(w.Teams[old])
		at := slices.Index(members, next)
		if at < 1 {
			return false
		}
		members[0], members[at] = members[at], members[0]
		return w.SetTeamRoster(members)
	}
	return false
}

func (w *World) stopTeamFollower(id uint32) {
	if id == w.Player.ID {
		w.StopWalk()
	} else if peer := w.Peers[id]; peer != nil {
		peer.Walker.Stop(&peer.Player)
	}
}

func (w *World) resetTeamCompanion(owner uint32) {
	if pet := w.Companions[owner]; pet != nil {
		pet.initialized = false
		pet.walker.Stop(&pet.position)
	}
}

// FUN_00406e0c places a new follower at the leader's current feet immediately.
// Missing appearances defer this placement, never turn it into a catch-up walk.
func (w *World) placeJoiningFollowers(members []uint32, trail *teamTrail) {
	leader := w.companionOwner(members[0])
	if leader == nil {
		return
	}
	for _, id := range members[1:] {
		if !trail.joining[id] {
			continue
		}
		follower := w.companionOwner(id)
		if follower == nil {
			continue
		}
		w.stopTeamFollower(id)
		follower.X, follower.Y = leader.X, leader.Y
		if id == w.Player.ID {
			w.followCamera.ready = false
		}
		delete(trail.joining, id)
	}
}

func (w *World) stepTeamFollowing(now time.Time) {
	for leader, members := range w.Teams {
		if w.TeamFollowingPaused && slices.Contains(members, w.Player.ID) {
			continue
		}
		p := w.companionOwner(leader)
		if p == nil {
			continue
		}
		trail := w.teamTrails[leader]
		if trail == nil {
			if w.teamTrails == nil {
				w.teamTrails = map[uint32]*teamTrail{}
			}
			trail = &teamTrail{}
			w.teamTrails[leader] = trail
		}
		at := image.Pt(p.X, p.Y)
		w.placeJoiningFollowers(members, trail)
		jump := trail.relocated || trail.initialized && (abs(at.X-trail.last.X) >= companionTeleportDistance || abs(at.Y-trail.last.Y) >= companionTeleportDistance)
		trail.relocated = false
		if !trail.initialized || jump {
			for i := range trail.points {
				trail.points[i] = at
			}
			trail.initialized = true
		}
		trail.last = at
		if abs(at.X-trail.points[0].X) >= teamTrailSpacing || abs(at.Y-trail.points[0].Y) >= teamTrailSpacing {
			copy(trail.points[1:], trail.points[:len(trail.points)-1])
			trail.points[0] = at
		}
		for i, id := range members[1:] {
			follower := w.companionOwner(id)
			if follower == nil {
				continue
			}
			var walker *Walker
			if id == w.Player.ID {
				walker = &w.walker
			} else {
				walker = &w.Peers[id].Walker
			}
			walker.OnLeg = nil // Following never emits an independent AC6 request.
			goal := trail.points[i+1]
			if p.VehicleID != 0 {
				goal = at
			}
			if jump {
				walker.Stop(follower)
				follower.X, follower.Y = goal.X, goal.Y
				continue
			}
			if current, moving := walker.destination(); moving && current == goal {
				continue
			}
			if follower.X == goal.X && follower.Y == goal.Y {
				walker.Stop(follower)
				continue
			}
			walker.Start(follower, []image.Point{goal}, now)
		}
	}
}

func (w *World) resetTeamTrail(id uint32) {
	if trail := w.teamTrails[id]; trail != nil {
		trail.relocated = true
	}
}
