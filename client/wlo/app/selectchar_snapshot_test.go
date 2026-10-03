package app

import (
	"time"

	"image/png"
	"os"
	"path/filepath"
	"testing"
	"wonderland-go/client/wlo/role"
)

// TestSelectCharacterSnapshot writes the character selection frame to
// SNAPSHOT_DIR for comparison with reference screenshots.
func TestSelectCharacterSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("SNAPSHOT_DIR is not set")
	}
	c := testClient(t)
	c.Frame()
	c.Servers.Hide()
	c.Chars.Roster([]byte{1})
	c.Frame()
	save(t, c, filepath.Join(dir, "select_character.png"))
	// Two starter characters (outfits from internal/game StarterOutfit).
	p := []byte{1}
	// Colours are nine digits per value, 4 neutral: Dango is neutral, Ann
	// has shifted ranges 0x10..0x3f and 0x40, 0xd0.
	p = append(p, slotRecord(1, "Dango", 4, 0, 444444444, 444444444, []uint16{22005, 21006, 23001, 24006})...)
	p = append(p, slotRecord(2, "Ann", 1, 0, 714472228, 444274444, []uint16{21001, 24001})...)
	// A fixed clock keeps the idle animation on its first frame.
	now := time.Now()
	for _, r := range c.Chars.Roles {
		if h, ok := r.(*role.Human); ok {
			h.Now = func() time.Time { return now }
		}
	}
	c.Chars.Roster(p)
	c.Frame()
	save(t, c, filepath.Join(dir, "select_character_roles.png"))

	// Costumes (slot 6): Knight's Cape and Angel Wings+5.
	p = []byte{1}
	p = append(p, slotRecord(1, "Dango", 4, 0, 444444444, 444444444, []uint16{22005, 21006, 23001, 24006, 25365})...)
	p = append(p, slotRecord(2, "Ann", 1, 0, 714472228, 444274444, []uint16{21001, 24001, 25593})...)
	c.Chars.Roster(p)
	c.Frame()
	save(t, c, filepath.Join(dir, "select_character_costumes.png"))
}

// slotRecord builds one 63/1 slot record.
func slotRecord(slot byte, name string, body, head byte, color1, color2 uint32, equip []uint16) []byte {
	r := []byte{slot, byte(len(name))}
	r = append(r, name...)
	r = append(r, 1, 3)
	for _, v := range []uint32{100, 100, 50, 50, 6, 1000} {
		r = append(r, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
	}
	r = append(r, body, 0, head, 0)
	for _, v := range []uint32{color1, color2} {
		r = append(r, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
	}
	r = append(r, 0, 0)
	for i := range 6 {
		var id uint16
		if i < len(equip) {
			id = equip[i]
		}
		r = append(r, byte(id), byte(id>>8))
	}
	return r
}

func save(t *testing.T, c *Client, path string) {
	// Character colours need each archive's sprites.json, opened in the
	// background on first use; wait for it and draw again.
	c.Sprites.WaitNative()
	c.Frame()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, c.Screen.RGBA())
}
