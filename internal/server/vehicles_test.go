package server

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/config"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
	"wonderland-go/internal/world"
)

func vehicleFixture(t *testing.T) (*Server, *Session, []*captureConn) {
	t.Helper()
	s, c, wires := mountFixture(t)
	for _, id := range []uint16{48001, 48010, 48016, 48028} {
		s.Assets.Items[id] = game.ItemDefinition{ID: id, Type: game.VehicleType}
	}
	next := c.character.Clone()
	next.Bag[1] = game.Item{ID: 48016, Count: 1}
	next.Bag[2] = game.Item{ID: 48016, Count: 1} // Another identical raft must survive.
	next.Bag[3] = game.Item{ID: 48001, Count: 1}
	next.Bag[4] = game.Item{ID: 48028, Count: 1}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	return s, c, wires
}

func vehicleRequest(sub, slot byte, id uint16) []byte { return protocol.Builder{15, sub, slot}.U16(id) }
func vehicleDo(t *testing.T, s *Server, c *Session, sub, slot byte, id uint16) {
	t.Helper()
	if err := s.dispatch(context.Background(), c, vehicleRequest(sub, slot, id)); err != nil {
		t.Fatal(err)
	}
}

func TestVehiclePlacementBoardingAndLanding(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	vehicleDo(t, s, c, 14, 2, 48016)
	want := protocol.Builder{15, 18, 2}.U32(c.character.ID).U16(48016).U32(uint32(c.character.X)).U32(uint32(c.character.Y))
	packets := wires[0].packets(t)
	if len(want) != 17 || len(packets) != 1 || !bytes.Equal(packets[0], want) || c.character.ActiveVehicle != 0 || wires[1].Len() != 0 {
		t.Fatal("placement boarded or replicated", packets)
	}
	vehicleDo(t, s, c, 7, 2, 48016)
	want = protocol.Builder{15, 10, 2}.U32(c.character.ID).U16(48016)
	for _, wire := range wires[:2] {
		p := wire.packets(t)
		if len(p) != 1 || !bytes.Equal(p[0], want) {
			t.Fatal("boarding packet", p)
		}
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || stored[0].ActiveVehicle != 48016 || stored[0].VehicleSlot != 2 {
		t.Fatal("boarding not durable", err)
	}
	if !contains(s.peerPetPackets(c.character), want) {
		t.Fatal("vehicle missing in peer snapshot")
	}
	vehicleDo(t, s, c, 9, 2, 48016)
	if wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("repeated boarding replayed")
	}
	vehicleDo(t, s, c, 10, 2, 48010)
	if c.character.ActiveVehicle != 48016 || wires[0].Len() != 0 {
		t.Fatal("stale landing dismounted another vehicle")
	}
	vehicleDo(t, s, c, 10, 99, 48016) // Landing discriminator is not the inventory slot.
	p := wires[0].packets(t)
	if len(p) != 4 || !bytes.Equal(p[0], []byte{23, 9, 2, 1}) || !bytes.Equal(p[1], protocol.Builder{15, 15}.U32(c.character.ID).U16(48016)) || !bytes.Equal(p[2], protocol.Builder{15, 11, 2}.U32(c.character.ID)) || !bytes.Equal(p[3], []byte{5, 4}) {
		t.Fatal("wreck sequence", p)
	}
	if len(wires[1].packets(t)) != 2 || wires[2].Len() != 0 {
		t.Fatal("wreck map isolation")
	}
	if !c.character.Bag[1].Empty() || c.character.Bag[2].ID != 48016 || c.character.ActiveVehicle != 0 {
		t.Fatal("wreck consumed wrong raft")
	}
	stored, err = s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || !stored[0].Bag[1].Empty() || stored[0].ActiveVehicle != 0 {
		t.Fatal("wreck not saved", err)
	}
	vehicleDo(t, s, c, 10, 99, 48016)
	if wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("wreck callback replayed")
	}
}

func TestVehicleRejectsMalformedAndUnownedCommands(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	for _, p := range [][]byte{{15, 7}, {15, 7, 2}, {15, 14, 2, 0}, {15, 7, 2, 0, 0, 0}, {15, 13, 0}} {
		if err := s.worldCommand(context.Background(), c, p); err == nil {
			t.Fatal("accepted malformed vehicle command", p)
		}
	}
	for _, p := range [][]byte{vehicleRequest(7, 0, 48016), vehicleRequest(7, 51, 48016), vehicleRequest(7, 1, 48016), vehicleRequest(7, 2, 48010), vehicleRequest(14, 2, 48028)} {
		if err := s.worldCommand(context.Background(), c, p); err != nil {
			t.Fatal(err)
		}
	}
	c.character.Bag[1].Damage = 100
	vehicleDo(t, s, c, 7, 2, 48016)
	c.character.Bag[1].Damage = 0
	c.event = &eventSession{}
	vehicleDo(t, s, c, 7, 2, 48016)
	c.event = nil
	if c.character.ActiveVehicle != 0 || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("invalid boarding changed state")
	}
}

func TestVehicleMovesAreBlockedAndRemovalDismounts(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	vehicleDo(t, s, c, 7, 2, 48016)
	wires[0].Reset()
	wires[1].Reset()
	for _, p := range [][]byte{{23, 10, 2, 1, 8}, {23, 10, 3, 1, 2}} {
		if err := s.worldCommand(context.Background(), c, p); err != nil {
			t.Fatal(err)
		}
	}
	if c.character.Bag[1].ID != 48016 || c.character.Bag[2].ID != 48016 || wires[0].Len() != 0 {
		t.Fatal("boarded slot moved")
	}
	if err := s.worldCommand(context.Background(), c, []byte{30, 2, 2}); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveVehicle != 0 || !c.character.Bag[1].Empty() || c.character.Storage[0].ID != 48016 {
		t.Fatal("storage failed to release mount")
	}
	packet := protocol.Builder{15, 11, 2}.U32(c.character.ID)
	if !contains(wires[0].packets(t), packet) || !contains(wires[1].packets(t), packet) {
		t.Fatal("removal not replicated")
	}
}

func TestVehicleRaftWearAndShore(t *testing.T) {
	for _, id := range []uint16{48016, 48028, 48001} {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			s, c, wires := vehicleFixture(t)
			slot := byte(2)
			if id == 48028 {
				slot = 5
			}
			if id == 48001 {
				slot = 4
			}
			vehicleDo(t, s, c, 7, slot, id)
			wires[0].Reset()
			wires[1].Reset()
			move := protocol.Builder{6, 1, 0}.U16(c.character.X + 1).U16(c.character.Y)
			if err := s.worldCommand(context.Background(), c, move); err != nil {
				t.Fatal(err)
			}
			damage := byte(1)
			if id == 48001 {
				damage = 0
			}
			if c.character.Bag[slot-1].Damage != damage {
				t.Fatal("wrong movement wear", id)
			}
			wires[0].Reset()
			wires[1].Reset()
			if err := s.worldCommand(context.Background(), c, move); err != nil {
				t.Fatal(err)
			}
			if c.character.Bag[slot-1].Damage != damage {
				t.Fatal("stationary packet wore vehicle")
			}
			if id == 48001 {
				vehicleDo(t, s, c, 10, 99, id)
				if c.character.ActiveVehicle != 0 || c.character.Bag[slot-1].ID != id {
					t.Fatal("ordinary landing destroyed vehicle")
				}
				return
			}
			next := c.character.Clone()
			next.Bag[slot-1].Damage = 99
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			if err := s.worldCommand(context.Background(), c, protocol.Builder{6, 1, 0}.U16(c.character.X+1).U16(c.character.Y)); err != nil {
				t.Fatal(err)
			}
			if !c.character.Bag[slot-1].Empty() || c.character.ActiveVehicle != 0 {
				t.Fatal("exhausted raft retained")
			}
		})
	}
	s, c, _ := vehicleFixture(t)
	vehicleDo(t, s, c, 7, 2, 48016)
	c.character.Map = 11016
	if err := s.worldCommand(context.Background(), c, protocol.Builder{6, 1, 0}.U16(280).U16(950)); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveVehicle != 0 || !c.character.Bag[1].Empty() || c.character.Bag[2].ID != 48016 {
		t.Fatal("authored beach did not wreck exact raft")
	}
}

func TestVehiclesAndCompanionMountsExcludeEachOther(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	if err := s.worldCommand(context.Background(), c, protocol.Builder{15, 11, 1}.U32(14156)); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	wires[1].Reset()
	vehicleDo(t, s, c, 7, 2, 48016)
	if c.character.ActiveMount != 0 || c.character.ActiveVehicle != 48016 {
		t.Fatal("boarding kept companion mount")
	}
	if !contains(wires[0].packets(t), protocol.Builder{15, 17}.U32(c.character.ID)) {
		t.Fatal("companion not unmounted")
	}
	wires[1].Reset()
	if err := s.worldCommand(context.Background(), c, protocol.Builder{15, 11, 2}.U32(12032)); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveVehicle != 0 || c.character.ActiveMount != 12178 || c.character.Bag[1].ID != 48016 {
		t.Fatal("companion mount kept or destroyed item vehicle")
	}
	if !contains(wires[1].packets(t), protocol.Builder{15, 11, 2}.U32(c.character.ID)) {
		t.Fatal("item dismount not sent")
	}
}

func TestVehicleSaveFailuresPublishNothing(t *testing.T) {
	for _, op := range []string{"board", "wreck", "wear", "remove", "mount"} {
		t.Run(op, func(t *testing.T) {
			s, c, wires := vehicleFixture(t)
			if op != "board" {
				vehicleDo(t, s, c, 7, 2, 48016)
			}
			for _, w := range wires {
				w.Reset()
			}
			before := c.character.Clone()
			s.Store.Close()
			var err error
			switch op {
			case "board":
				err = s.vehicleCommand(context.Background(), c, vehicleRequest(7, 2, 48016))
			case "wreck":
				err = s.wreckVehicle(context.Background(), c)
			case "wear":
				err = s.wearVehicle(context.Background(), c)
			case "remove":
				err = s.destroyItem(context.Background(), c, 2, 1)
			case "mount":
				err = s.mountCommand(context.Background(), c, protocol.Builder{15, 11, 2}.U32(12032))
			}
			if err == nil || c.character.ActiveVehicle != before.ActiveVehicle || c.character.ActiveMount != before.ActiveMount || c.character.Bag != before.Bag || wires[0].Len() != 0 || wires[1].Len() != 0 {
				t.Fatal("failed save adopted or published vehicle mutation", err)
			}
		})
	}
}

func TestVehicleSnapshotRestoreAndPortalLanding(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	vehicleDo(t, s, c, 7, 2, 48016)
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	s.depart(c, c.character.WarpPacket(0))
	c.character = &stored[0]
	for _, w := range wires {
		w.Reset()
	}
	if err := s.acknowledgeWorld(c); err != nil {
		t.Fatal(err)
	}
	mount := vehicleMountPacket(c.character)
	if !contains(wires[0].packets(t), mount) || !contains(wires[1].packets(t), mount) {
		t.Fatal("map acknowledgment did not restore vehicle")
	}
	// A successful regular exit from the authored beach destroys this raft.
	s.Assets.Maps[11016] = assets.Map{ID: 11016, Warps: []assets.Warp{{ClickID: 1, MapID: 10017, X: 1042, Y: 1075}}}
	s.World = world.New(s.Assets)
	c.character.Map = 11016
	c.arrived = false
	for _, w := range wires {
		w.Reset()
	}
	if err := s.enterPortalStep(context.Background(), c, 1, true); err != nil {
		t.Fatal(err)
	}
	if c.character.Map != 10017 || c.character.ActiveVehicle != 0 || !c.character.Bag[1].Empty() || c.character.Bag[2].ID != 48016 {
		t.Fatal("regular beach portal did not land raft")
	}
	if !contains(wires[0].packets(t), protocol.Builder{15, 15}.U32(c.character.ID).U16(48016)) {
		t.Fatal("portal break not reported")
	}
}

func TestVehicleStaleBagSlotNeverConsumesAnotherItem(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	vehicleDo(t, s, c, 7, 2, 48016)
	// Simulate stale imported/runtime state. No search for the other raft is allowed.
	c.character.Bag[1] = game.Item{ID: 48001, Count: 1}
	for _, w := range wires {
		w.Reset()
	}
	if err := s.wreckVehicle(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveVehicle != 0 || c.character.Bag[1].ID != 48001 || c.character.Bag[2].ID != 48016 {
		t.Fatal("wreck found an unrelated replacement")
	}
	p := wires[0].packets(t)
	if len(p) != 1 || !bytes.Equal(p[0], protocol.Builder{15, 11, 2}.U32(c.character.ID)) {
		t.Fatal("stale mount sent break/removal", p)
	}
}

func TestVehicleLoginAfterDatabaseReopen(t *testing.T) {
	s, c, _ := vehicleFixture(t)
	vehicleDo(t, s, c, 7, 2, 48016)
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	// A separate persisted DB exercises load/serialization and a fresh TCP session.
	path := filepath.Join(t.TempDir(), "reconnect.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	account, err := db.Register(context.Background(), "Alice", "password", "")
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	char := chars[0]
	char.ID = account.CharacterID(1)
	if err := db.CreateCharacter(context.Background(), account, char); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	db, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server := New(config.Default(), db, s.Assets, s.Log)
	wire := &captureConn{}
	session := &Session{conn: wire, info: SessionInfo{ID: 10}}
	if err := server.dispatch(context.Background(), session, loginPayload("Alice")); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	if err := server.dispatch(context.Background(), session, []byte{63, 2, 1}); err != nil {
		t.Fatal(err)
	}
	if session.character.ActiveVehicle != 48016 || session.character.VehicleSlot != 2 || session.character.Bag[1].Count != 1 {
		t.Fatal("login lost or duplicated boarded item")
	}
	for _, p := range wire.packets(t) {
		if len(p) > 1 && p[0] == 15 && p[1] == 10 {
			t.Fatal("mount sent before map acknowledgment")
		}
	}
	if err := server.dispatch(context.Background(), session, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	if !contains(wire.packets(t), vehicleMountPacket(session.character)) {
		t.Fatal("reopened vehicle was not synchronized")
	}
}

func TestVehicleSwitchAndBattleGate(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	vehicleDo(t, s, c, 7, 2, 48016)
	for _, w := range wires {
		w.Reset()
	}
	c.battle = &battleRun{}
	vehicleDo(t, s, c, 7, 4, 48001)
	c.battle = nil
	if c.character.ActiveVehicle != 48016 || wires[0].Len() != 0 {
		t.Fatal("boarded during battle")
	}
	vehicleDo(t, s, c, 9, 4, 48001)
	for _, w := range wires[:2] {
		p := w.packets(t)
		if len(p) != 2 || !bytes.Equal(p[0], protocol.Builder{15, 11, 2}.U32(c.character.ID)) || !bytes.Equal(p[1], protocol.Builder{15, 10, 4}.U32(c.character.ID).U16(48001)) {
			t.Fatal("switch ordering", p)
		}
	}
	if c.character.Bag[1].ID != 48016 || c.character.ActiveVehicle != 48001 {
		t.Fatal("switch consumed previous vehicle")
	}
}
