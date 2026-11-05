package app

import (
	"reflect"
	"slices"
	"testing"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/world"
)

func TestShapeshiftNativeRecords(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.dispatch(selfPacket(0x01020304, 4, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "ShapeTest"))
	c.npcTemplates = map[uint32]world.NPCTemplate{14531: {Name: "Bimbashi", Look: 123}}
	c.World.Peers = map[uint32]*world.Peer{9: {Player: world.Player{ID: 9}}}
	before := c.World.Player
	// FUN_004560c0 reads one or more six-byte owner/template records.
	c.dispatch([]byte{5, 5, 4, 3, 2, 1, 195, 56, 9, 0, 0, 0, 195, 56})
	if c.World.Shapeshifts[before.ID].Template != 14531 || c.World.Shapeshifts[9].Template != 14531 || !reflect.DeepEqual(before, c.World.Player) {
		t.Fatal("disguise not cosmetic or batch lost")
	}
	c.dispatch([]byte{5, 5, 4, 3, 2, 1, 255, 255})
	if c.World.Shapeshifts[before.ID].Template != 14531 {
		t.Fatal("unknown template replaced shape")
	}
	c.dispatch([]byte{5, 5, 4, 3, 2, 1, 0})
	if len(c.World.Shapeshifts) != 2 {
		t.Fatal("malformed shape changed state")
	}
	c.dispatch([]byte{5, 5, 4, 3, 2, 1, 0, 0})
	if _, ok := c.World.Shapeshifts[before.ID]; ok {
		t.Fatal("shape not restored")
	}
	c.World.RemovePeer(9)
	if len(c.World.Shapeshifts) != 0 {
		t.Fatal("departed peer retained disguise")
	}
}

func TestShapeshiftRenderingAndBattleRestoration(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.dispatch(selfPacket(0x01020304, 4, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "ShapeTest"))
	if _, known := c.npcTemplates[14531]; !known {
		t.Fatal("native disguise missing")
	}
	player := c.World.Player
	f := clientbattle.Fighter{ID: player.ID, Kind: clientbattle.Player}
	before := c.battleBody(f)
	if before.human == nil {
		t.Fatal("normal player body missing")
	}
	c.dispatch([]byte{5, 5, 4, 3, 2, 1, 195, 56})
	shape := c.World.Shapeshifts[player.ID]
	if shape.Painter == nil {
		t.Fatal("disguise painter missing")
	}
	render := surface.New(300, 300)
	shape.Painter.Draw(render, 150, 250, 11)
	if !slices.ContainsFunc(render.Pix, func(pixel uint16) bool { return pixel != 0 }) {
		t.Fatal("disguise sprite rendered empty")
	}
	if _, ok := c.MainStatus.Portrait.(*shapePortrait); !ok {
		t.Fatal("status portrait retained human")
	}
	disguised := c.battleBody(f)
	if disguised.npc == nil || disguised.human != nil || string(disguised.name) != string(player.Name) {
		t.Fatal("battle disguise lost player identity")
	}
	c.dispatch([]byte{5, 5, 4, 3, 2, 1, 0, 0})
	restored := c.battleBody(f)
	if restored.human == nil || restored.npc != nil || c.MainStatus.Portrait != c.World.Body || !reflect.DeepEqual(player, c.World.Player) {
		t.Fatal("disguise restoration damaged original appearance")
	}
}
