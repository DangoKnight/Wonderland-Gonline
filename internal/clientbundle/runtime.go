package clientbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"wonderland-gonline/internal/clientimage"
	"wonderland-gonline/internal/clientruntime"
	"wonderland-gonline/internal/spritepack"
)

const PacksName = "_packs.json"
const runtimeCompilerVersion = 2
const runtimePackDescriptorVersion = 1
const compiledImageCodecVersion = 2
const compiledGroundMaxGridSide = 900

type runtimeCacheEntry struct {
	Signature  string
	ImageCodec int
	Outputs    map[string]string
}
type runtimeCompiler struct {
	source, root    string
	imageDimensions map[string]Dimensions
	recompressed    map[string]optimizedImagePage
	files           []string
	wanted          map[string]bool
	old, next       map[string]runtimeCacheEntry
}

// BuildRuntime compiles heavy exports into independently readable binary records
// and writes four independently replaceable packs. Output is the core pack.
func BuildRuntime(o Options) (Result, error) {
	absolute, err := filepath.Abs(o.Source)
	if err != nil {
		return Result{}, err
	}
	cache := filepath.Join(filepath.Dir(o.Output), ".client-runtime-"+digest([]byte(absolute))[:12])
	c := runtimeCompiler{source: o.Source, root: cache, wanted: map[string]bool{}, old: map[string]runtimeCacheEntry{}, next: map[string]runtimeCacheEntry{}}
	if b, e := os.ReadFile(filepath.Join(cache, ".cache.json")); e == nil {
		_ = json.Unmarshal(b, &c.old)
	}
	err = filepath.WalkDir(o.Source, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("asset symlink: %s", path)
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(o.Source, path)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		c.files = append(c.files, rel)
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	sort.Strings(c.files)
	if err = c.validateImages(o); err != nil {
		return Result{}, err
	}
	for _, name := range c.files {
		if !runtimeSource(name) {
			continue
		}
		if err := c.link(name); err != nil {
			return Result{}, err
		}
	}
	if err = c.compileImages(); err != nil {
		return Result{}, err
	}
	if contains(c.files, "ground_data.json") {
		if err = c.generate("ground", []string{"ground_data.json"}, func(write func(string, any) error) error {
			var doc struct {
				Entries []clientruntime.Ground `json:"entries"`
			}
			if e := c.readJSON("ground_data.json", &doc); e != nil {
				return e
			}
			seen := map[uint16]bool{}
			for _, g := range doc.Entries {
				id, e := strconv.ParseUint(strings.TrimSuffix(strings.ToLower(g.Name), ".map"), 10, 16)
				if e != nil {
					return e
				}
				if seen[uint16(id)] {
					return fmt.Errorf("duplicate terrain map %d", id)
				}
				seen[uint16(id)] = true
				t := &g.Terrain
				t.Cells, e = hex.DecodeString(t.CellsHex)
				if e != nil {
					return e
				}
				t.CellsHex = ""
				if t.Width == 0 || t.Height == 0 || t.GridWidth == 0 || t.GridHeight == 0 || t.GridWidth > compiledGroundMaxGridSide || t.GridHeight > compiledGroundMaxGridSide || len(t.Cells) != int(t.GridWidth)*int(t.GridHeight) {
					return fmt.Errorf("invalid terrain %s", g.Name)
				}
				if e = write(filepath.ToSlash(clientruntime.MapPath(uint16(id))), g); e != nil {
					return e
				}
			}
			return nil
		}); err != nil {
			return Result{}, err
		}
	}
	if contains(c.files, "eve_data.json") {
		if err = c.generate("events", []string{"eve_data.json"}, func(write func(string, any) error) error {
			var doc struct {
				Maps []struct {
					ID    uint16 `json:"id"`
					Scene uint16 `json:"scene"`
					Hex   string `json:"decoded_hex"`
				} `json:"maps"`
			}
			if e := c.readJSON("eve_data.json", &doc); e != nil {
				return e
			}
			index := map[uint16]uint16{}
			for _, m := range doc.Maps {
				if _, ok := index[m.ID]; ok {
					return fmt.Errorf("duplicate event map %d", m.ID)
				}
				index[m.ID] = m.Scene
				b, e := hex.DecodeString(m.Hex)
				if e != nil {
					return e
				}
				if e = write(filepath.ToSlash(clientruntime.EventPath(m.ID)), b); e != nil {
					return e
				}
			}
			return write(clientruntime.EventIndex, index)
		}); err != nil {
			return Result{}, err
		}
	}
	if contains(c.files, "wem_data.json") {
		if err = c.generate("objects", []string{"wem_data.json"}, func(write func(string, any) error) error {
			var doc struct {
				Entries []struct {
					Name string `json:"name"`
					Hex  string `json:"decoded_hex"`
				} `json:"entries"`
			}
			if e := c.readJSON("wem_data.json", &doc); e != nil {
				return e
			}
			records := map[uint32][]byte{}
			for _, r := range doc.Entries {
				id, e := strconv.ParseUint(strings.TrimSuffix(strings.ToLower(r.Name), ".wem"), 10, 32)
				if e != nil {
					return e
				}
				b, e := hex.DecodeString(r.Hex)
				if e != nil {
					return e
				}
				records[uint32(id)] = b
			}
			return write(clientruntime.ObjectsFile, records)
		}); err != nil {
			return Result{}, err
		}
	}
	for _, name := range c.files {
		if !strings.HasPrefix(name, "sprites/") || filepath.Base(name) != spritepack.EditableFile {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(name))
		deps := []string{}
		for _, file := range c.files {
			if strings.HasPrefix(file, dir+"/") && (strings.HasSuffix(file, ".png") || file == name || filepath.Base(file) == spritepack.NativeFile) {
				deps = append(deps, file)
			}
		}
		if contains(c.files, "sprites/"+spritepack.ManifestFile) {
			deps = append(deps, "sprites/"+spritepack.ManifestFile)
		}
		if err = c.generate(dir, deps, func(write func(string, any) error) error {
			return spritepack.CompileRuntime(filepath.Join(c.source, filepath.FromSlash(dir)), filepath.Base(dir), func(path string, v any) error { return write(dir+"/"+filepath.ToSlash(path), v) })
		}); err != nil {
			return Result{}, err
		}
	}
	// Remove stale outputs after successful source compilation, never from data/.
	err = filepath.WalkDir(cache, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(cache, path)
		if e != nil {
			return e
		}
		if !c.wanted[filepath.ToSlash(rel)] {
			return os.Remove(path)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return Result{}, err
	}
	cacheRaw, err := json.Marshal(c.next)
	if err != nil {
		return Result{}, err
	}
	if err = writeAtomic(filepath.Join(cache, ".cache.json"), cacheRaw); err != nil {
		return Result{}, err
	}
	base := strings.TrimSuffix(o.Output, filepath.Ext(o.Output))
	packPaths := []string{base + "-world.build.zip", base + "-sprites.build.zip", base + "-audio.build.zip"}
	descriptor := struct {
		Version int      `json:"version"`
		Packs   []string `json:"packs"`
	}{runtimePackDescriptorVersion, []string{}}
	var result Result
	// Companion packs first; core is published after all builds succeed.
	for i, group := range []string{"world", "sprites", "audio"} {
		part := o
		part.Source = cache
		part.Output = packPaths[i]
		part.group = group
		r, e := Build(part)
		if e != nil {
			return result, e
		}
		result.Files += r.Files
		result.Reused += r.Reused
		result.SourceBytes += r.SourceBytes
		sum, e := hashFile(part.Output)
		if e != nil {
			return result, e
		}
		published := base + "-" + group + "-" + sum[:16] + ".zip"
		if _, e = os.Stat(published); os.IsNotExist(e) {
			if e = os.Link(part.Output, published); e != nil {
				// Copy on filesystems without hard links, then publish atomically.
				if e = copyAtomic(part.Output, published); e != nil {
					return result, e
				}
			}
		} else if e != nil {
			return result, e
		}
		descriptor.Packs = append(descriptor.Packs, filepath.Base(published))
	}
	b, err := json.Marshal(descriptor)
	if err != nil {
		return result, err
	}
	if err = writeAtomic(filepath.Join(cache, PacksName), b); err != nil {
		return result, err
	}
	core := o
	core.Source = cache
	core.group = "core"
	r, err := Build(core)
	result.Files += r.Files
	result.Reused += r.Reused
	result.SourceBytes += r.SourceBytes
	return result, err
}
func contains(list []string, s string) bool {
	i := sort.SearchStrings(list, s)
	return i < len(list) && list[i] == s
}
func runtimeSource(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "audio/odd/") || strings.HasPrefix(lower, "audio/odd_d01/") {
		return strings.HasSuffix(lower, ".ogg")
	}
	if strings.HasPrefix(lower, "sprites/") {
		return strings.HasSuffix(lower, ".png")
	}
	if strings.HasPrefix(lower, "pictures/") || strings.HasPrefix(lower, "media/") {
		return included(name) && !strings.HasSuffix(lower, ".png")
	}
	switch name {
	case "item_data.json", "formula_data.json", "skill_data.json", "animation_data.json", "npc_data.json", "scene_data.json", "talk_data.json":
		return true
	}
	return false
}
func packGroup(name string) string {
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "sprites/") {
		return "sprites"
	}
	if strings.HasPrefix(lower, "runtime/images/core/") {
		return "core"
	}
	if strings.HasPrefix(lower, "runtime/") || strings.HasPrefix(lower, "pictures/") {
		return "world"
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".wav", ".ogg", ".avi":
		return "audio"
	}
	return "core"
}
func (c *runtimeCompiler) readJSON(name string, v any) error {
	b, e := os.ReadFile(filepath.Join(c.source, filepath.FromSlash(name)))
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func (c *runtimeCompiler) link(name string) error {
	target := filepath.Join(c.root, filepath.FromSlash(name))
	source := filepath.Join(c.source, filepath.FromSlash(name))
	c.wanted[name] = true
	a, e := os.Stat(source)
	if e != nil {
		return e
	}
	if b, e := os.Stat(target); e == nil && os.SameFile(a, b) {
		return nil
	}
	if e = os.MkdirAll(filepath.Dir(target), 0755); e != nil {
		return e
	}
	if e = os.Remove(target); e != nil && !os.IsNotExist(e) {
		return e
	}
	if e = os.Link(source, target); e == nil {
		return nil
	}
	// Filesystems without hard links still work; use a fresh copy of this source.
	in, e := os.Open(source)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.Create(target)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, in)
	ce := out.Close()
	if e != nil {
		return e
	}
	return ce
}
func (c *runtimeCompiler) generate(key string, deps []string, build func(func(string, any) error) error) error {
	return c.generateBytes(key, deps, func(write func(string, []byte) error) error {
		return build(func(name string, v any) error {
			b, err := clientruntime.Encode(v)
			if err != nil {
				return err
			}
			return write(name, b)
		})
	})
}
func (c *runtimeCompiler) generateBytes(key string, deps []string, build func(func(string, []byte) error) error) error {
	signature, err := c.signature(deps)
	if err != nil {
		return err
	}
	old := c.old[key]
	valid := old.Signature == signature && len(old.Outputs) > 0
	if valid {
		for name, sum := range old.Outputs {
			actual, e := hashFile(filepath.Join(c.root, filepath.FromSlash(name)))
			if e != nil || actual != sum {
				valid = false
				break
			}
		}
	}
	if valid {
		for name := range old.Outputs {
			c.wanted[name] = true
		}
		c.next[key] = old
		return nil
	}
	next := runtimeCacheEntry{Signature: signature, ImageCodec: compiledImageCodecVersion, Outputs: map[string]string{}}
	err = build(func(name string, b []byte) error {
		path := filepath.Join(c.root, filepath.FromSlash(name))
		// Shared image tiles are written only once, even when multiple groups
		// reference an identical content hash.
		if strings.HasSuffix(name, clientimage.ChunkSuffix) {
			if sum, err := hashFile(path); err == nil && sum == digest(b) {
				c.wanted[name] = true
				next.Outputs[name] = sum
				return nil
			}
		}
		if e := writeAtomic(path, b); e != nil {
			return e
		}
		next.Outputs[name] = digest(b)
		c.wanted[name] = true
		return nil
	})
	if err != nil {
		return fmt.Errorf("compile %s: %w", key, err)
	}
	current, err := c.signature(deps)
	if err != nil {
		return err
	}
	if current != signature {
		return fmt.Errorf("sources changed during runtime compilation: %s; rebuild", key)
	}
	c.next[key] = next
	return nil
}

func copyAtomic(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(target), ".client-pack-*")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	defer out.Close()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	return os.Rename(out.Name(), target)
}

func (c *runtimeCompiler) signature(deps []string) (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "runtime-%d\n", runtimeCompilerVersion)
	for _, name := range deps {
		sum, err := hashFile(filepath.Join(c.source, filepath.FromSlash(name)))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s:%s\n", name, sum)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
