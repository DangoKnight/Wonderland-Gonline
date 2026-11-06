// Package localization resolves stable text keys. It never matches or replaces
// literal server messages or player-entered strings.
package localization

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/traditionalchinese"
	"wonderland-gonline/internal/clientfs"
)

const (
	English   = "en"
	Chinese   = "zh-TW"
	Directory = "runtime/locales"
	Version   = 1
)

type Document struct {
	Version  int               `json:"version"`
	Language string            `json:"language"`
	Strings  map[string]string `json:"strings"`
}
type Catalog struct {
	Root, Language    string
	Strings, Fallback map[string]string
}

//go:embed ui_en.json
var baseEnglish []byte

//go:embed ui_zh-TW.json
var baseChinese []byte

func Normalize(language string) (string, error) {
	switch strings.ToLower(language) {
	case "", "en", "en-us", "english":
		return English, nil
	case "zh", "zh-tw", "zh-hant", "chinese":
		return Chinese, nil
	}
	return "", fmt.Errorf("unsupported language %q", language)
}
func parse(raw []byte) (Document, error) {
	var d Document
	if err := json.Unmarshal(raw, &d); err != nil {
		return d, err
	}
	if d.Version != Version || d.Strings == nil {
		return d, fmt.Errorf("unsupported language catalog")
	}
	return d, nil
}
func base(language string) map[string]string {
	raw := baseEnglish
	if language == Chinese {
		raw = baseChinese
	}
	d, err := parse(raw)
	if err != nil {
		panic(err)
	}
	return d.Strings
}
func BaseDocument(language string) []byte {
	if language == Chinese {
		return baseChinese
	}
	return baseEnglish
}
func BaseStrings(language string) map[string]string { return base(language) }
func Load(root, language string) (*Catalog, error) {
	language, err := Normalize(language)
	if err != nil {
		return nil, err
	}
	c := &Catalog{Root: root, Language: language, Strings: base(language), Fallback: base(English)}
	codes := []string{English}
	if language != English {
		codes = append(codes, language)
	}
	for _, code := range codes {
		raw, err := clientfs.ReadFile(filepath.Join(root, filepath.FromSlash(Directory), code+".json"))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		d, err := parse(raw)
		if err != nil {
			return nil, err
		}
		if d.Language != code {
			return nil, fmt.Errorf("language catalog identity mismatch")
		}
		target := c.Strings
		if code == English && language != English {
			target = c.Fallback
		}
		for k, v := range d.Strings {
			target[k] = v
		}
	}
	return c, nil
}
func (c *Catalog) Lookup(key string) string {
	if value, ok := c.Find(key); ok {
		return value
	}
	return key
}
func (c *Catalog) Find(key string) (string, bool) {
	if s, ok := c.Strings[key]; ok {
		return s, true
	}
	if s, ok := c.Fallback[key]; ok {
		return s, true
	}
	return "", false
}
func Key(table string, id uint32, field string) string {
	return fmt.Sprintf("%s.%d.%s", table, id, field)
}
func Ref(key string) string { return "@{" + key + "}" }
func (c *Catalog) Resolve(value string) string {
	if strings.HasPrefix(value, "@{") && strings.HasSuffix(value, "}") {
		return c.Lookup(value[2 : len(value)-1])
	}
	return value
}

var active atomic.Pointer[Catalog]
var callbacks struct {
	sync.Mutex
	values []func()
}

func init() {
	active.Store(&Catalog{Language: English, Strings: base(English), Fallback: base(English)})
}
func Install(c *Catalog) {
	active.Store(c)
	callbacks.Lock()
	defer callbacks.Unlock()
	for _, f := range callbacks.values {
		f()
	}
}
func Register(f func()) {
	callbacks.Lock()
	callbacks.values = append(callbacks.values, f)
	callbacks.Unlock()
}
func Text(key string) string      { return active.Load().Lookup(key) }
func Resolve(value string) string { return active.Load().Resolve(value) }
func UI(key string) string {
	b, err := encoding.ReplaceUnsupported(traditionalchinese.Big5.NewEncoder()).String(Text(key))
	if err != nil {
		return Text(key)
	}
	return b
}
func CurrentLanguage() string { return active.Load().Language }

type keyedError string

func (e keyedError) Error() string { return UI(string(e)) }

// Comparable keys preserve errors.Is identity across language changes.
func Error(key string) error                   { return keyedError(key) }
func Format(format string, args ...any) string { return fmt.Sprintf(Resolve(format), args...) }

// ExpandDocument resolves explicit asset references once, before typed loading.
// Raw legacy exports and network packets never undergo source-string matching.
func ExpandDocument(raw []byte) ([]byte, error) {
	return active.Load().Expand(raw)
}
func ReadAsset(path string) ([]byte, error) {
	raw, err := clientfs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !bytes.Contains(raw, []byte(`"@{`)) {
		return raw, nil
	}
	root := filepath.Dir(path)
	if filepath.Base(root) == "text" && filepath.Base(filepath.Dir(root)) == "runtime" {
		root = filepath.Dir(filepath.Dir(root))
	}
	c := active.Load()
	if filepath.Clean(c.Root) != filepath.Clean(root) {
		c, err = Load(root, c.Language)
		if err != nil {
			return nil, err
		}
	}
	return c.Expand(raw)
}
func (c *Catalog) Expand(raw []byte) ([]byte, error) {
	if !bytes.Contains(raw, []byte(`"@{`)) {
		return raw, nil
	}
	var value any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&value); err != nil {
		return nil, err
	}
	var walk func(any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case string:
			return c.Resolve(x)
		case map[string]any:
			for k, v := range x {
				x[k] = walk(v)
			}
		case []any:
			for i, v := range x {
				x[i] = walk(v)
			}
		}
		return v
	}
	return json.Marshal(walk(value))
}
