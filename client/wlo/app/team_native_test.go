package app

import (
	"bytes"
	"testing"
	"time"

	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

func acceptTeamPrompt(t *testing.T, c *Client) {
	t.Helper()
	if c.UI.Modal == nil {
		t.Fatal("missing confirmation")
	}
	for _, child := range c.UI.Modal.Base().Children {
		if button, ok := child.(*seui.FixedButton); ok && button.Image == c.Env.Pics.Find("btn_ok_1") {
			button.Click()
			return
		}
	}
	t.Fatal("missing confirm button")
}

func TestTeamNativeLeaveAndMapMembership(t *testing.T) {
	c := nativeTeamClient(t)
	c.dispatch([]byte{13, 5, 199, 117, 70, 0, 118, 117, 70, 0})
	sent := wire(t, c)
	c.Team.Show()
	c.Team.End.Click()
	acceptTeamPrompt(t, c)
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{13, 4, 199, 117, 70, 0}) {
		t.Fatal("native leave request", got)
	}
	// A map change replaces actor objects, while membership survives until its
	// authoritative roster is replayed; the leader may not have appeared yet.
	c.dispatch(protocol.Builder{12}.U32(4617590).U16(10027).U16(674).U16(1067).U16(1).U8(0))
	if !c.inTeam() || len(c.World.TeamMembers(c.World.Player.ID)) != 2 {
		t.Fatal("map replacement lost membership")
	}
	c.dispatch([]byte{13, 4, 199, 117, 70, 0})
	if c.inTeam() || c.World.FollowingTeam() {
		t.Fatal("disband after map transition failed")
	}
}

func TestJoinTeamToolbarTargetsExistingTeamLeader(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.World.Peers = map[uint32]*world.Peer{20002: {Player: world.Player{ID: 20002, X: c.World.Player.X + 50, Y: c.World.Player.Y}}, 30000: {Player: world.Player{ID: 30000, X: c.World.Player.X + 100, Y: c.World.Player.Y}}}
	c.World.SetTeamRoster([]uint32{30000, 20002})
	sent := wire(t, c)
	c.FuncButtons.Buttons[joinTeamButton].Click()
	cx, cy := c.World.Camera()
	c.GroundClick(c.World.Peers[20002].X-cx, c.World.Peers[20002].Y-cy-20)
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{13, 1, 48, 117, 0, 0}) {
		t.Fatal("toolbar selected follower instead of leader", got)
	}
}

func nativeTeamClient(t *testing.T) *Client {
	t.Helper()
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.World.Player.ID = 4617590
	c.TeamState.Reset(4617590)
	c.World.Peers = map[uint32]*world.Peer{4617671: {Player: world.Player{ID: 4617671, Name: []byte("Test0001"), X: c.World.Player.X, Y: c.World.Player.Y}}}
	c.rememberPlayer(4617671, []byte("Test0001"))
	return c
}

func TestTeamCapturedIDOnlyInvitationAndFormation(t *testing.T) {
	c := nativeTeamClient(t)
	c.TeamState.BattlePet = 12032
	sent := wire(t, c)
	c.dispatch([]byte{13, 1, 199, 117, 70, 0})
	acceptTeamPrompt(t, c)
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{13, 3, 1, 199, 117, 70, 0}) {
		t.Fatal("native acceptance", got)
	}
	c.dispatch([]byte{13, 3, 1, 199, 117, 70, 0})
	c.dispatch([]byte{13, 5, 199, 117, 70, 0, 118, 117, 70, 0})
	if !c.inTeam() || c.TeamState.Leader != 4617671 || !c.World.FollowingTeam() {
		t.Fatal("join without full roster failed")
	}
	c.sendLeg(1, 100, 100)
	if got := sent(); len(got) != 1 {
		t.Fatal("following added movement", got)
	}
	c.dispatch([]byte{13, 4, 199, 117, 70, 0})
	if c.inTeam() || c.World.FollowingTeam() {
		t.Fatal("departing leader retained local membership")
	}
	if c.TeamState.BattlePet != 12032 {
		t.Fatal("disband changed the active pet")
	}
}

func TestTeamNativeLeadershipAcceptanceAndTimeout(t *testing.T) {
	c := nativeTeamClient(t)
	c.dispatch([]byte{13, 5, 199, 117, 70, 0, 118, 117, 70, 0})
	sent := wire(t, c)
	now := c.Now()
	c.Now = func() time.Time { return now }
	c.dispatch([]byte{13, 15, 1, 199, 117, 70, 0})
	acceptTeamPrompt(t, c)
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{13, 10, 2, 3}) {
		t.Fatal("native transfer acceptance", got)
	}
	if c.TeamState.Leader != 4617671 {
		t.Fatal("optimistic transfer")
	}
	c.dispatch([]byte{13, 15, 3, 118, 117, 70, 0, 199, 117, 70, 0})
	if c.TeamState.Leader != 4617590 || c.World.FollowingTeam() {
		t.Fatal("transfer completion ignored")
	}
	// Restore the original formation and allow the native five-second prompt to expire.
	c.dispatch([]byte{13, 6, 199, 117, 70, 0, 1, 118, 117, 70, 0})
	c.dispatch([]byte{13, 15, 1, 199, 117, 70, 0})
	now = now.Add(5 * time.Second)
	c.teamTransferTick()
	got := sent()
	if len(got) != 2 || !bytes.Equal(got[1], []byte{13, 10, 3}) || c.UI.Modal != nil {
		t.Fatal("transfer timeout", got)
	}
	// Delayed UI actions cannot send a second answer after cancellation.
	c.teamTransferTick()
	if len(sent()) != 2 {
		t.Fatal("timeout sent twice")
	}
}

func TestTeamNativeLeadershipDeclineAndPendingDisband(t *testing.T) {
	c := nativeTeamClient(t)
	c.dispatch([]byte{13, 5, 199, 117, 70, 0, 118, 117, 70, 0})
	sent := wire(t, c)
	c.dispatch([]byte{13, 15, 1, 199, 117, 70, 0})
	if c.UI.Modal == nil {
		t.Fatal("missing leadership offer")
	}
	c.UI.Modal.KeyDown(escapeKey, 0)
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{13, 10, 2, 2}) {
		t.Fatal("native refusal", got)
	}
	c.dispatch([]byte{13, 15, 1, 199, 117, 70, 0})
	c.dispatch([]byte{13, 4, 199, 117, 70, 0})
	if c.UI.Modal != nil || c.teamTransferLeader != 0 {
		t.Fatal("disband retained an obsolete leadership prompt")
	}
	c.teamTransferTick()
	if len(sent()) != 1 {
		t.Fatal("obsolete offer sent a later response")
	}
}

func TestTeamHiddenPetRemainsEnabledInBattle(t *testing.T) {
	c := nativeTeamClient(t)
	c.InventoryState.Pets[0].ID = 12032
	c.dispatch([]byte{19, 1, 0, 47, 0, 0})
	c.dispatch([]byte{13, 5, 199, 117, 70, 0, 118, 117, 70, 0})
	if c.TeamState.BattlePet != 12032 || c.World.Companions[4617590] == nil {
		t.Fatal("joining rested or removed the pet")
	}
	// Native 32-byte player and pet records, independent of the server builder.
	c.dispatch([]byte{11, 250, 82, 25,
		1, 2, 118, 117, 70, 0, 0, 0, 0, 0, 0, 0, 4, 2, 100, 0, 0, 0, 50, 0, 100, 0, 0, 0, 50, 0, 1, 1, 0, 0, 0, 0,
		1, 4, 0, 47, 0, 0, 0, 0, 118, 117, 70, 0, 3, 2, 100, 0, 0, 0, 50, 0, 100, 0, 0, 0, 50, 0, 1, 1, 0, 0, 0, 0})
	pet := c.battle.state.At(clientbattle.Cell{X: 3, Y: 2})
	if !c.battle.state.Active || pet == nil || !c.battle.state.Owned(pet) || pet.HP != 100 {
		t.Fatal("hidden pet did not enter battle", pet)
	}
	c.dispatch([]byte{13, 4, 199, 117, 70, 0})
	if c.TeamState.BattlePet != 12032 || c.InventoryState.Pets[0].ID != 12032 || c.World.Companions[4617590] == nil {
		t.Fatal("disband changed combat pet selection")
	}
}
