package assets

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"wonderland-gonline/internal/game"
)

func TestNativeNPCOffsets(t *testing.T) {
	b := make([]byte, 276)
	r := b[138:]
	binary.LittleEndian.PutUint16(r[12:], (10001+1)^0x5209)
	r[37] = (12 + 1) ^ 0xc8
	r[57] = (3 + 1) ^ 0xc8
	binary.LittleEndian.PutUint32(r[38:], (1200+1)^0x0baeb716)
	copy(r[1:11], []byte("acoR      "))
	for i := 0; i < 5; i++ {
		binary.LittleEndian.PutUint16(r[46+i*2:], uint16(20+i+1)^0x5209)
	}
	n, e := ParseNPCs(b)
	if e != nil {
		t.Fatal(e)
	}
	v := n[10001]
	if v.Name != "Roca" || v.Level != 12 || v.HP != 1200 || v.Stats[4] != 24 {
		t.Fatal(v)
	}
}
func TestTalkIDBeforeOffset(t *testing.T) {
	b := make([]byte, 292)
	binary.LittleEndian.PutUint16(b, (812+5)^0xecea)
	b[2] = 5
	copy(b[252:257], "olleH")
	v, e := ParseTalks(b)
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []uint32{812, 252} {
		s, ok := v.Resolve(id)
		if !ok || s != "Hello" {
			t.Fatalf("%d: %q", id, s)
		}
	}
	if _, ok := v.Resolve(1); ok {
		t.Fatal("record index must not masquerade as native ID")
	}
}
func TestEVEBounds(t *testing.T) {
	for _, b := range [][]byte{nil, make([]byte, 10), append(make([]byte, 8), 255, 255, 255, 127)} {
		if _, e := ParseEVE(b); e == nil {
			t.Fatal("accepted invalid map index")
		}
	}
}
func FuzzEVE(f *testing.F) {
	f.Add(make([]byte, 12))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			t.Skip()
		}
		ParseEVE(b)
	})
}

// Enable with WONDERLAND_TEST_DATA to validate a legally supplied native data directory.
func TestNativeAssets(t *testing.T) {
	dir := os.Getenv("WONDERLAND_TEST_DATA")
	if dir == "" {
		t.Skip("set WONDERLAND_TEST_DATA for native asset checks")
	}
	if !filepath.IsAbs(dir) {
		_, source, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("cannot locate module root")
		}
		dir = filepath.Join(filepath.Dir(source), "../..", dir)
	}
	c, err := Load(dir, filepath.Join("../..", "data/item_data.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := c.Summary()
	t.Logf("%+v", s)
	if len(s.Warnings) != 1 || s.Warnings[0] != "gacha_packs.json: unknown gacha pack 34381" || s.Items != 7312 {
		t.Fatal(s.Warnings)
	}
	if s.NPCs < 4000 || s.Dialogues < 15000 || s.Maps < 1000 || s.MapNPCs < 8000 || s.Events < 10000 || s.Warps < 2000 {
		t.Fatal("native asset census is incomplete")
	}
}

func TestNativeStarterCharacters(t *testing.T) {
	dir := os.Getenv("WONDERLAND_TEST_DATA")
	if dir == "" {
		t.Skip("set WONDERLAND_TEST_DATA for native starter checks")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join("../..", dir)
	}
	catalog, err := Load(dir, filepath.Join("../..", "data/item_data.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Warnings) != 1 || catalog.Warnings[0] != "gacha_packs.json: unknown gacha pack 34381" {
		t.Fatal(catalog.Warnings)
	}
	for body, heads := range map[uint16]int{1: 1, 2: 2, 3: 4, 4: 8} {
		for head := 0; head < heads; head++ {
			for element := byte(1); element <= 4; element++ {
				c, e := game.NewCharacter(10001, 1, "Player", game.Appearance{Body: body, Head: uint16(head), Element: element, Base: game.Attributes{Strength: 5, Constitution: 5, Intelligence: 5, Wisdom: 5, Agility: 5}}, catalog.StarterItems, catalog.Items, time.Unix(0, 0))
				if e != nil {
					t.Fatalf("body %d head %d element %d: %v", body, head, element, e)
				}
				if _, e = c.BaseStatsPacket(func(id uint16) (uint16, bool) { skill, ok := catalog.Skills[id]; return skill.TableOrder, ok }); e != nil {
					t.Fatal(e)
				}
				if c.Bag[0].ID != 34038 || c.Bag[2].ID != 32176 || c.Bag[2].Count != 50 {
					t.Fatal("native starter order/count")
				}
			}
		}
	}
}
