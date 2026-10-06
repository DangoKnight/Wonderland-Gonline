package role

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/surface"
)

func TestArchiveLookup(t *testing.T) {
	root := filepath.Join("..", "..", "..", "data")
	if _, err := os.Stat(filepath.Join(root, "sprites")); err != nil {
		t.Skip("client assets not installed")
	}
	l := NewLibrary(root)
	for _, tc := range []struct {
		base  string
		id    int
		entry string
	}{
		{"002", 2000, "2000.jmp"},
		{"002h", 2100, "2100.jmp"},
		{"002c", 2200, "2200.jmp"},
		{"002c", 2245, "2245.jmp"},   // 002c_c01
		{"002c", 2900, "2900.jmp"},   // 002c1
		{"002w", 2001, "2001.jmp"},   // the base weapon archive is one ID lower
		{"002w", 2712, "2712.jmp"},   // 002w1_c01 clears the shift
		{"002w", 12050, "12050.jmp"}, // 002w2
	} {
		a, key := l.lookup(tc.base, tc.id)
		if a == nil {
			t.Fatalf("%s %d: no archive", tc.base, tc.id)
		}
		i := a.index(key)
		if s := a.arc.Sprite(i); s == nil || s.Name != tc.entry {
			t.Errorf("%s %d: entry %d", tc.base, tc.id, i)
		}
	}
}

// TestWeaponPatchOrder follows FUN_0045ac44's order: for 2700 the later
// "002w_01" check (against 2699) wins. That archive is not installed, so
// the base archive is used, and it has no such sprite.
func TestWeaponPatchOrder(t *testing.T) {
	root := filepath.Join("..", "..", "..", "data")
	if _, err := os.Stat(filepath.Join(root, "sprites", "002w")); err != nil {
		t.Skip("sprite packs not built")
	}
	a, key := NewLibrary(root).lookup("002w", 2700)
	if a == nil || a.sprite(key) != nil {
		t.Fatal("2700 should fall back to 002w and find nothing there")
	}
}

// TestMissingPatchFallsBack: cape sprite 2335 chooses 002e_1, which this
// install lacks; its base 002e carries the sprite.
func TestMissingPatchFallsBack(t *testing.T) {
	root := filepath.Join("..", "..", "..", "data")
	if _, err := os.Stat(filepath.Join(root, "sprites", "002e")); err != nil {
		t.Skip("sprite packs not built")
	}
	a, key := NewLibrary(root).lookup("002e", 2335)
	if a == nil || a.sprite(key) == nil {
		t.Fatal("2335 should be found in 002e")
	}
}

func TestRaftRiderUsesDirectionalSeatedFrames(t *testing.T) {
	root := filepath.Join("..", "..", "..", "data")
	if _, err := os.Stat(filepath.Join(root, "sprites", "002")); err != nil {
		t.Skip("client sprite assets not installed")
	}
	h := NewHuman(NewLibrary(root), nil)
	h.body = 2
	h.Now = func() time.Time { return time.Unix(0, 0) }
	h.SetVehiclePose(6005, true)
	for facing := int32(0); facing < 8; facing++ {
		h.DrawBody(surface.New(200, 200), 100, 100, facing+8)
		if h.lastAction != 46+int(facing) {
			t.Fatalf("raft facing %d used action %d", facing, h.lastAction)
		}
		arc, key := h.Lib.lookup("002", 2000)
		if arc == nil || arc.sprite(key) == nil || arc.sprite(key).frameCount(46+int(facing)) == 0 {
			t.Fatalf("native seated frame missing: %d", facing)
		}
	}
	h.SetVehicle(0)
	h.DrawBody(surface.New(200, 200), 100, 100, 12)
	if h.lastAction != 12 {
		t.Fatal("dismount retained raft pose")
	}
	h.SetVehiclePose(6005, false)
	h.DrawBody(surface.New(200, 200), 100, 100, 12)
	if h.lastAction != 12 {
		t.Fatal("non-water vehicle received raft pose")
	}
}
