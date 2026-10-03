package app

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wonderland-go/client/wlo/world"

	"wonderland-go/internal/protocol"
)

// selfPacket builds AC3 as the server sends it (game.AppearancePacket).
func selfPacket(id uint32, body byte, mapID, x, y uint16, head byte, c1, c2 uint32, items []uint16, name string) []byte {
	p := []byte{protocol.CommandMapLoad}
	p = binary.LittleEndian.AppendUint32(p, id)
	p = append(p, body)
	for _, v := range []uint16{mapID, x, y} {
		p = binary.LittleEndian.AppendUint16(p, v)
	}
	p = append(p, 0, head, 0)
	p = binary.LittleEndian.AppendUint32(p, c1)
	p = binary.LittleEndian.AppendUint32(p, c2)
	p = append(p, byte(len(items)))
	for _, it := range items {
		p = binary.LittleEndian.AppendUint16(p, it)
	}
	p = binary.LittleEndian.AppendUint32(p, 0)
	p = append(p, byte(len(name)))
	p = append(p, name...)
	p = append(p, 0)
	return binary.LittleEndian.AppendUint32(p, 0)
}

// baseStats builds 5/3 as the server sends it (game.BaseStatsPacket)
// without skills.
func baseStats(element byte, hp uint32, sp uint16, attrs [5]uint16, level byte, exp uint32, maxHP, maxSP uint16) []byte {
	p := []byte{protocol.CommandCharacterState, protocol.CharacterStateWireCode3, element}
	p = binary.LittleEndian.AppendUint32(p, hp)
	p = binary.LittleEndian.AppendUint16(p, sp)
	for _, a := range attrs {
		p = binary.LittleEndian.AppendUint16(p, a)
	}
	p = append(p, level)
	p = binary.LittleEndian.AppendUint32(p, exp)
	p = binary.LittleEndian.AppendUint16(p, maxHP)
	p = binary.LittleEndian.AppendUint16(p, maxSP)
	p = binary.LittleEndian.AppendUint32(p, 417)
	p = binary.LittleEndian.AppendUint16(p, 0)
	for _, v := range []uint32{0, 240, 0, 0, 0, 0, 0} {
		p = binary.LittleEndian.AppendUint32(p, v)
	}
	p = binary.LittleEndian.AppendUint16(p, 0) // no skills
	return append(p, 0, 0, 0, 0, 0, 0, 0, 0)   // two words, rebirth, job, two bytes
}

// statUpdate builds 8/1 (game.StatPackets).
func statUpdate(id byte, v uint32) []byte {
	p := []byte{protocol.CommandStats, protocol.StatsStatUpdate, id, 1}
	p = binary.LittleEndian.AppendUint32(p, v)
	return binary.LittleEndian.AppendUint32(p, 0)
}

// TestWorldSnapshot renders the player on the Ship Deck at the position of
// In-Game/Ship_Deck.png.
func TestWorldSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("SNAPSHOT_DIR is not set")
	}
	c := testClient(t)
	now := time.Now()
	c.Now = func() time.Time { return now }
	c.Frame()
	c.G.ServerText = []byte("Wonderland Go") // 1/9 from this repository's server
	c.dispatch(selfPacket(10001, 2, 10017, 1042, 1075, 0, 444444444, 444444444, []uint16{22003, 21002, 24002}, "Dango"))
	if c.World == nil {
		t.Fatal("AC3 did not open the world")
	}
	// The capture's values: level 1, HP 181/181, SP 100/100, no gold; the
	// maxima come from 8/1's bonuses through Formula.Dat (CON 0, WIS 1).
	c.dispatch(baseStats(1, 181, 100, [5]uint16{0, 0, 0, 0, 1}, 1, 0, 181, 100))
	for _, u := range [][2]uint32{{0x1d, 0}, {0x21, 1}, {0xcf, 0}, {0xd0, 0}, {0x19, 181}, {0x1a, 100}} {
		c.dispatch(statUpdate(byte(u[0]), u[1]))
	}
	c.dispatch([]byte{protocol.CommandGold, protocol.GoldBalance, 0, 0, 0, 0})
	if c.Stats.MaxHP != 181 || c.Stats.MaxSP != 100 {
		t.Fatalf("maxima %d/%d", c.Stats.MaxHP, c.Stats.MaxSP)
	}
	c.Frame()
	// The capture caught the welcome ticker 33 characters in.
	now = now.Add(33 * 100 * time.Millisecond)
	c.Frame()
	save(t, c, filepath.Join(dir, "world_ship_deck.png"))

	// A click to the lower left starts a walk; 300 ms later the player is
	// on the way with the walking pose.
	c.GroundClick(250, 380)
	if !c.World.Walking() {
		t.Fatal("the click did not start a walk")
	}
	now = now.Add(300 * time.Millisecond)
	c.Frame()
	save(t, c, filepath.Join(dir, "world_walking.png"))

	// In-Game/Ship_Deck_Talking.png: the Visitor with the man's face (template 14080) says
	// talk 30126 to the player at (922, 1295).
	c.dispatch([]byte{protocol.CommandEvent, protocol.EventResume})
	c.World.StopWalk()
	c.World.Player.X, c.World.Player.Y = 922, 1295
	for _, n := range c.World.NPCs {
		if n.Template == visitorTemplate {
			c.dispatch(talkFrame(n.ClickID, 30126))
		}
	}
	if !c.Talk.Shown() {
		t.Fatal("no Visitor on the deck")
	}
	settle(c, &now)
	save(t, c, filepath.Join(dir, "world_talking.png"))

	// The Persian Cat (click 8) has no face sprite: its body stands in the
	// window.
	c.dispatch([]byte{protocol.CommandEvent, protocol.EventResume})
	c.World.Player.X, c.World.Player.Y = 922, 955 // Cat_Dialogue.png
	c.dispatch(talkFrame(8, 30137))
	settle(c, &now)
	save(t, c, filepath.Join(dir, "world_cat_talking.png"))

	// Highlight.png: the pointer over the cat lights it.
	c.dispatch([]byte{protocol.CommandEvent, protocol.EventResume})
	settle(c, &now)
	c.World.Player.X, c.World.Player.Y = 982, 955
	c.Frame()
	cat := c.World.NPCs[8]
	cx, cy := c.World.Camera()
	r := cat.Painter.Bounds(cat.X-cx, cat.Y-cy, cat.Action)
	c.Input.X, c.Input.Y = r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	c.Frame()
	if c.World.Hovered() != cat {
		t.Fatal("the cat is not hovered")
	}
	c.Frame()
	save(t, c, filepath.Join(dir, "world_cat_hover.png"))

	// A question (map 10001's first: a prompt and two options) asked by
	// the Visitor.
	rec, err := world.MapRecord(c.Assets, 10001)
	if err != nil {
		t.Fatal(err)
	}
	c.World.Questions = world.MapQuestions(rec)
	for _, n := range c.World.NPCs {
		if n.Template == visitorTemplate {
			c.dispatch(questionFrame(n.ClickID, 1))
		}
	}
	settle(c, &now)
	p := c.Talk.OptionPoint(0)
	c.Input.X, c.Input.Y = p.X, p.Y
	c.Frame()
	save(t, c, filepath.Join(dir, "world_question.png"))
}

const visitorTemplate = 14080

// settle runs frames 16 ms apart until the talk window's animation ends.
func settle(c *Client, now *time.Time) {
	for i := 0; i < 6; i++ {
		*now = now.Add(16 * time.Millisecond)
		c.Frame()
	}
}
