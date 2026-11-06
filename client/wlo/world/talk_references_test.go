package world

import (
	"os"
	"path/filepath"
	"testing"

	"wonderland-gonline/client/wlo/login"
)

func TestTalkReferencesKeepCanonicalIDsAheadOfOffsetAliases(t *testing.T) {
	root := t.TempDir()
	raw := []byte(`{"records":[{"fields":{"id":1},"file_offset":2,"text":{"text":"first"}},{"fields":{"id":2},"file_offset":40,"text":{"text":"second"}}]}`)
	if err := os.WriteFile(filepath.Join(root, "talk_data.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	texts, ids, err := TalksWithReferences(login.NewAssets(root))
	if err != nil {
		t.Fatal(err)
	}
	if texts[2] != "second" || ids[2] != 2 || texts[40] != "second" || ids[40] != 2 || ids[1] != 1 {
		t.Fatal("text/reference alias precedence changed", texts, ids)
	}
}
