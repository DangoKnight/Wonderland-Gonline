package server

import (
	"bytes"
	"context"
	"testing"
	"time"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

func hasPacket(packets [][]byte, want []byte) bool {
	for _, p := range packets {
		if bytes.Equal(p, want) {
			return true
		}
	}
	return false
}

func countPrefix(packets [][]byte, prefix ...byte) int {
	n := 0
	for _, p := range packets {
		if bytes.HasPrefix(p, prefix) {
			n++
		}
	}
	return n
}

func partyFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	s, players, wires := worldFixture(t)
	players[2].character.Map = players[0].character.Map
	s.Assets.Maps[10017] = assets.Map{ID: 10017, Warps: []assets.Warp{{ClickID: 1, MapID: 20000, X: 500, Y: 600}}}
	s.Assets.Maps[20000] = assets.Map{ID: 20000}
	s.World = world.New(s.Assets)
	for i := range players {
		if err := s.worldCommand(context.Background(), players[i], []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
	}
	for _, w := range wires {
		w.Reset()
	}
	return s, players, wires
}

func TestPartyJoinRosterAndVitals(t *testing.T) {
	s, players, wires := partyFixture(t)
	ctx := context.Background()
	alice, bobby, carol := players[0], players[1], players[2]
	a, b := alice.character.ID, bobby.character.ID

	// An accept without a request does nothing.
	if err := s.partyCommand(ctx, alice, protocol.Builder{13, 3, 1}.U32(b)); err != nil {
		t.Fatal(err)
	}
	if alice.party != nil || wires[1].Len() != 0 {
		t.Fatal("unsolicited accept joined a team")
	}
	if err := s.partyCommand(ctx, bobby, protocol.Builder{13, 1}.U32(a)); err != nil {
		t.Fatal(err)
	}
	want, _ := protocol.Builder{13, 1}.U32(b).String("Bobby")
	if got := wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("request: %v", got)
	}
	// A declined request is forgotten.
	if err := s.partyCommand(ctx, alice, protocol.Builder{13, 3, 2}.U32(b)); err != nil {
		t.Fatal(err)
	}
	if err := s.partyCommand(ctx, alice, protocol.Builder{13, 3, 1}.U32(b)); err != nil {
		t.Fatal(err)
	}
	if alice.party != nil {
		t.Fatal("declined request accepted later")
	}
	if err := s.partyCommand(ctx, bobby, protocol.Builder{13, 1}.U32(a)); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.partyCommand(ctx, alice, protocol.Builder{13, 3, 1}.U32(b)); err != nil {
		t.Fatal(err)
	}
	if alice.party == nil || alice.party != bobby.party || alice.party.leader() != alice {
		t.Fatal("team not formed")
	}
	follow := protocol.Builder{13, 5}.U32(a).U32(b)
	roster := protocol.Builder{13, 6}.U32(a).U8(1).U32(b)
	for i, w := range wires[:2] {
		got := w.packets(t)
		if !hasPacket(got, follow) || !hasPacket(got, roster) || countPrefix(got, 8, 3) != 8 || countPrefix(got, 8, 1) == 0 {
			t.Fatalf("member %d: %v", i, got)
		}
	}
	if got := wires[2].packets(t); len(got) != 1 || !bytes.Equal(got[0], follow) {
		t.Fatalf("map follow: %v", got)
	}
	if alice.view.Team.Load() != 1 || bobby.view.Team.Load() != 1 {
		t.Fatal("team size not tracked")
	}

	// Vital changes reach the teammate once.
	bobby.character.HP--
	s.partySync(bobby)
	s.partySync(bobby)
	hp := protocol.Builder{8, 3}.U32(b).U8(0x19).U8(1).U32(uint32(bobby.character.HP)).U32(0)
	if got := wires[0].packets(t); len(got) != 8 || !hasPacket(got, hp) {
		t.Fatalf("vitals: %v", got)
	}

	// Only the leader transfers or kicks.
	if err := s.partyCommand(ctx, bobby, protocol.Builder{13, 10}.U32(a)); err != nil {
		t.Fatal(err)
	}
	if alice.party.leader() != alice {
		t.Fatal("member transferred leadership")
	}
	if err := s.partyCommand(ctx, alice, protocol.Builder{13, 10}.U32(b)); err != nil {
		t.Fatal(err)
	}
	if alice.party.leader() != bobby || !hasPacket(wires[0].packets(t), protocol.Builder{13, 6}.U32(b).U8(1).U32(a)) {
		t.Fatal("leadership not transferred")
	}
	wires[1].Reset()
	wires[2].Reset()
	if err := s.partyCommand(ctx, bobby, protocol.Builder{13, 9}.U32(a)); err != nil {
		t.Fatal(err)
	}
	if alice.party != nil || bobby.party != nil || alice.view.Team.Load() != 0 {
		t.Fatal("kick did not dissolve the team")
	}
	if got := wires[0].packets(t); !hasPacket(got, protocol.Builder{13, 6}.U32(a).U8(0)) || !hasPacket(got, protocol.Builder{13, 4}.U32(a)) {
		t.Fatalf("kicked member: %v", got)
	}
	if got := wires[1].packets(t); !hasPacket(got, protocol.Builder{13, 6}.U32(b).U8(0)) {
		t.Fatalf("remaining member: %v", got)
	}
	if got := wires[2].packets(t); len(got) != 1 || !bytes.Equal(got[0], protocol.Builder{13, 4}.U32(a)) {
		t.Fatalf("map leave: %v", got)
	}
	_ = carol
}

func TestPartyLimitFollowAndLogout(t *testing.T) {
	s, players, wires := partyFixture(t)
	ctx := context.Background()
	alice, bobby, carol := players[0], players[1], players[2]
	join := func(leader, member *Session) {
		t.Helper()
		if err := s.partyCommand(ctx, member, protocol.Builder{13, 1}.U32(leader.character.ID)); err != nil {
			t.Fatal(err)
		}
		if err := s.partyCommand(ctx, leader, protocol.Builder{13, 3, 1}.U32(member.character.ID)); err != nil {
			t.Fatal(err)
		}
	}
	join(alice, bobby)
	join(alice, carol)
	if len(alice.party.members) != 3 {
		t.Fatal("team size", len(alice.party.members))
	}
	// A member cannot accept on the team's behalf, and a member cannot ask to join elsewhere.
	join(bobby, carol)
	if len(alice.party.members) != 3 || carol.party != alice.party {
		t.Fatal("member accepted a request")
	}

	// The leader's portal brings members on its map.
	for _, w := range wires {
		w.Reset()
	}
	if err := s.worldCommand(ctx, alice, []byte{20, 8, 1, 0}); err != nil {
		t.Fatal(err)
	}
	for i, p := range players {
		if p.character.Map != 20000 {
			t.Fatalf("player %d stayed on map %d", i, p.character.Map)
		}
	}
	for _, p := range players {
		if err := s.worldCommand(ctx, p, []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
	}
	follow := protocol.Builder{13, 5}.U32(alice.character.ID).U32(bobby.character.ID)
	if !hasPacket(wires[1].packets(t), follow) {
		t.Fatal("formation not replayed on arrival")
	}

	// A disconnected member leaves the team.
	for _, w := range wires {
		w.Reset()
	}
	s.leaveWorld(carol)
	if carol.party != nil || len(alice.party.members) != 2 {
		t.Fatal("logout kept the member")
	}
	roster := protocol.Builder{13, 6}.U32(alice.character.ID).U8(1).U32(bobby.character.ID)
	if got := wires[0].packets(t); !hasPacket(got, protocol.Builder{13, 4}.U32(carol.character.ID)) || !hasPacket(got, roster) {
		t.Fatalf("logout: %v", got)
	}
	if wires[2].Len() != 0 {
		t.Fatal("packets sent to the departed player")
	}
}

func teamBattle(t *testing.T, hp int) (*Server, []*Session, []*captureConn) {
	t.Helper()
	battleSleep, turnTimeout = func(time.Duration) {}, time.Hour
	t.Cleanup(func() { battleSleep, turnTimeout = time.Sleep, 30*time.Second })
	s, players, wires := partyFixture(t)
	ctx := context.Background()
	alice, bobby := players[0], players[1]
	if err := s.partyCommand(ctx, bobby, protocol.Builder{13, 1}.U32(alice.character.ID)); err != nil {
		t.Fatal(err)
	}
	if err := s.partyCommand(ctx, alice, protocol.Builder{13, 3, 1}.U32(bobby.character.ID)); err != nil {
		t.Fatal(err)
	}
	s.Assets.NPCs = map[uint16]assets.NPC{10500: {ID: 10500, Name: "Slime"}}
	for _, w := range wires {
		w.Reset()
	}
	s.worldMu.Lock()
	err := s.startBattle(alice, &battleRun{}, []battle.Enemy{{Template: 10500, Name: "Slime", Level: 5, HP: hp}})
	s.worldMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	return s, players, wires
}

func TestTeamBattleVictory(t *testing.T) {
	s, players, wires := teamBattle(t, 1)
	ctx := context.Background()
	alice, bobby, carol := players[0], players[1], players[2]
	if alice.battle == nil || alice.battle != bobby.battle || carol.battle != nil {
		t.Fatal("team not in one battle")
	}
	// Each sees the other player's fighter record.
	b := wires[1].packets(t)
	if !contains(b, []byte{20, 12}) || countPrefix(b, 11, 5) < 2 {
		t.Fatalf("teammate intro: %v", b)
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, alice, []byte{50, 1, 4, 2, 2, 2}); err != nil {
		t.Fatal(err)
	}
	if alice.battle.b.Processing {
		t.Fatal("round ran before the teammate's command")
	}
	if err := s.worldCommand(ctx, bobby, []byte{50, 1, 4, 3, 2, 2}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, alice)
	settle(t, s, bobby)
	if alice.battle != nil || bobby.battle != nil {
		t.Fatal("battle not finished")
	}
	for i, p := range players[:2] {
		got := wires[i].packets(t)
		if !contains(got, []byte{53, 3, 2, 2}) || !contains(got, protocol.Builder{11, 0}.U32(alice.character.ID).U16(0)) || !contains(got, protocol.Builder{11, 0}.U32(bobby.character.ID).U16(0)) {
			t.Fatalf("player %d ending: %v", i, got)
		}
		chars, err := s.Store.Characters(ctx, p.account.ID)
		if err != nil || chars[0].EXP != 81 || chars[0].Gold != 40 {
			t.Fatal("rewards", i, chars[0].EXP, chars[0].Gold, err)
		}
	}
}

func TestTeamBattleFleeAndDisconnect(t *testing.T) {
	s, players, _ := teamBattle(t, 50000)
	ctx := context.Background()
	alice, bobby := players[0], players[1]
	// A disconnected member's fighter defends; the turn waits only for Alice.
	s.leaveWorld(bobby)
	if bobby.battle != nil || alice.battle == nil {
		t.Fatal("disconnect")
	}
	if err := s.worldCommand(ctx, alice, []byte{50, 4, 4, 2, 0, 0}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, alice)
	if alice.battle == nil || alice.battle.b.Turn != 1 {
		t.Fatal("round did not run without the absent member")
	}

	s2, team, _ := teamBattle(t, 50000)
	if err := s2.worldCommand(ctx, team[1], []byte{11, 1, 3}); err != nil {
		t.Fatal(err)
	}
	settle(t, s2, team[0])
	settle(t, s2, team[1])
	if team[0].battle != nil || team[1].battle != nil {
		t.Fatal("a member's escape did not end the battle for the team")
	}
}

func TestPartyLeaveNativeReset(t *testing.T) {
	for _, operation := range []string{"leader leave", "follower leave", "kick", "leader disconnect", "follower disconnect", "hidden scene", "different maps"} {
		t.Run(operation, func(t *testing.T) {
			s, players, wires := partyFixture(t)
			leader, follower := players[0], players[1]
			ctx := context.Background()
			if err := s.partyCommand(ctx, follower, protocol.Builder{13, 1}.U32(leader.character.ID)); err != nil {
				t.Fatal(err)
			}
			if err := s.partyCommand(ctx, leader, protocol.Builder{13, 3, 1}.U32(follower.character.ID)); err != nil {
				t.Fatal(err)
			}
			if operation == "hidden scene" {
				m := s.Assets.Maps[leader.character.Map]
				m.Scene = 10001
				s.Assets.Maps[m.ID] = m
				s.World = world.New(s.Assets)
			}
			if operation == "different maps" {
				follower.character.Map = 20000
			}
			for _, wire := range wires {
				wire.Reset()
			}
			var err error
			disconnected := -1
			switch operation {
			case "leader leave", "hidden scene", "different maps":
				err = s.partyCommand(ctx, leader, []byte{13, 4})
			case "follower leave":
				err = s.partyCommand(ctx, follower, []byte{13, 4})
			case "kick":
				err = s.partyCommand(ctx, leader, protocol.Builder{13, 9}.U32(follower.character.ID))
			case "leader disconnect":
				disconnected = 0
				s.leaveWorld(leader)
			case "follower disconnect":
				disconnected = 1
				s.leaveWorld(follower)
			}
			if err != nil {
				t.Fatal(err)
			}
			for i, member := range players[:2] {
				if member.party != nil || member.view.Team.Load() != 0 {
					t.Fatalf("member %d still in party", i)
				}
				packets := wires[i].packets(t)
				if i == disconnected {
					if len(packets) != 0 {
						t.Fatalf("sent to disconnected member: %v", packets)
					}
					continue
				}
				var teamPackets [][]byte
				for _, packet := range packets {
					if len(packet) >= 2 && packet[0] == 13 {
						teamPackets = append(teamPackets, packet)
					}
				}
				packets = teamPackets
				// Independent native wire fixture: empty AC13:6 sets role=leader,
				// and must be followed by AC13:4(self) to clear role and follow state.
				id := member.character.ID
				reset := []byte{13, 6, byte(id), byte(id >> 8), byte(id >> 16), byte(id >> 24), 0}
				leave := []byte{13, 4, byte(id), byte(id >> 8), byte(id >> 16), byte(id >> 24)}
				if len(packets) < 2 || !bytes.Equal(packets[len(packets)-2], reset) || !bytes.Equal(packets[len(packets)-1], leave) {
					t.Fatalf("member %d native reset order: %v", i, packets)
				}
			}
		})
	}
}

func TestPartyLeaderLeaveKeepsRemainingMembers(t *testing.T) {
	s, players, wires := partyFixture(t)
	ctx := context.Background()
	leader := players[0]
	for _, member := range players[1:] {
		if err := s.partyCommand(ctx, member, protocol.Builder{13, 1}.U32(leader.character.ID)); err != nil {
			t.Fatal(err)
		}
		if err := s.partyCommand(ctx, leader, protocol.Builder{13, 3, 1}.U32(member.character.ID)); err != nil {
			t.Fatal(err)
		}
	}
	for _, wire := range wires {
		wire.Reset()
	}
	if err := s.partyCommand(ctx, leader, []byte{13, 4}); err != nil {
		t.Fatal(err)
	}
	if leader.party != nil || players[1].party == nil || players[1].party != players[2].party || players[1].party.leader() != players[1] {
		t.Fatal("leader leave did not preserve remaining party")
	}
	roster := protocol.Builder{13, 6}.U32(players[1].character.ID).U8(1).U32(players[2].character.ID)
	for _, wire := range wires[1:] {
		packets := wire.packets(t)
		if len(packets) < 2 || !bytes.Equal(packets[0], protocol.Builder{13, 4}.U32(leader.character.ID)) || !bytes.Equal(packets[1], roster) {
			t.Fatalf("old formation must detach before new roster: %v", packets)
		}
	}
}
