package app

import (
	"bytes"
	"encoding/binary"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

// TestDeckDoor: Ship Deck's cabin door is area 1, cells (93, 41) to
// (96, 45) with the small door light 37, 57 pixels from its corner.
// Walking into it sends 20/8 with its ID once the map is ready (5/4);
// after the server's 20/8 the player must leave the area before it
// triggers again.
func TestDeckDoor(t *testing.T) {
	c, now, _ := enteredClient(t)
	if len(c.World.Areas) != 1 {
		t.Fatalf("areas %+v", c.World.Areas)
	}
	door := c.World.Areas[0]
	if door.ID != 1 || door.Rect != image.Rect(1840, 800, 1920, 900) || door.Light != world.SmallLight || door.LightAt != image.Pt(1877, 857) {
		t.Fatalf("door %+v", door)
	}
	sent := wire(t, c)
	enter := binary.LittleEndian.AppendUint16([]byte{protocol.CommandEvent, protocol.EventPortal}, door.ID)
	seen := 0
	step := func(x, y int) [][]byte {
		*now = now.Add(areaCheckInterval)
		c.World.Player.X, c.World.Player.Y = x, y
		c.Frame()
		all := sent()
		fresh := all[seen:]
		seen = len(all)
		return fresh
	}
	// Until 5/4 marks the map ready the door is quiet.
	if got := step(1880, 880); len(got) != 0 {
		t.Fatalf("before 5/4 the door sent %x", got)
	}
	c.dispatch([]byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh})
	if got := step(1782, 875); len(got) != 0 {
		t.Fatalf("outside the door sent %x", got)
	}
	if got := step(1880, 880); len(got) != 1 || !bytes.Equal(got[0], enter) {
		t.Fatalf("entering sent %x, want %x", got, enter)
	}
	c.dispatch([]byte{protocol.CommandEvent, protocol.EventResume})
	if got := step(1890, 885); len(got) != 0 {
		t.Fatalf("staying inside sent %x", got)
	}
	step(1782, 875)
	if got := step(1880, 880); len(got) != 1 || !bytes.Equal(got[0], enter) {
		t.Fatalf("re-entering sent %x, want %x", got, enter)
	}
}

// TestDoorTeleportWalks: a door's event holds the player (6/2 with 1) and
// teleports; the arrival's closing 20/8 releases the hold, so the player
// walks on the new map.
func TestDoorTeleportWalks(t *testing.T) {
	c, _, legs := enteredClient(t)
	c.enterArea(1)
	c.dispatch([]byte{protocol.CommandMovement, protocol.MovementMovementLock, 1})
	c.dispatch([]byte{protocol.CommandEvent, protocol.EventClose})
	warp := binary.LittleEndian.AppendUint32([]byte{protocol.CommandMapAcknowledgment}, c.World.Player.ID)
	warp = binary.LittleEndian.AppendUint16(warp, 10027)
	warp = binary.LittleEndian.AppendUint16(warp, 674)
	warp = binary.LittleEndian.AppendUint16(warp, 1067)
	warp = append(binary.LittleEndian.AppendUint16(warp, 1), 0)
	c.dispatch(warp)
	if c.World.Player.Map != 10027 {
		t.Fatalf("map %d after AC12", c.World.Player.Map)
	}
	c.World.OnLeg = func(f, x, y int) { *legs = append(*legs, nil) }
	c.dispatch([]byte{protocol.CommandEvent, protocol.EventResume})
	c.GroundClick(400+60, 300)
	if c.held || c.event.active || len(*legs) == 0 {
		t.Fatalf("after the teleport: held %v, event %v, legs %d", c.held, c.event.active, len(*legs))
	}
}

// TestDeckDoorLight renders the door's light where the capture
// (In-Game/Ship_Deck_Teleport_Event.png) shows it.
func TestDeckDoorLight(t *testing.T) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("SNAPSHOT_DIR not set")
	}
	c, now, _ := enteredClient(t)
	c.World.Player.X, c.World.Player.Y = 1782, 875
	for i := 0; i < 3; i++ {
		*now = now.Add(300 * time.Millisecond)
		c.Frame()
		save(t, c, filepath.Join(dir, "deck_door_"+string(rune('a'+i))+".png"))
	}
}

// TestArrivalPropSilent: the arrival replays an opened chest with AC22:1
// before the map is ready; only an opening after 5/4 sounds.
func TestArrivalPropSilent(t *testing.T) {
	c := testClient(t)
	var played []string
	c.Env.Sound = func(path string) { played = append(played, path) }
	c.dispatch(selfPacket(10001, 4, 10003, 1282, 2275, 7, 444444444, 444444444, nil, "Dango"))
	c.dispatch([]byte{protocol.CommandScene, protocol.SceneActorPosition, 2, 0, 0, 0, 0x41, 3, 0x5e, 7, 1, 0, 0, 0, 0, 0})
	c.dispatch([]byte{protocol.CommandScene, protocol.SceneActorState, 2, 0, 1})
	if len(played) != 0 {
		t.Fatalf("arrival replay played %v", played)
	}
	c.dispatch([]byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh})
	c.dispatch([]byte{protocol.CommandScene, protocol.SceneActorPosition, 3, 0, 0, 0, 0xdd, 3, 0x2f, 8, 1, 0, 0, 0, 0, 0})
	c.dispatch([]byte{protocol.CommandScene, protocol.SceneActorState, 3, 0, 1})
	if len(played) != 1 || played[0] != `Sound\wav9900.wav` {
		t.Fatalf("opening after 5/4 played %v", played)
	}
}
