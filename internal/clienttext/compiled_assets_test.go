package clienttext

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"wonderland-gonline/internal/localization"
)

// These checks run from a clean checkout without the ignored native exports.
// Keep explicit content expectations independent of the compiler's matching code.
func TestTrackedStringAssets(t *testing.T) {
	for _, code := range []string{localization.English, localization.Chinese} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "data", filepath.FromSlash(localization.Directory), code+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if !utf8.Valid(raw) {
			t.Fatalf("%s: invalid UTF-8", code)
		}
		var doc localization.Document
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		if doc.Version != localization.Version || doc.Language != code {
			t.Fatal("catalog identity mismatch", code)
		}
		canonical, err := marshalCatalog(doc)
		if err != nil || !bytes.Equal(raw, canonical) {
			t.Fatalf("%s is not canonically formatted; regenerate with asset-build -text-only", code)
		}
		for key, want := range localization.BaseStrings(code) {
			if doc.Strings[key] != want {
				t.Errorf("%s: stale UI entry %s; regenerate compiled catalogs", code, key)
			}
		}
		for _, domain := range []string{"ui.", "npc.", "item.", "skill.", "scene.", "talk.", "mark."} {
			found := false
			for key := range doc.Strings {
				if strings.HasPrefix(key, domain) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: missing %s text", code, domain)
			}
		}
		want := map[string]string{"npc.10000.name": "Breillat", "item.10034.name": "Pure Iron Knife", "talk.57166.text": "Breillat", "talk.57171.text": "Priest in church"}
		if code == localization.Chinese {
			want = map[string]string{"npc.10000.name": "布蕾雅", "item.10034.name": "赤鐵刀", "talk.57166.text": "布蕾雅", "talk.57171.text": "神父"}
		}
		for key, text := range want {
			if doc.Strings[key] != text {
				t.Errorf("%s: %s = %q, want %q", code, key, doc.Strings[key], text)
			}
		}
		t.Logf("%s: %d compiled strings", code, len(doc.Strings))
	}
}
