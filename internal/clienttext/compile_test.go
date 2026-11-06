package clienttext

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/localization"
)

func TestCompileTextPreservesNativeFieldsAndCustomAnswers(t *testing.T) {
	raw := []byte(`{"source_sha256":"original","records":[{"fields":{"id":57166,"large_value":18446744073709551615},"text":{"text":"Breillat*","length":9},"decoded_hex":"0011aaff"},{"fields":{"id":57171},"text":{"text":"Custom priest*"}},{"fields":{"id":1},"text":{"text":"Breillat*"}}]}`)
	compiled, catalogs, err := CompileLocalizedDocument("talk_data.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"text":"@{talk.57166.text}"`, `"large_value":18446744073709551615`, `"decoded_hex":"0011aaff"`, `"source_sha256":"original"`} {
		if !bytes.Contains(compiled, []byte(want)) {
			t.Fatal("missing or altered preserved field", want, string(compiled))
		}
	}
	for key, want := range map[string]string{"talk.57166.text": "Breillat", "talk.57171.text": "Custom priest*", "talk.1.text": "Breillat*"} {
		if catalogs[localization.English][key] != want {
			t.Fatal("compiled catalog", key, catalogs)
		}
	}
	if _, err = CompileDocument("talk_data.json", compiled); err == nil {
		t.Fatal("keyed input must not overwrite source catalogs")
	}
	// An IGG name identical to the source must suppress lower-priority patches.
	compiled, catalogs, err = CompileLocalizedDocument("npc_data.json", []byte(`{"records":[{"fields":{"id":12073},"name":{"text":"Cool Ronin"},"decoded_hex":"aabb"}]}`))
	if err != nil || catalogs[localization.English]["npc.12073.name"] != "Cool Ronin" {
		t.Fatal("IGG precedence lost", string(compiled), err)
	}
}

func TestCompileLooseTextPreservesExtractionAndHandlesRebuilds(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "talk_data.json")
	raw := []byte(`{"records":[{"fields":{"id":57166},"text":{"text":"Breillat*"}}]}`)
	if err := os.WriteFile(source, raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err := CompileDirectory(root); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(source)
	if !bytes.Equal(raw, before) {
		t.Fatal("native extraction rewritten")
	}
	output := filepath.Join(root, "runtime", "text", "talk_data.json")
	b, _ := os.ReadFile(output)
	var doc map[string]any
	if json.Unmarshal(b, &doc) != nil || !bytes.Contains(b, []byte(`"text":"@{talk.57166.text}"`)) {
		t.Fatal("compiled loose output missing", string(b))
	}
	os.WriteFile(source, []byte(`{"records":[{"fields":{"id":57166},"text":{"text":"Authored replacement*"}}]}`), 0644)
	if err := CompileDirectory(root); err != nil {
		t.Fatal(err)
	}
	catalog, err := localization.Load(root, localization.English)
	if err != nil || catalog.Lookup("talk.57166.text") != "Authored replacement*" {
		t.Fatal("custom text was replaced or stale", string(b))
	}
	os.Remove(source)
	if err := CompileDirectory(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("removed source retained stale compiled text", err)
	}
}

func TestCompileSingleAndDualLanguageSources(t *testing.T) {
	for _, tt := range []struct {
		name, source, original, supplemental, english, chinese string
	}{
		{"Chinese only", "zh-TW", "\u67f4\u72ac", "", "Shiba inu", "\u67f4\u72ac"},
		{"English only", "en", "Shiba inu", "", "Shiba inu", "\u67f4\u72ac"},
		{"both with updated Chinese", "en", "Shiba inu", "\u65b0\u540d\u7a31", "Shiba inu", "\u65b0\u540d\u7a31"},
		{"new Chinese content", "zh-TW", "\u65b0\u540d\u7a31", "", "\u65b0\u540d\u7a31", "\u65b0\u540d\u7a31"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			doc := map[string]any{
				"source_language": tt.source, "source_sha256": "original",
				"records": []any{map[string]any{"fields": map[string]any{"id": 11003, "hp": 123}, "name": map[string]any{"text": tt.original}, "decoded_hex": "aabb"}},
			}
			if tt.supplemental != "" {
				doc["text_languages"] = map[string]any{"zh-TW": map[string]string{"npc.11003.name": tt.supplemental}}
			}
			raw, _ := json.Marshal(doc)
			out, catalogs, err := CompileLocalizedDocument("npc_data.json", raw)
			if err != nil {
				t.Fatal(err)
			}
			if got := catalogs[localization.English]["npc.11003.name"]; got != tt.english {
				t.Fatalf("English: %q, want %q", got, tt.english)
			}
			if got := catalogs[localization.Chinese]["npc.11003.name"]; got != tt.chinese {
				t.Fatalf("Chinese: %q, want %q", got, tt.chinese)
			}
			for _, preserved := range []string{`"hp":123`, `"decoded_hex":"aabb"`, `"source_sha256":"original"`} {
				if !bytes.Contains(out, []byte(preserved)) {
					t.Fatal("native content changed", preserved)
				}
			}
			if bytes.Contains(out, []byte(`"text_languages"`)) {
				t.Fatal("source translation tables leaked into runtime projection")
			}
		})
	}
}
