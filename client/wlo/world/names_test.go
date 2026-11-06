package world

import (
	"os"
	"path/filepath"
	"testing"

	"wonderland-gonline/client/wlo/login"
)

func TestTalksPreservesExtendedOffsetsAndIDPrecedence(t *testing.T) {
	root := t.TempDir()
	// Offset 0x123456 must survive, and the real ID 42 must win over an
	// overlapping offset regardless of record order.
	raw := []byte(`{"records":[
		{"file_offset":1193046,"fields":{"id":42},"text":{"text":"ID and extended offset"}},
		{"file_offset":42,"fields":{"id":99},"text":{"text":"Overlapping offset"}}
	]}`)
	if err := os.WriteFile(filepath.Join(root, "talk_data.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Talks(login.NewAssets(root))
	if err != nil {
		t.Fatal(err)
	}
	if got[0x123456] != "ID and extended offset" || got[42] != "ID and extended offset" || got[99] != "Overlapping offset" {
		t.Fatal(got)
	}
	if _, exists := got[0x3456]; exists {
		t.Fatal("extended reference truncated")
	}
}
