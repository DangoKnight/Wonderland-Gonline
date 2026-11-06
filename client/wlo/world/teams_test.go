package world

import (
	"image"
	"slices"
	"testing"
	"time"
)

func TestTeamCapturedFormationAndLeaderDisband(t *testing.T) {
	w := &World{Player: Player{ID: 4617590}, Peers: map[uint32]*Peer{4617671: {Player: Player{ID: 4617671}}}}
	// Independent bytes from the two-client Legacy capture, including no roster.
	join := []byte{13, 5, 199, 117, 70, 0, 118, 117, 70, 0}
	if !w.ApplyTeam(join) || !w.FollowingTeam() || !slices.Equal(w.TeamMembers(4617590), []uint32{4617671, 4617590}) {
		t.Fatal("formation did not establish own membership")
	}
	if !w.ApplyTeam(join) || len(w.TeamMembers(4617590)) != 2 {
		t.Fatal("duplicate join changed formation")
	}
	if w.WalkTo(100, 100, time.Now()) {
		t.Fatal("follower planned an independent walk")
	}
	if !w.ApplyTeam([]byte{13, 15, 3, 118, 117, 70, 0, 199, 117, 70, 0}) || w.FollowingTeam() {
		t.Fatal("native leader change ignored")
	}
	if !w.ApplyTeam([]byte{13, 4, 118, 117, 70, 0}) || len(w.TeamMembers(4617671)) != 0 {
		t.Fatal("leader disband retained followers")
	}
}

func TestTeamNativeTrailSpacingAndNoWalkRequests(t *testing.T) {
	w := &World{Player: Player{ID: 1}, Peers: map[uint32]*Peer{2: {Player: Player{ID: 2}}, 3: {Player: Player{ID: 3}}, 4: {Player: Player{ID: 4}}}}
	if !w.ApplyTeam([]byte{13, 6, 1, 0, 0, 0, 3, 2, 0, 0, 0, 3, 0, 0, 0, 4, 0, 0, 0}) {
		t.Fatal("roster")
	}
	now := time.Unix(1, 0)
	w.stepTeamFollowing(now)
	for x := 20; x <= 100; x += 20 {
		w.Player.X = x
		now = now.Add(time.Second)
		w.stepTeamFollowing(now)
	}
	for id, goal := range map[uint32]image.Point{2: image.Pt(80, 0), 3: image.Pt(60, 0), 4: image.Pt(40, 0)} {
		got, ok := w.Peers[id].Walker.destination()
		if !ok || got != goal {
			t.Fatalf("member %d goal %v, want %v", id, got, goal)
		}
	}
	// Rebind the local player as the first follower, retaining the remote leader.
	f := &World{Player: Player{ID: 2}, Peers: map[uint32]*Peer{1: {Player: Player{ID: 1}}}}
	sent := 0
	f.walker.OnLeg = func(int, int, int) { sent++ }
	f.SetTeamRoster([]uint32{1, 2})
	f.stepTeamFollowing(now)
	f.Peers[1].X = 20
	f.stepTeamFollowing(now.Add(time.Second))
	f.Peers[1].X = 40
	f.stepTeamFollowing(now.Add(2 * time.Second))
	if sent != 0 {
		t.Fatal("following emitted a movement request")
	}
	f.PlacePeer(1, 900, 500)
	f.stepTeamFollowing(now.Add(3 * time.Second))
	if f.Player.X != 900 || f.Player.Y != 500 {
		t.Fatal("leader relocation retained an old trail", f.Player)
	}
}

func TestTeamMalformedAndDeferredAppearances(t *testing.T) {
	w := &World{Player: Player{ID: 2}, Peers: map[uint32]*Peer{}}
	w.SetTeamRoster([]uint32{1, 2})
	for _, p := range [][]byte{{13, 5, 1}, {13, 6, 1, 0, 0, 0, 1, 1, 0, 0, 0}, {13, 5, 1, 0, 0, 0, 1, 0, 0, 0}, {13, 15, 3, 9, 0, 0, 0, 1, 0, 0, 0}} {
		if w.ApplyTeam(p) || len(w.TeamMembers(2)) != 2 {
			t.Fatal("malformed formation changed state", p)
		}
	}
	w.stepTeamFollowing(time.Now()) // Leader has not appeared yet.
	if w.FollowingTeam() {
		t.Fatal("absent leader imposed a walking lock")
	}
	w.Peers[1] = &Peer{Player: Player{ID: 1, X: 50, Y: 50}}
	w.stepTeamFollowing(time.Now())
	if w.Walking() || w.Player.X != 50 || w.Player.Y != 50 {
		t.Fatal("late leader appearance did not snap the new follower")
	}
	if !w.ApplyTeam([]byte{13, 6, 3, 0, 0, 0, 1, 4, 0, 0, 0}) || !w.FollowingTeam() {
		t.Fatal("unrelated public team replaced local party")
	}
}

func TestTeamJoiningSnapsOnlyNewFollowersThenWalks(t *testing.T) {
	w := &World{Player: Player{ID: 1, X: 200, Y: 300}, Peers: map[uint32]*Peer{2: {Player: Player{ID: 2, X: 50, Y: 60}}}}
	join := []byte{13, 5, 1, 0, 0, 0, 2, 0, 0, 0}
	w.ApplyTeam(join)
	if p := w.Peers[2]; p.X != 200 || p.Y != 300 || p.Walker.Walking() {
		t.Fatal("join walked instead of snapping", p.Player)
	}
	now := time.Unix(1, 0)
	w.Step(now)
	w.Player.X = 220
	w.Step(now.Add(100 * time.Millisecond))
	w.Player.X = 240
	w.Step(now.Add(200 * time.Millisecond))
	w.Step(now.Add(250 * time.Millisecond))
	x := w.Peers[2].X
	if x <= 200 || x >= 220 || !w.Peers[2].Walker.Walking() {
		t.Fatal("following did not resume normal walking", x)
	}
	w.ApplyTeam(join)
	if w.Peers[2].X != x {
		t.Fatal("duplicate join snapped an existing follower")
	}
	w.Peers[3] = &Peer{Player: Player{ID: 3, X: 10, Y: 20}}
	w.ApplyTeam([]byte{13, 5, 1, 0, 0, 0, 3, 0, 0, 0})
	if w.Peers[3].X != 240 || w.Peers[3].Y != 300 || w.Peers[2].X != x {
		t.Fatal("adding a member moved the existing follower")
	}
}

func TestTeamPetsHiddenForBothRolesAndRestoredOnDisband(t *testing.T) {
	w := &World{Player: Player{ID: 1, X: 200, Y: 300}, Peers: map[uint32]*Peer{2: {Player: Player{ID: 2, X: 50, Y: 60}}, 3: {Player: Player{ID: 3, X: 400, Y: 300}}}}
	w.SetCompanion(1, 12032, nil, nil)
	w.SetCompanion(2, 25020, nil, nil)
	w.SetCompanion(3, 11003, nil, nil)
	leaderPet, followerPet := w.Companions[1], w.Companions[2]
	w.SetTeamRoster([]uint32{1, 2})
	for _, id := range []uint32{1, 2} {
		if _, _, _, visible := w.companionPosition(id); visible {
			t.Fatal("party pet visible", id)
		}
	}
	if _, _, _, visible := w.companionPosition(3); !visible {
		t.Fatal("unrelated pet hidden")
	}
	w.ApplyTeam([]byte{13, 15, 3, 2, 0, 0, 0, 1, 0, 0, 0})
	for _, id := range []uint32{1, 2} {
		if _, _, _, visible := w.companionPosition(id); visible {
			t.Fatal("leader transfer exposed a pet", id)
		}
	}
	w.ApplyTeam([]byte{13, 4, 2, 0, 0, 0})
	for _, id := range []uint32{1, 2} {
		x, y, _, visible := w.companionPosition(id)
		owner := w.companionOwner(id)
		if !visible || x != owner.X || y != owner.Y {
			t.Fatal("pet did not reappear beside its owner", id, x, y)
		}
	}
	if w.Companions[1] != leaderPet || w.Companions[2] != followerPet {
		t.Fatal("hiding replaced active companion state")
	}
}

func TestTeamPetFormationDoesNotCreatePartyAndBattlePausesFollowing(t *testing.T) {
	w := &World{Player: Player{ID: 1}, Peers: map[uint32]*Peer{2: {Player: Player{ID: 2}}}}
	w.SetCompanion(1, 12032, nil, nil)
	if !w.ApplyTeam([]byte{13, 5, 1, 0, 0, 0, 0, 47, 0, 0}) || len(w.TeamMembers(1)) != 0 {
		t.Fatal("pet treated as a teammate")
	}
	w.SetTeamRoster([]uint32{1, 2})
	now := time.Unix(1, 0)
	w.Step(now)
	w.Player.X = 100
	w.Step(now.Add(time.Second))
	w.Player.X = 120
	w.Step(now.Add(1100 * time.Millisecond))
	if !w.Peers[2].Walker.Walking() {
		t.Fatal("fixture did not follow")
	}
	w.TeamFollowingPaused = true
	w.Step(now.Add(2 * time.Second))
	if w.Peers[2].Walker.Walking() {
		t.Fatal("following restarted during battle")
	}
	// A battle freezes only the local formation, not unrelated public teams.
	w.Peers[3] = &Peer{Player: Player{ID: 3}}
	w.Peers[4] = &Peer{Player: Player{ID: 4}}
	w.SetTeamRoster([]uint32{3, 4})
	w.Step(now.Add(3 * time.Second))
	w.Peers[3].X = 40
	w.Step(now.Add(4 * time.Second))
	w.Peers[3].X = 80
	w.Step(now.Add(5 * time.Second))
	if !w.Peers[4].Walker.Walking() {
		t.Fatal("local battle paused an unrelated team")
	}
}
