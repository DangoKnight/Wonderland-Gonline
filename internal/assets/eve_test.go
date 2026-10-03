package assets

import (
	"testing"
	"wonderland-go/internal/protocol"
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
