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
	want[115] = 1 // Child bypasses the ground collision grid.
	if !bytes.Equal(got, want) {
		t.Fatal("native 57-byte item tree", got)
	}
}
