package login

import "wonderland-gonline/internal/localization"

import (
	"encoding/json"
	"errors"
	"image"
	"path/filepath"
	"strings"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/clientimage"
	"wonderland-gonline/internal/clientruntime"
)

// Assets locates the decompiled data root directly.
// Media and sprites are resolved inside that one root.
type Assets struct {
	Root     string
	Media    string
	Data     string
	UserRoot string
}

const PicturesDir = "pictures"

func NewAssets(root string) Assets {
	return Assets{Root: root, Media: filepath.Join(root, "media"), Data: root}
}
func (a Assets) MediaPath(parts ...string) string { return Path(a.Media, parts...) }
func (a Assets) UserPath(name string) string {
	if a.UserRoot != "" {
		return Path(a.UserRoot, "user", name)
	}
	return Path(filepath.Join(filepath.Dir(a.Root), "var", "client"), "user", name)
}
func (a Assets) SkinPicture(parts ...string) string {
	path := append([]string(nil), parts...)
	path[len(path)-1] += ".png"
	if len(path) >= 3 && strings.EqualFold(path[0], "menu") && strings.EqualFold(path[1], "skins") {
		for _, root := range a.UISkinRoots(localization.CurrentLanguage()) {
			candidate := Path(root, path[2:]...)
			if _, err := clientfs.Stat(candidate); err == nil {
				return candidate
			}
			if _, err := clientfs.Stat(candidate + clientimage.DescriptorSuffix); err == nil {
				return candidate
			}
		}
	}
	return a.MediaPath(path...)
}

// UI artwork availability is independent of the selected text catalog. Common
// skins are the legacy English layout; either locale can stand alone.
func (a Assets) UISkinRoots(language string) []string {
	en := a.MediaPath("locales", localization.English, "menu", "Skins")
	zh := a.MediaPath("locales", localization.Chinese, "menu", "Skins")
	common := a.MediaPath("menu", "Skins")
	code, _ := localization.Normalize(language)
	if code == localization.Chinese {
		return []string{zh, en, common}
	}
	return []string{en, common, zh}
}

// DataPath is a file of the extracted data.
func (a Assets) DataPath(parts ...string) string {
	if len(parts) == 1 {
		if compiled, ok := clientruntime.TextPath(parts[0]); ok {
			path := Path(a.Data, compiled)
			if _, err := clientfs.Stat(path); err == nil {
				return path
			}
		}
	}
	return Path(a.Data, parts...)
}

// Picture returns a logical PNG path for compatibility. Use LoadPicture to
// resolve images that now occupy rectangles on shared atlas pages.
func (a Assets) Picture(parts ...string) string {
	p := append([]string{PicturesDir}, parts...)
	p[len(p)-1] += ".png"
	return Path(a.Data, p...)
}

// LoadPicture resolves a loose picture or a rectangle on an exported atlas.
func (a Assets) LoadPicture(parts ...string) (*image.NRGBA, error) {
	path := append([]string(nil), parts...)
	path[len(path)-1] += ".png"
	return clientassets.LoadPicture(Path(a.Data, PicturesDir), filepath.ToSlash(filepath.Join(path...)))
}

// CompiledPicture returns a lazily decoded view, preserving atlas rectangles.
func (a Assets) CompiledPicture(parts ...string) (*clientimage.Image, error) {
	path := append([]string(nil), parts...)
	path[len(path)-1] += ".png"
	return clientassets.CompiledPicture(Path(a.Data, PicturesDir), filepath.ToSlash(filepath.Join(path...)))
}

// exportHeader is the leading part of an extracted record export.
type exportHeader struct {
	Format      string `json:"format"`
	SourceBytes int64  `json:"source_bytes"`
	DecodedHex  string `json:"decoded_hex"`
}

// readExportHeader decodes an export's top-level fields up to the first
// large array, which it skips without loading.
func readExportHeader(path string, stop string) (exportHeader, error) {
	var h exportHeader
	f, err := clientfs.Open(path)
	if err != nil {
		return h, err
	}
	defer f.Close()
	d := json.NewDecoder(f)
	if t, err := d.Token(); err != nil || t != json.Delim('{') {
		return h, errors.New(path + localization.UI("ui.login.not_an_object_e42945a8"))
	}
	for d.More() {
		t, err := d.Token()
		if err != nil {
			return h, err
		}
		key, _ := t.(string)
		if key == stop {
			break
		}
		switch key {
		case "format":
			err = d.Decode(&h.Format)
		case "source_bytes":
			err = d.Decode(&h.SourceBytes)
		case "decoded_hex":
			err = d.Decode(&h.DecodedHex)
		default:
			var skip json.RawMessage
			err = d.Decode(&skip)
		}
		if err != nil {
			return h, err
		}
	}
	return h, nil
}
