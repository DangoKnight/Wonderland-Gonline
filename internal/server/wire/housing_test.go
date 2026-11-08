package wire

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/store"
)

func TestTentAccessoryTreeGolden(t *testing.T) {
	tent := store.Tent{OwnerID: 0x01020304, Items: []store.TentItem{{ItemID: 38049, X: 43, Y: 42, Rotation: 2, Tools: []store.TentTool{{ItemID: 38058}}}}}
	got := NativeTentTree(tent)
	want := make([]byte, 124)
	copy(want, []byte{62, 4, 4, 3, 2, 1, 1, 0, 0xa1, 0x94, 43, 0, 0, 0, 42, 0, 0, 0, 1})
	want[22] = 2 // Root rotation.
	want[27] = 2 // Root's first child index.
	copy(want[65:], []byte{2, 0, 0xaa, 0x94})
	want[82] = 1  // Child's parent index.
	want[114] = 1 // Child level byte at 0x2F of its record (client layout).
	if !bytes.Equal(got, want) {
		t.Fatal("native 57-byte item tree", got)
	}
}

func TestTentTreeFloorFilterAndLevelByte(t *testing.T) {
	tent := store.Tent{OwnerID: 7, Items: []store.TentItem{
		{ItemID: 38049, X: 1, Y: 1, Floor: 0},
		{ItemID: 38027, X: 2, Y: 2, Floor: 1},
		{ItemID: 38055, X: 3, Y: 3, Floor: 1},
	}}
	ground := NativeTentTreeFloor(tent, 0)
	if len(ground) != 6+59 {
		t.Fatalf("ground layout length %d", len(ground))
	}
	if ground[6+49] != 0 {
		t.Fatal("ground record level byte")
	}
	upper := NativeTentTreeFloor(tent, 1)
	if len(upper) != 6+2*59 {
		t.Fatalf("upper layout length %d", len(upper))
	}
	for i := 0; i < 2; i++ {
		record := upper[6+i*59:]
		if record[49] != 1 {
			t.Fatalf("record %d level byte %d", i, record[49])
		}
	}
	all := NativeTentTree(tent)
	if len(all) != 6+3*59 {
		t.Fatalf("full layout length %d", len(all))
	}
}
