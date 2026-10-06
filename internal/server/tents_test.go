package server

import (
	"context"
	"errors"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func tentFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	s, players, wires := worldFixture(t)
	s.Assets.Tents = assets.TentRules{SpawnX: 460, SpawnY: 700, Floor: 39062, Wallpaper: 39064, Furniture: []assets.TentFurnitureDefault{{ItemID: 38027, X: 43, Y: 42}}}
	s.Assets.Items[36002] = game.ItemDefinition{ID: 36002, Type: 27, Name: "Tent"}
	s.Assets.Items[38027] = game.ItemDefinition{ID: 38027, Type: 29, Name: "Workbench"}
	for _, c := range players {
		next := c.character.Clone()
		next.Map = 12000
		next.Bag = game.Inventory{{ID: 36002, Count: 1}, {ID: 38027, Count: 2, Damage: 3, Metadata: [26]byte{9}}}
		if err := s.commit(context.Background(), c, next); err != nil {
			t.Fatal(err)
		}
		tradeDo(t, s, c, []byte{12, 1})
	}
	for _, w := range wires {
		w.Reset()
	}
	return s, players, wires
}
func enterTestTent(t *testing.T, s *Server, c *Session, owner uint32) {
	t.Helper()
	tradeDo(t, s, c, protocol.Builder{65, 1}.U32(owner))
	if c.tentOwner != owner || c.character.Map != 63507 {
		t.Fatal("tent entry failed")
	}
	tradeDo(t, s, c, []byte{12, 1})
}
func TestTentNativeOpenWarpPrivateScenesAndClose(t *testing.T) {
	s, players, wires := tentFixture(t)
	a, b, guest := players[0], players[1], players[2]
	for _, c := range []*Session{a, b} {
		tradeDo(t, s, c, []byte{23, 96, 1})
	}
	sign := protocol.Builder{65, 1}.U32(a.character.ID).U16(36002).U32(uint32(a.character.X)).U32(uint32(a.character.Y)).U16(0)
	if !contains(wires[2].packets(t), sign) || !contains(wires[0].packets(t), []byte{62, 59, 2}) || !a.character.Bag[0].Locked {
		t.Fatal("native tent open or reservation missing")
	}
	original := []game.Location{{Map: a.character.Map, X: a.character.X, Y: a.character.Y}, {Map: b.character.Map, X: b.character.X, Y: b.character.Y}, {Map: guest.character.Map, X: guest.character.X, Y: guest.character.Y}}
	enterTestTent(t, s, a, a.character.ID)
	warp := protocol.Builder{12}.U32(a.character.ID).U16(63507).U16(460).U16(700).U16(0).U8(0).U16(1).U8(1).U8(1)
	furniture := protocol.Builder{23, 3}.U16(38027).U32(43).U32(42).U32(0).U8(1).U8(0).U16(0)
	interior := wires[0].packets(t)
	if !contains(interior, warp) || !contains(interior, furniture) {
		t.Fatal("native interior packets missing")
	}
	enterTestTent(t, s, b, b.character.ID)
	enterTestTent(t, s, guest, a.character.ID)
	if sameScene(a, b) || s.mapPlayer(a, b.character.ID) != nil || tradeNear(a, b) || !sameScene(a, guest) {
		t.Fatal("private scenes merged")
	}
	for _, w := range wires {
		w.Reset()
	}
	tradeDo(t, s, guest, protocol.Builder{6, 1, 2}.U16(500).U16(600))
	movement := protocol.Builder{6, 1}.U32(guest.character.ID).U8(2).U16(500).U16(600)
	if !contains(wires[0].packets(t), movement) || wires[1].Len() != 0 {
		t.Fatal("private movement leaked")
	}
	tradeDo(t, s, a, []byte{65, 2})
	for _, i := range []int{0, 2} {
		c := players[i]
		if c.tentOwner != 0 || c.character.TentReturn != nil || c.character.Map != original[i].Map || c.character.X != original[i].X {
			t.Fatal("occupant not returned", i)
		}
		rows, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil || rows[0].TentReturn != nil || rows[0].Map != original[i].Map {
			t.Fatal("return not saved", err)
		}
	}
	if a.openTent != nil || a.character.Bag[0].Locked || b.tentOwner != b.character.ID {
		t.Fatal("wrong home closed")
	}
}
func TestTentLocksFurniturePermissionsAndGuestLogout(t *testing.T) {
	s, players, wires := tentFixture(t)
	owner, guest := players[0], players[1]
	tradeDo(t, s, owner, []byte{23, 96, 1})
	s.worldMu.Lock()
	_, err := s.tentChatCommand(context.Background(), owner, "tentlock", nil)
	s.worldMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	tradeDo(t, s, guest, protocol.Builder{65, 1}.U32(owner.character.ID))
	if guest.tentOwner != 0 {
		t.Fatal("locked home admitted guest")
	}
	s.worldMu.Lock()
	_, err = s.tentChatCommand(context.Background(), owner, "tentunlock", nil)
	s.worldMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	enterTestTent(t, s, owner, owner.character.ID)
	enterTestTent(t, s, guest, owner.character.ID)
	place := protocol.Builder{62, 1}.U8(0).U8(2).U32(50).U32(60).U32(0)
	tradeDo(t, s, guest, place)
	home, _ := s.Store.Tent(context.Background(), owner.character.ID)
	if len(home.Items) != 1 {
		t.Fatal("guest placed furniture")
	}
	tradeDo(t, s, owner, place)
	home, _ = s.Store.Tent(context.Background(), owner.character.ID)
	if len(home.Items) != 2 || home.Items[1].Metadata[0] != 9 || owner.character.Bag[1].Count != 1 || !owner.character.Bag[0].Locked {
		t.Fatal("placement damaged state")
	}
	move := protocol.Builder{62, 3}.U16(1).U32(75).U32(85).U32(0).U8(2)
	tradeDo(t, s, owner, move)
	home, _ = s.Store.Tent(context.Background(), owner.character.ID)
	if home.Items[1].X != 75 || home.Items[1].Rotation != 2 {
		t.Fatal("move failed")
	}
	wires[0].Reset()
	s.leaveWorld(guest)
	departure := protocol.Builder{12}.U32(guest.character.ID).U16(0).U16(0).U16(0).U16(0).U8(0)
	if !contains(wires[0].packets(t), departure) {
		t.Fatal("guest departure lost scene identity")
	}
	rows, _ := s.Store.Characters(context.Background(), guest.account.ID)
	if rows[0].TentReturn != nil || rows[0].Map == 63507 {
		t.Fatal("logout recovery failed")
	}
	// A reservation must block direct commits and SQL mail transactions alike.
	next := owner.character.Clone()
	next.Bag[0] = game.Item{}
	if err = s.commit(context.Background(), owner, next); !errors.Is(err, game.ErrItemLocked) {
		t.Fatal("commit removed reserved tent", err)
	}
}
func TestTentOwnerLogoutEvictsLoadingGuestAndReconnectRecovery(t *testing.T) {
	s, players, _ := tentFixture(t)
	owner, guest := players[0], players[1]
	tradeDo(t, s, owner, []byte{23, 96, 1})
	tradeDo(t, s, guest, protocol.Builder{65, 1}.U32(owner.character.ID))
	if guest.ready {
		t.Fatal("fixture guest should still be loading")
	}
	rows, _ := s.Store.Characters(context.Background(), guest.account.ID)
	crashed := rows[0].Clone()
	if !recoverTentCharacter(&crashed) || crashed.Map != 12000 || crashed.TentReturn != nil {
		t.Fatal("crash recovery failed")
	}
	s.leaveWorld(owner)
	if guest.tentOwner != 0 || guest.character.Map != 12000 || guest.character.TentReturn != nil {
		t.Fatal("loading guest orphaned")
	}
}
func TestTargetedGachaRejectsQuantityAndPetBeforeConsumption(t *testing.T) {
	s, players, _ := gachaFixture(t)
	c := players[0]
	before := c.character.Bag
	for _, p := range [][]byte{{23, 15, 9, 2, 0, 0}, {23, 15, 9, 1, 1, 0}} {
		tradeDo(t, s, c, p)
		if c.character.Bag != before {
			t.Fatal("invalid target consumed pack")
		}
	}
	tradeDo(t, s, c, []byte{23, 15, 9, 1, 0, 0})
	if !c.character.Bag[8].Empty() || c.character.Bag[0].ID != 32176 {
		t.Fatal("valid targeted opening failed")
	}
}

func TestTentReadyResyncsFurnitureChangedDuringLoad(t *testing.T) {
	s, players, wires := tentFixture(t)
	owner, guest := players[0], players[1]
	tradeDo(t, s, owner, []byte{23, 96, 1})
	enterTestTent(t, s, owner, owner.character.ID)
	tradeDo(t, s, guest, protocol.Builder{65, 1}.U32(owner.character.ID))
	wires[1].Reset()
	tradeDo(t, s, owner, protocol.Builder{62, 3}.U16(0).U32(100).U32(200).U32(0).U8(3))
	tradeDo(t, s, guest, []byte{12, 1})
	updated := protocol.Builder{23, 3}.U16(38027).U32(100).U32(200).U32(0).U8(1).U8(3).U16(0)
	if !contains(wires[1].packets(t), updated) {
		t.Fatal("ready snapshot missed changed furniture")
	}
}

func TestTentSnapshotReplaysPeerPose(t *testing.T) {
	s, players, wires := tentFixture(t)
	owner, guest := players[0], players[2]
	tradeDo(t, s, owner, []byte{23, 96, 1})
	enterTestTent(t, s, owner, owner.character.ID)
	enterTestTent(t, s, guest, owner.character.ID)
	owner.emote = 3
	wires[2].Reset()
	if err := s.tentSnapshot(context.Background(), guest); err != nil {
		t.Fatal(err)
	}
	want := protocol.Builder{32, 2}.U32(owner.character.ID).U8(3)
	if !contains(wires[2].packets(t), want) {
		t.Fatal("missing native peer pose replay")
	}
}
