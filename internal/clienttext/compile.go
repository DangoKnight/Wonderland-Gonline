package clienttext

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/clientruntime"
	"wonderland-gonline/internal/localization"
)

var exports = map[string]Table{
	"npc_data.json": NPC, "item_data.json": Item, "skill_data.json": Skill,
	"scene_data.json": Scene, "talk_data.json": Talk, "mark_data.json": Mark,
}

const textCompilerVersion = 3

func IsExport(name string) bool { _, ok := exports[name]; return ok }

// Fingerprint invalidates compiled text when translation data or compilation
// semantics change, without invalidating unrelated image and sprite outputs.
func Fingerprint() string {
	npc, err := assets.NPCNamePatches()
	if err != nil {
		panic(err)
	}
	items, err := assets.ItemNamePatches()
	if err != nil {
		panic(err)
	}
	n, _ := json.Marshal(npc)
	i, _ := json.Marshal(items)
	h := sha256.New()
	fmt.Fprintf(h, "client-text-v%d\n%s\n%s", textCompilerVersion, iggCSV, dialogueCSV)
	h.Write([]byte(chineseCSV))
	h.Write(localization.BaseDocument(localization.English))
	h.Write(localization.BaseDocument(localization.Chinese))
	h.Write(n)
	h.Write(i)
	return hex.EncodeToString(h.Sum(nil))
}

func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func recordID(m map[string]any) uint32 {
	n, ok := m["id"].(json.Number)
	if !ok {
		return 0
	}
	id, err := strconv.ParseUint(string(n), 10, 32)
	if err != nil {
		return 0
	}
	return uint32(id)
}

// CompileDocument changes text fields only. Unknown fields, exact numeric values,
// decoded native bytes, IDs and gameplay definitions are preserved.
func CompileDocument(name string, raw []byte) ([]byte, error) {
	out, _, err := CompileLocalizedDocument(name, raw)
	return out, err
}

func CompileLocalizedDocument(name string, raw []byte) ([]byte, map[string]map[string]string, error) {
	table, ok := exports[name]
	if !ok {
		return raw, nil, nil
	}
	if bytes.Contains(raw, []byte(`"@{`)) {
		return nil, nil, fmt.Errorf("compile text %s: expected original export, received keyed projection", name)
	}
	var doc map[string]any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&doc); err != nil {
		return nil, nil, fmt.Errorf("compile text %s: %w", name, err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, nil, fmt.Errorf("compile text %s: trailing JSON data", name)
	}
	sourceLanguage, _ := doc["source_language"].(string)
	if sourceLanguage == "" {
		sourceLanguage = localization.English // Compatibility with earlier exports.
	}
	if sourceLanguage != localization.English && sourceLanguage != localization.Chinese {
		return nil, nil, fmt.Errorf("compile text %s: unsupported source language %q", name, sourceLanguage)
	}
	var sourceTexts map[string]map[string]string
	if value, ok := doc["text_languages"]; ok {
		encoded, _ := json.Marshal(value)
		if err := json.Unmarshal(encoded, &sourceTexts); err != nil {
			return nil, nil, fmt.Errorf("compile text %s: invalid language sources: %w", name, err)
		}
		for code := range sourceTexts {
			if code != localization.English && code != localization.Chinese {
				return nil, nil, fmt.Errorf("compile text %s: unsupported language source %q", name, code)
			}
		}
	}
	npc, err := assets.NPCNamePatches()
	if err != nil {
		return nil, nil, err
	}
	items, err := assets.ItemNamePatches()
	if err != nil {
		return nil, nil, err
	}
	catalogs := map[string]map[string]string{localization.English: {}, localization.Chinese: {}}
	collection := "records"
	if table == Item {
		collection = "items"
	}
	rows, _ := doc[collection].([]any)
	for _, value := range rows {
		r := object(value)
		fields := object(r["fields"])
		if table == Item {
			fields = object(r["definition"])
		}
		id := recordID(fields)
		if id == 0 {
			continue
		}
		apply := func(field, original string) string {
			lookupKey := localization.Key(string(table), id, field)
			english := original
			if sourceLanguage == localization.Chinese {
				// Reuse only a known Chinese match, never an ID alone. New or
				// edited untranslated content falls back to the installed source.
				if zh, ok := chineseText()[lookupKey]; ok && original == zh {
					if p, ok := englishOverrides()[key{table, id, field}]; ok {
						english = p.original
					}
				}
			}
			if supplied, ok := sourceTexts[localization.English][lookupKey]; ok {
				english = supplied
			}
			text, matched := Resolve(table, id, field, english)
			if !matched && field == Name && id <= math.MaxUint16 {
				switch table {
				case NPC:
					text = npc[uint16(id)].Apply(english)
				case Item:
					if p, ok := items[uint16(id)]; ok && english == p.OriginalName {
						text = p.Name
					}
				}
			}
			catalogs[localization.English][lookupKey] = text
			if sourceLanguage == localization.Chinese {
				catalogs[localization.Chinese][lookupKey] = original
			} else if zh, ok := sourceTexts[localization.Chinese][lookupKey]; ok {
				catalogs[localization.Chinese][lookupKey] = zh
			} else if zh, ok := chineseText()[lookupKey]; ok {
				catalogs[localization.Chinese][lookupKey] = zh
			}
			return localization.Ref(lookupKey)
		}
		if table == Item {
			if text, ok := fields["name"].(string); ok {
				fields["name"] = apply(Name, text)
			}
			if text, ok := r["description"].(string); ok {
				r["description"] = apply(Description, text)
			}
			continue
		}
		for _, field := range []string{Name, Description, Text, "secondary_name"} {
			part := object(r[field])
			original, ok := part["text"].(string)
			if !ok {
				continue
			}
			text := apply(field, original)
			if text == original {
				continue
			}
			part["text"] = text
			if _, ok := part["length"]; ok {
				part["length"] = len([]byte(text))
			}
			if _, ok := part["encoding"]; ok {
				part["encoding"] = "utf-8"
			}
		}
	}
	// Source annotations are build inputs, not runtime translation tables.
	delete(doc, "text_languages")
	delete(doc, "text_language_sources")
	out, err := json.Marshal(doc)
	return out, catalogs, err
}

// CompileDirectory emits keyed runtime documents and language catalogs. Original
// extraction files and their source manifests are never modified. Missing
// optional exports remove stale projections from an earlier extraction.
func CompileDirectory(root string) error {
	outputs := map[string][]byte{}
	catalogs := NewCatalogs()
	for name := range exports {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		out, texts, err := CompileLocalizedDocument(name, raw)
		if err != nil {
			return err
		}
		outputs[name] = out
		MergeCatalogs(catalogs, texts)
	}
	for name := range exports {
		rel, _ := clientruntime.TextPath(name)
		path := filepath.Join(root, rel)
		raw, exists := outputs[name]
		if !exists {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if previous, err := os.ReadFile(path); err == nil && bytes.Equal(previous, raw) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := writeText(path, raw); err != nil {
			return err
		}
	}
	for code, strings := range catalogs {
		raw, err := marshalCatalog(localization.Document{Version: localization.Version, Language: code, Strings: strings})
		if err != nil {
			return err
		}
		path := filepath.Join(root, filepath.FromSlash(localization.Directory), code+".json")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, raw) {
			continue
		}
		if err := writeText(path, raw); err != nil {
			return err
		}
	}
	return nil
}

// Keep compiled string assets readable and stable for repository review. Bundle
// packaging may minify them without changing their keys or values.
func marshalCatalog(catalog localization.Document) ([]byte, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(catalog); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func writeText(path string, raw []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".text-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
