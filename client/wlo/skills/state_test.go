package skills

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/internal/assets"
)

func testState() *State {
	c := &Catalog{Definitions: map[uint16]Definition{11016: {Skill: assets.Skill{ID: 11016}}, 11166: {Skill: assets.Skill{ID: 11166}}}, Orders: map[uint16]uint16{94: 11016, 113: 11166}}
	return NewState(c)
}
func snapshot(records ...[]byte) []byte {
	p := make([]byte, 64)
	p[0], p[1] = 5, 3
	binary.LittleEndian.PutUint16(p[62:], uint16(len(records)))
	for _, r := range records {
		p = append(p, r...)
	}
	return append(p, make([]byte, 8)...)
}
func TestSnapshotAndIncrementalGolden(t *testing.T) {
	s := testState()
	p := snapshot([]byte{94, 0, 2, 50, 0, 0, 0}, []byte{113, 0, 1, 0, 0, 0, 0})
	if !s.Snapshot(p) || s.Learned[11016] != (Progress{Grade: 2, EXP: 50, Proficiency: 2500}) {
		t.Fatal("native snapshot", s.Learned)
	}
	for _, p := range [][]byte{{5, 11, 8, 43, 0, 0, 0x88, 0x13}, {8, 1, 110, 1, 3, 0, 0, 0, 8, 43, 0, 0}, {5, 12, 8, 43, 4}} {
		if !s.Apply(p) {
			t.Fatal("incremental packet", p)
		}
	}
	if s.Learned[11016].Grade != 4 || s.Learned[11016].Proficiency != 5000 {
		t.Fatal("progress", s.Learned)
	}
	if !s.Snapshot(snapshot()) || len(s.Learned) != 0 {
		t.Fatal("snapshot didn't replace stale skills")
	}
}
func TestMalformedSkillUpdatesPreserveState(t *testing.T) {
	s := testState()
	s.Snapshot(snapshot([]byte{94, 0, 2, 50, 0, 0, 0}))
	before := map[uint16]Progress{11016: s.Learned[11016]}
	for _, p := range [][]byte{
		snapshot([]byte{94, 0, 2, 50, 0, 0, 0}, []byte{94, 0, 1, 0, 0, 0, 0}),
		snapshot([]byte{94, 0, 11, 0, 0, 0, 0}),
		{5, 12, 8, 43, 11}, {5, 11, 8, 43, 0, 0, 0x11, 0x27}, {8, 1, 110, 1, 2, 0, 0, 0, 8, 43, 1, 0},
	} {
		valid := s.Apply(p)
		if len(p) > 2 && p[0] == 5 && p[1] == 3 {
			valid = s.Snapshot(p)
		}
		if valid || !reflect.DeepEqual(s.Learned, before) {
			t.Fatal("malformed update mutated state", p)
		}
	}
	p := snapshot([]byte{94, 0, 2, 50, 0, 0, 0})
	for n := 0; n < len(p); n++ {
		if n == len(p)-2 {
			continue
		}
		if s.Snapshot(p[:n]) {
			t.Fatal("truncated snapshot accepted", n)
		}
	}
	if !reflect.DeepEqual(s.Learned, before) {
		t.Fatal("truncation changed state")
	}
}
func TestCatalogDecodedMetadata(t *testing.T) {
	decoded := make([]byte, 148)
	decoded[100], decoded[101], decoded[105] = 4, 0x12, 5
	raw, _ := json.Marshal(map[string]any{"records": []any{map[string]any{"name": map[string]string{"text": "Flame Attack"}, "description": map[string]string{"text": "Attacks like a flame"}, "fields": map[string]any{"id": 11016, "effect_layer": 1, "table_order": 94, "type": 3}, "decoded_hex": hex.EncodeToString(decoded)}}})
	c, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	d := c.Definitions[11016]
	if d.Icon != 4612 || d.MaximumGrade != 5 || d.Name != "Flame Attack" || d.EffectLayer != 1 || c.Orders[94] != 11016 {
		t.Fatal("decoded metadata", d)
	}
	tail := make([]byte, 198)
	copy(tail[103:], []byte{1, 1, 1, 0, 0})
	raw, _ = json.Marshal(map[string]any{"records": []any{map[string]any{"id": 11016, "decoded_hex": hex.EncodeToString(tail)}}})
	if err = c.ApplyWeapons(raw); err != nil {
		t.Fatal(err)
	}
	if c.Definitions[11016].Weapons != ([5]bool{true, true, true, false, false}) {
		t.Fatal("native weapon flags")
	}
}
func TestInstalledSkillCatalog(t *testing.T) {
	a := login.NewAssets("../../../data")
	if _, err := os.Stat(a.DataPath("skill_data.json")); err != nil {
		t.Skip("assets not installed")
	}
	c, err := Load(a)
	if err != nil {
		t.Fatal(err)
	}
	flame := c.Definitions[11016]
	slow := c.Definitions[11056]
	if flame.EffectLayer != 1 || flame.Icon != 12036 || flame.MaximumGrade != 5 || flame.Weapons != ([5]bool{true, true, true, false, false}) {
		t.Fatal("physical catalog", flame)
	}
	if slow.Weapons != ([5]bool{true, true, true, true, true}) || c.Orders[130] != 11056 {
		t.Fatal("assistant catalog", slow)
	}
}
