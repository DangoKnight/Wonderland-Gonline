package assets

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/game"
)

// Fixed artificial records verify every decoded byte, independently of the
// production field-offset tables. No original game data is committed.
func nativeItemGolden(t *testing.T) ([]byte, []byte) {
	t.Helper()
	data, err := os.ReadFile("testdata/native_item_record.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Encrypted string `json:"encrypted_hex"`
		Decoded   string `json:"decoded_hex"`
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	encrypted, err := hex.DecodeString(fixture.Encrypted)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := hex.DecodeString(fixture.Decoded)
	if err != nil {
		t.Fatal(err)
	}
	header := make([]byte, 451)
	header[114] = 9
	return append(header, encrypted...), decoded
}

func TestNativeItemCompleteGoldenRecord(t *testing.T) {
	data, decoded := nativeItemGolden(t)
	before := bytes.Clone(data)
	items, err := ParseNativeItems(data)
	if err != nil {
		t.Fatal(err)
	}
	v := items[10002]
	if len(items) != 1 || !bytes.Equal(v.Record[:], decoded) || !bytes.Equal(data, before) {
		t.Fatal("full decryption or source isolation failed")
	}
	if v.Definition != (game.ItemDefinition{CellWidth: 187, CellHeight: 224, ID: 10002, Name: "測試Item", Type: 4, EquipSlot: 3, Level: 12, Status: [2]uint16{210, 214}, Values: [2]int32{120, -2}}) || v.Description != "中文 description" || v.Icon != 525 || v.LargeIcon != 1000 || v.Sprites != [4]uint16{10, 20, 30, 40} {
		t.Fatal(v)
	}
	// Zero IDs are decoded then skipped. The first duplicate wins.
	zero := bytes.Clone(data[451:])
	zero[16] = 0xca
	zero[17] = 0xef // encoded zero = 9 XOR EFC3
	duplicate := bytes.Clone(data[451:])
	duplicate[15] = 0x90 // encoded type 1
	data = append(append(data, zero...), duplicate...)
	items, err = ParseNativeItems(data)
	if err != nil || len(items) != 1 || items[10002].Definition.Type != 4 {
		t.Fatal("lookup precedence", items, err)
	}
}

func TestNativeItemRejectsIncompleteOrUnsupportedData(t *testing.T) {
	data, _ := nativeItemGolden(t)
	for _, n := range []int{0, 1, 450, 451, 452, 901} {
		if _, err := ParseNativeItems(data[:n]); err == nil {
			t.Fatalf("accepted length %d", n)
		}
	}
	for _, version := range []byte{0, 1, 8, 10, 255} {
		bad := bytes.Clone(data)
		bad[114] = version
		if _, err := ParseNativeItems(bad); err == nil {
			t.Fatal("accepted version", version)
		}
	}
	for _, at := range []int{451, 451 + 146} {
		bad := bytes.Clone(data)
		bad[at] = 255
		if _, err := ParseNativeItems(bad); err == nil {
			t.Fatal("accepted bad text length", at)
		}
	}
	// Validate the entire table before returning any definitions.
	bad := append(bytes.Clone(data), data[451:]...)
	bad[902] = 15
	if items, err := ParseNativeItems(bad); err == nil || items != nil {
		t.Fatal("partially published corrupt table")
	}
}

func TestNativeItemCatalogCensusAndDecodedHash(t *testing.T) {
	dir := os.Getenv("WONDERLAND_TEST_DATA")
	if dir == "" {
		t.Skip("set WONDERLAND_TEST_DATA for native item checks")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join("../..", dir)
	}
	data, err := os.ReadFile(filepath.Join(dir, "Item.dat"))
	if err != nil {
		t.Fatal(err)
	}
	items, err := ParseNativeItems(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 7243 {
		t.Fatal("native census", len(items))
	}
	// Hash all decoded fields in original file order against an independent
	// reference extraction from the native client's in-memory offsets.
	h := sha256.New()
	for off := 451; off < len(data); off += 451 {
		id := (le.Uint16(data[off+16:]) ^ 0xefc3) - 9
		record := items[id].Record
		h.Write(record[:])
	}
	if got := fmt.Sprintf("%x", h.Sum(nil)); got != "47ac57137e64ab78a646e62a92336848e82ee7cb26eb7f83cb980a1c27d3898f" {
		t.Fatal("native decoded hash", got)
	}
	if items[10002].Definition.Name != "Plate" || items[27025].Definition.Type != 20 || items[27025].Description == "" {
		t.Fatal("native projection")
	}
}

func FuzzNativeItems(f *testing.F) {
	f.Add([]byte{})
	header := make([]byte, 902)
	header[114] = 9
	f.Add(header)
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		ParseNativeItems(data)
	})
}

// The translated client ships a different catalog. Validate it separately from
// the server-data census when its directory is supplied.
func TestTranslatedClientNativeItemCatalog(t *testing.T) {
	dir := os.Getenv("WONDERLAND_TEST_CLIENT_DATA")
	if dir == "" {
		t.Skip("set WONDERLAND_TEST_CLIENT_DATA for the translated client catalog")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join("../..", dir)
	}
	data, err := os.ReadFile(filepath.Join(dir, "Item.dat"))
	if err != nil {
		t.Fatal(err)
	}
	items, err := ParseNativeItems(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 7312 {
		t.Fatal("translated client census", len(items))
	}
	h := sha256.New()
	for off := 451; off < len(data); off += 451 {
		id := (le.Uint16(data[off+16:]) ^ 0xefc3) - 9
		record := items[id].Record
		h.Write(record[:])
	}
	if got := fmt.Sprintf("%x", h.Sum(nil)); got != "105274f6b0e8f063d18ebb00cda3f37c383e5b44145013dd66afec12eea1f84d" {
		t.Fatal("translated catalog decoded hash", got)
	}
}
