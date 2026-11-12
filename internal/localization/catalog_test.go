package localization

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"golang.org/x/text/encoding/traditionalchinese"
)

func TestBaseCatalogsCompleteAndFormattingCompatible(t *testing.T) {
	en, zh := BaseStrings(English), BaseStrings(Chinese)
	// Format specifiers must retain their order and types, including literal %%.
	verbs := regexp.MustCompile(`%[-+# .0-9]*[vTtbcdoOqxXUeEfFgGspw%]`)
	for key, english := range en {
		chinese, ok := zh[key]
		if !ok {
			t.Errorf("missing Chinese key %s", key)
			continue
		}
		if !slices.Equal(verbs.FindAllString(english, -1), verbs.FindAllString(chinese, -1)) {
			t.Errorf("format mismatch %s: %q / %q", key, english, chinese)
		}
	}
	for key := range zh {
		if _, ok := en[key]; !ok {
			t.Errorf("orphan Chinese key %s", key)
		}
	}
}

func TestCatalogResolutionFallbackAndRawMessages(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, filepath.FromSlash(Directory))
	os.MkdirAll(dir, 0755)
	for code, values := range map[string]map[string]string{English: {"talk.1.text": "English line", "talk.2.text": "Fallback"}, Chinese: {"talk.1.text": "中文對話"}} {
		raw, _ := json.Marshal(Document{Version: Version, Language: code, Strings: values})
		if err := os.WriteFile(filepath.Join(dir, code+".json"), raw, 0644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := Load(root, Chinese)
	if err != nil {
		t.Fatal(err)
	}
	if c.Resolve("@{talk.1.text}") != "中文對話" || c.Resolve("@{talk.2.text}") != "Fallback" {
		t.Fatal("lookup/fallback", c)
	}
	for _, literal := range []string{"English line", "Server says English line", "@{talk.1.text} trailing"} {
		if c.Resolve(literal) != literal {
			t.Fatal("literal changed", literal)
		}
	}
	raw := []byte(`{"text":"@{talk.1.text}","native":"aabb","value":18446744073709551615}`)
	expanded, err := c.Expand(raw)
	if err != nil || !bytes.Contains(expanded, []byte("中文對話")) || !bytes.Contains(expanded, []byte("18446744073709551615")) {
		t.Fatal(string(expanded), err)
	}
	old := active.Load()
	t.Cleanup(func() { Install(old) })
	Install(c)
	encoded := UI("talk.1.text")
	decoded, err := traditionalchinese.Big5.NewDecoder().String(encoded)
	if err != nil || decoded != "中文對話" {
		t.Fatal("native font encoding", decoded, err)
	}
	if !errors.Is(Error("same"), Error("same")) {
		t.Fatal("keyed errors lost identity")
	}
}

func TestInvalidCatalogFailsInsteadOfSilentlyFallingBack(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, filepath.FromSlash(Directory))
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, Chinese+".json"), []byte(`{"version":999}`), 0644)
	if _, err := Load(root, Chinese); err == nil {
		t.Fatal("corrupt catalog accepted")
	}
	if _, err := Normalize("unknown"); err == nil {
		t.Fatal("unsupported language accepted")
	}
}

func TestClientReferencesHaveCatalogEntries(t *testing.T) {
	en := BaseStrings(English)
	keys := regexp.MustCompile(`"(ui\.[a-zA-Z0-9_.]+)"`)
	// API command names share the ui.* namespace but are not display text.
	actionNames := regexp.MustCompile(`agentActionDef\(\s*"(ui\.[a-zA-Z0-9_.]+)"`)
	commands := map[string]bool{}
	references := map[string][]string{}
	err := filepath.WalkDir(filepath.Join("..", "..", "client", "wlo"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range actionNames.FindAllSubmatch(raw, -1) {
			commands[string(match[1])] = true
		}
		for _, match := range keys.FindAllSubmatch(raw, -1) {
			references[string(match[1])] = append(references[string(match[1])], path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for key, paths := range references {
		if _, ok := en[key]; !ok && !commands[key] {
			t.Errorf("%v references missing key %s", paths, key)
		}
	}
}
