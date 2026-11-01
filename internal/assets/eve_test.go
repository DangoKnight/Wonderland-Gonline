package assets

import (
	"testing"
	"wonderland-gonline/internal/protocol"
)

func miningFixture() []byte {
	section := protocol.Builder{}.U16(2)
	for id := uint16(1); id <= 2; id++ {
		section = section.U16(id).Bytes(make([]byte, 20)).U8(5).U32(166 + uint32(id)).U32(89).U8(1).U8(byte(id + 5)).U8(0)
		section = section.U8(0).U32(168 + uint32(id)).U32(93).U8(2).U8(33)
	}
	block := protocol.Builder{0, 0, 0, 0}.Bytes(section)
	for category := 0; category < 11; category++ {
		if category == 2 {
			block = block.U32(4)
		} else {
			block = block.U32(0)
		}
	}
	file := protocol.Builder(make([]byte, 8)).U32(1).U16(11023).U16(1).U32(22).U16(uint16(len(block)))
	return file.Bytes(block)
}

func TestMiningRecordBoundaries(t *testing.T) {
	maps, err := ParseEVE(miningFixture())
	if err != nil {
		t.Fatal(err)
	}
	records := maps[11023].Mining
	if len(records) != 2 {
		t.Fatalf("got %d records", len(records))
	}
	for i, entry := range records {
		if entry.ClickID != uint16(i+1) || entry.X != uint32(167+i) || len(entry.Tail) != 11 || len(entry.Events) != 1 || entry.Events[0] != byte(i+6) {
			t.Fatalf("record %d: %+v", i, entry)
		}
	}
}

func TestTruncatedMiningRecord(t *testing.T) {
	b := miningFixture()
	// Remove the final tail byte while keeping the map index and category table consistent.
	cut := len(b) - 44 - 1
	b = append(b[:cut], b[cut+1:]...)
	le.PutUint16(b[20:], uint16(len(b)-22))
	if _, err := ParseEVE(b); err == nil {
		t.Fatal("accepted truncated mining record")
	}
}

// TestNPCRoutes: an NPC record's sign and speed bytes and its route table (the
// "ParaNode" records of FUN_0047e5b4), keyed by their own slot byte, with
// absent routes dropped.
func TestNPCRoutes(t *testing.T) {
	route := func(slot, present byte, points ...uint32) []byte {
		b := make([]byte, 92)
		b[0], b[1], b[3] = slot, present, byte(len(points)/2)
		for i := 0; i < len(points); i += 2 {
			le.PutUint32(b[12+i*4:], points[i])
			le.PutUint32(b[16+i*4:], points[i+1])
		}
		return b
	}
	npc := protocol.Builder{}.U16(1).U16(7).Bytes(make([]byte, 20)).U8(1).U32(50).U32(60).U8(0).U8(0).U8(4)
	npc = npc.U32(17001).U8(1).U8(2).U8(0).U8(1).U32(70).U32(80).U32(500)
	npc = npc.U8(5).Bytes(make([]byte, 12)).U8(2).Bytes(route(3, 1, 100, 200, 300, 400)).Bytes(route(4, 0, 1, 2))
	npc = npc.U16(0).U16(0).U32(9)
	block := protocol.Builder{0, 0, 0, 0}.Bytes(npc).U32(4)
	for range 10 {
		block = block.U32(0)
	}
	m, err := ParseEVEMap(11003, 1, block)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.NPCs) != 1 {
		t.Fatalf("%d NPCs", len(m.NPCs))
	}
	n := m.NPCs[0]
	want := []RoutePoint{{100, 200}, {300, 400}}
	if n.ClickID != 7 || n.Sign != 4 || n.Speed != 5 || len(n.WalkSteps) != 1 || len(n.Routes) != 1 || len(n.Routes[3]) != 2 || n.Routes[3][0] != want[0] || n.Routes[3][1] != want[1] || n.UnknownWords[2] != 9 {
		t.Fatalf("%+v", n)
	}
}
