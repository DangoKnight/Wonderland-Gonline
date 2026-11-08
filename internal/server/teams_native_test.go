package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

func nativeParty(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	s, players, wires := partyFixture(t)
	ctx := context.Background()
	if err := s.partyCommand(ctx, players[1], protocol.Builder{13, 1}.U32(players[0].character.ID)); err != nil {
		t.Fatal(err)
	}
	if err := s.partyCommand(ctx, players[0], protocol.Builder{13, 3, 1}.U32(players[1].character.ID)); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	return s, players, wires
}

func TestPartyNativeTransferRequiresCandidateAcceptance(t *testing.T) {
	s, p, w := nativeParty(t)
	ctx := context.Background()
	leader, candidate, outsider := p[0], p[1], p[2]
	if err := s.partyCommand(ctx, leader, protocol.Builder{13, 10, 1}.U32(candidate.character.ID)); err != nil {
		t.Fatal(err)
	}
	if leader.party.leader() != leader || !hasPacket(w[1].packets(t), protocol.Builder{13, 15, 1}.U32(leader.character.ID)) {
		t.Fatal("offer did not await candidate")
	}
	if err := s.partyCommand(ctx, outsider, []byte{13, 10, 3}); err != nil {
		t.Fatal(err)
	}
	if leader.party.leader() != leader {
		t.Fatal("outsider accepted transfer")
	}
	if err := s.partyCommand(ctx, candidate, []byte{13, 10, 3}); err != nil {
		t.Fatal(err)
	}
	if leader.party.leader() != candidate || leader.party.transfer != nil {
		t.Fatal("acceptance did not commit transfer")
	}
	for _, wire := range w[:2] {
		if !hasPacket(wire.packets(t), protocol.Builder{13, 15, 3}.U32(candidate.character.ID).U32(leader.character.ID)) {
			t.Fatal("missing native transfer completion")
		}
	}
	// A replay cannot transfer leadership back.
	s.partyCommand(ctx, candidate, []byte{13, 10, 3})
	if leader.party.leader() != candidate {
		t.Fatal("replayed acceptance changed leader")
	}
}

func TestPartyNativeTransferRefusalExpiryAndDeparture(t *testing.T) {
	for _, kind := range []string{"refuse", "client timeout", "server timeout", "late confirmation", "disconnect", "warp", "battle"} {
		t.Run(kind, func(t *testing.T) {
			s, p, w := nativeParty(t)
			ctx := context.Background()
			s.partyCommand(ctx, p[0], protocol.Builder{13, 10, 1}.U32(p[1].character.ID))
			party := p[0].party
			switch kind {
			case "refuse":
				s.partyCommand(ctx, p[1], []byte{13, 10, 2, 2})
			case "client timeout":
				s.partyCommand(ctx, p[1], []byte{13, 10, 2, 3})
			case "server timeout":
				s.expirePartyLeadership(time.Now().Add(6 * time.Second))
			case "late confirmation":
				party.transfer.deadline = time.Now().Add(-time.Second)
				if err := s.partyCommand(ctx, p[1], []byte{13, 10, 3}); err != nil {
					t.Fatal(err)
				}
			case "disconnect":
				s.partyLeave(p[1], false)
			case "warp":
				if err := s.commandTeleport(ctx, p[1], world.Destination{Map: 20000, X: 500, Y: 600}); err != nil {
					t.Fatal(err)
				}
			case "battle":
				run := &battleRun{}
				if err := s.startBattle(p[0], run, []battle.Enemy{{Template: 10500, Level: 5, HP: 1}}); err != nil {
					t.Fatal(err)
				}
				if run.timer != nil {
					defer run.timer.Stop()
				}
			}
			if party.transfer != nil {
				t.Fatal("pending transfer survived", kind)
			}
			if len(party.members) > 0 && party.leader() != p[0] {
				t.Fatal("cancel changed leadership")
			}
			leaderPackets := w[0].packets(t)
			if countPrefix(leaderPackets, 13, 15, 2) == 0 {
				t.Fatal("leader was not notified")
			}
			if kind == "client timeout" || kind == "late confirmation" || kind == "server timeout" {
				if !hasPacket(leaderPackets, []byte{13, 15, 2, 3}) || !hasPacket(w[1].packets(t), []byte{13, 15, 2, 3}) {
					t.Fatal("timeout did not notify both participants")
				}
			}
		})
	}
}

func TestPartyFollowingCoordinatesBufferedAndIndependentWalkRejected(t *testing.T) {
	s, p, _ := nativeParty(t)
	ctx := context.Background()
	oldX, oldY := p[1].character.X, p[1].character.Y
	// The leader owns group movement; a follower's independent request is ignored.
	if err := s.worldCommand(ctx, p[1], []byte{6, 1, 0, 244, 1, 88, 2}); err != nil {
		t.Fatal(err)
	}
	if p[1].character.X != oldX || p[1].character.Y != oldY {
		t.Fatal("follower walked independently")
	}
	p[0].character.X, p[0].character.Y = 500, 600
	s.movePartyFollowers(p[0])
	if p[1].character.X != 500 || p[1].character.Y != 600 || p[1].autosaveBaseline == nil {
		t.Fatal("following not buffered for checkpoint")
	}
	if err := s.autosaveSession(ctx, p[1]); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Store.Characters(ctx, p[1].account.ID)
	if err != nil || len(saved) == 0 || saved[0].X != 500 || saved[0].Y != 600 {
		t.Fatal("follower checkpoint did not persist", err, saved)
	}
}

func TestPartyNativeLeaveScopeAndObserverSnapshot(t *testing.T) {
	s, p, w := nativeParty(t)
	ctx := context.Background()
	s.partySceneSnapshot(p[2], []*Session{p[0], p[1]})
	if got := w[2].packets(t); countPrefix(got, 13, 6) != 1 {
		t.Fatal("observer did not receive a single public roster", got)
	}
	if p[2].party != nil {
		t.Fatal("observer adopted party membership")
	}
	wrong := p[0].character.ID + 1
	if err := s.partyCommand(ctx, p[1], protocol.Builder{13, 4}.U32(wrong)); err != nil {
		t.Fatal(err)
	}
	if p[1].party == nil {
		t.Fatal("stale leader leave changed membership")
	}
	if err := s.partyCommand(ctx, p[1], protocol.Builder{13, 4}.U32(p[0].character.ID)); err != nil {
		t.Fatal(err)
	}
	if p[1].party != nil || p[0].party != nil {
		t.Fatal("native leave did not dissolve two-person team")
	}
}

func TestPartyArrivalUsesSilentRosterInEitherLoadOrder(t *testing.T) {
	for _, followerFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "leader first", true: "follower first"}[followerFirst], func(t *testing.T) {
			s, p, w := nativeParty(t)
			ctx := context.Background()
			if err := s.commandTeleport(ctx, p[0], world.Destination{Map: 20000, X: 500, Y: 600}); err != nil {
				t.Fatal(err)
			}
			// An observer is already present on the destination map.
			p[2].character.Map = 20000
			p[2].ready = true
			for _, wire := range w {
				wire.Reset()
			}
			order := []*Session{p[0], p[1]}
			if followerFirst {
				order[0], order[1] = order[1], order[0]
			}
			for _, member := range order {
				if err := s.worldCommand(ctx, member, []byte{12, 1}); err != nil {
					t.Fatal(err)
				}
			}
			roster := protocol.Builder{13, 6}.U32(p[0].character.ID).U8(1).U32(p[1].character.ID)
			for i, wire := range w {
				packets := wire.packets(t)
				if countPrefix(packets, 13, 5) != 0 || countPrefix(packets, 13, 3) != 0 {
					t.Fatal("map arrival replayed a team join", i, packets)
				}
				if !hasPacket(packets, roster) {
					t.Fatal("map arrival lost the team roster", i)
				}
			}
			if p[0].party == nil || p[0].party != p[1].party || p[0].party.leader() != p[0] || p[2].party != nil {
				t.Fatal("map arrival changed team membership")
			}
		})
	}
}

func TestPartyNativeConcurrentOfferReplyAndExpiry(t *testing.T) {
	s, p, _ := nativeParty(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			s.partyCommand(ctx, p[i%2], protocol.Builder{13, 10, 1}.U32(p[(i+1)%2].character.ID))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			s.partyCommand(ctx, p[i%2], []byte{13, 10, 3})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			s.expirePartyLeadership(time.Now().Add(6 * time.Second))
		}
	}()
	wg.Wait()
	if p[0].party != p[1].party || len(p[0].party.members) != 2 || p[0].party.members[0] == p[0].party.members[1] {
		t.Fatal("concurrent transfer corrupted membership")
	}
}

func TestPartyRequestToFollowerAndTransferFormationOrder(t *testing.T) {
	s, p, w := nativeParty(t)
	ctx := context.Background()
	if err := s.partyCommand(ctx, p[2], protocol.Builder{13, 1}.U32(p[1].character.ID)); err != nil {
		t.Fatal(err)
	}
	if countPrefix(w[0].packets(t), 13, 1) != 1 || countPrefix(w[1].packets(t), 13, 1) != 0 {
		t.Fatal("request to follower did not reach leader")
	}
	if err := s.partyCommand(ctx, p[0], protocol.Builder{13, 3, 1}.U32(p[2].character.ID)); err != nil {
		t.Fatal(err)
	}
	if err := s.partyCommand(ctx, p[0], protocol.Builder{13, 10, 1}.U32(p[2].character.ID)); err != nil {
		t.Fatal(err)
	}
	if err := s.partyCommand(ctx, p[2], []byte{13, 10, 3}); err != nil {
		t.Fatal(err)
	}
	members := p[0].party.members
	if len(members) != 3 || members[0] != p[2] || members[1] != p[1] || members[2] != p[0] {
		t.Fatal("native transfer did not swap formation slots")
	}
}

func TestTeamBattleCellsFollowTransferredLeaderEvenWhenFollowerStarts(t *testing.T) {
	s, p, _ := nativeParty(t)
	ctx := context.Background()
	s.partyCommand(ctx, p[0], protocol.Builder{13, 10, 1}.U32(p[1].character.ID))
	s.partyCommand(ctx, p[1], []byte{13, 10, 3})
	run := &battleRun{}
	if err := s.startBattle(p[0], run, []battle.Enemy{{Template: 10500, Level: 5, HP: 1}}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if run.timer != nil {
			run.timer.Stop()
		}
	}()
	leader, follower := run.member(p[1]).self, run.member(p[0]).self
	if leader.X != 4 || leader.Y != 2 || follower.X != 4 || follower.Y != 3 {
		t.Fatal("formation did not follow transferred leadership", leader.X, leader.Y, follower.X, follower.Y)
	}
	if run.members[0].c != p[0] {
		t.Fatal("formation correction reassigned quest/loot ownership")
	}
}

func TestPartyNativeTransferRejectsMalformedPackets(t *testing.T) {
	s, p, _ := nativeParty(t)
	for _, packet := range [][]byte{{13, 10}, {13, 10, 1}, {13, 10, 1, 1, 0, 0}, {13, 10, 2}, {13, 10, 2, 1}, {13, 10, 3, 0}, {13, 1, 1}, {13, 3, 1, 1}, {13, 4, 1}, {13, 4, 1, 0, 0}} {
		if err := s.partyCommand(context.Background(), p[0], packet); err != protocol.ErrMalformed {
			t.Fatal(packet, err)
		}
	}
}
