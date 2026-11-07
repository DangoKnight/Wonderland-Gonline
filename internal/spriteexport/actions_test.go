package spriteexport

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/spritepack"
)

func TestActionTablePreservesShortSourceAndRejectsMalformed(t *testing.T) {
	// Independent native header declares one 80-byte record; the archive has
	// two sprites. Neither the header nor record may be padded during export.
	header, _ := hex.DecodeString("6a78000000000000000001000000")
	record := make([]byte, 80)
	record[0], record[79] = 2, 7
	raw := append(append([]byte{}, header...), record...)
	for _, sprites := range []int{1, 2} {
		doc, missing, err := decodeActions("source.jxa", raw, sprites)
		if err != nil || missing != sprites-1 {
			t.Fatalf("short/exact table: %v, %d", err, missing)
		}
		rows := doc["records"].([]any)
		if len(rows) != 1 || rows[0].(map[string]any)["action_frame_counts"].([]int)[79] != 7 {
			t.Fatal("source actions changed")
		}
		rebuiltHeader, _ := hex.DecodeString(doc["header_hex"].(string))
		rebuiltRecord, _ := hex.DecodeString(rows[0].(map[string]any)["record_hex"].(string))
		if !bytes.Equal(append(rebuiltHeader, rebuiltRecord...), raw) {
			t.Fatal("native source reconstruction differs")
		}
	}
	for _, damaged := range [][]byte{raw[:13], raw[:len(raw)-1], append(append([]byte{}, raw...), 0)} {
		if _, _, err := decodeActions("bad.jxa", damaged, 2); err == nil {
			t.Fatal("malformed action table accepted")
		}
	}
	if _, _, err := decodeActions("excess.jxa", raw, 0); err == nil {
		t.Fatal("actions without archive sprites accepted")
	}
}

func TestEditableExportPreservesSpriteWithoutActionRecord(t *testing.T) {
	header, _ := hex.DecodeString("6a61000000000000000001000000" + "05612e6a6d7000000000000000000000000000003a0400002a000000")
	payload, _ := hex.DecodeString("6a6d000000000000000001000000")
	frame := make([]byte, 40)
	frame[0], frame[1] = 1, 'f'
	binary.LittleEndian.PutUint16(frame[12:], 8)
	binary.LittleEndian.PutUint16(frame[14:], 8)
	binary.LittleEndian.PutUint16(frame[16:], 2)
	binary.LittleEndian.PutUint16(frame[18:], 3)
	binary.LittleEndian.PutUint32(frame[20:], 4)
	binary.LittleEndian.PutUint32(frame[24:], 1078)
	binary.LittleEndian.PutUint32(frame[28:], 1)
	binary.LittleEndian.PutUint32(frame[32:], 1)
	palette := make([]byte, 1024)
	palette[4], palette[5], palette[6] = 0x12, 0x34, 0x56
	payload = append(append(append(payload, frame...), palette...), 1, 0, 0, 0)
	raw := append(header, payload...)
	root := t.TempDir()
	source := filepath.Join(root, "source.jma")
	if err := os.WriteFile(source, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := exportArchive(source, filepath.Join(root, "output", "archive"), filepath.Join(root, "output"), nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := jsonWrite(filepath.Join(root, "output", "archive", "actions.json"), map[string]any{"records": []any{}}); err != nil {
		t.Fatal(err)
	}
	if err := exportEditable(filepath.Join(root, "output"), "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := spritepack.OptimizeEditable(filepath.Join(root, "output", "archive"), "archive"); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Sprites []struct {
			Name       string `json:"name"`
			Animations []any  `json:"animations"`
			Frames     []any  `json:"frames"`
		} `json:"sprites"`
	}
	if err := readJSON(filepath.Join(root, "output", "archive", "editable.json"), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Sprites) != 1 || doc.Sprites[0].Name != "a.jmp" || len(doc.Sprites[0].Animations) != 0 || len(doc.Sprites[0].Frames) != 1 {
		t.Fatal("unindexed sprite lost or invented animations")
	}
	if err := verifyEditable(filepath.Join(root, "output"), "", true); err != nil {
		t.Fatal(err)
	}
}
