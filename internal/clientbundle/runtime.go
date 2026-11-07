package clientbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wonderland-gonline/internal/clientimage"
	"wonderland-gonline/internal/clientruntime"
	"wonderland-gonline/internal/clienttext"
	"wonderland-gonline/internal/localization"
	"wonderland-gonline/internal/spritepack"
)

const PacksName = "_packs.json"
const runtimeCompilerVersion = 2
const runtimePackDescriptorVersion = 1
const compiledImageCodecVersion = 2
const compiledGroundLayoutVersion = 2 // Retain the native isometric scenery grid.
const compiledGroundMaxGridSide = 900

// Parallel stage sizing. Every stage runs at most defaultWorkers goroutines
// unless -workers asks for more, and no stage ever exceeds the core count.
// Image decode stages are additionally bounded by physical memory: each
// worker holds one fully decoded source canvas in RAM while encoding its
// chunks, so the pool must stay a bounded fraction of installed memory
// rather than a fixed number that fits only one machine class.
const defaultWorkers = 4

// imageDecodeMemoryDivisor keeps the decode stages within 1/divisor of
// physical RAM for resident source canvases at peak.
const imageDecodeMemoryDivisor = 4

// decodeBudgets are per-worker resident-canvas allowances. The largest
// shipped maps (64-72 MiB PNG sources) decode to ~300 MiB including the
// normalized copy; small images stay within their tile-page footprint.
const largeImageDecodeBudget = 300 << 20
const smallImageDecodeBudget = 8 << 20

// poolWorkers resolves the decode pool size: the requested bound, limited by
// the core count and by how many per-worker budgets fit into the memory
// fraction. When memory size is unknown the memory bound is skipped.
func poolWorkers(requested int, budget uint64) int {
	workers := max(1, min(requested, runtime.NumCPU()))
	if total := memoryTotalBytes(); total > 0 && budget > 0 {
		workers = min(workers, max(1, int(total/imageDecodeMemoryDivisor/budget)))
	}
	return workers
}

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
	// mu guards wanted/old/next/recompressed during compileImages, where
	// independent image groups compile on a shared worker pool.
	mu      sync.Mutex
	verbose bool
	// workers bounds every parallel stage; normalized to defaultWorkers.
	workers int
	// published tracks content-addressed chunks already claimed by a worker
	// in this run, so concurrent publishers of identical content write once.
	published map[string]bool
}

// BuildRuntime compiles heavy exports into independently readable binary records
// and writes four independently replaceable packs. Output is the core pack.
func BuildRuntime(o Options) (Result, error) {
	absolute, err := filepath.Abs(o.Source)
	if err != nil {
		return Result{}, err
	}
	cache := filepath.Join(filepath.Dir(o.Output), ".client-runtime-"+digest([]byte(absolute))[:12])
	c := runtimeCompiler{source: o.Source, root: cache, wanted: map[string]bool{}, old: map[string]runtimeCacheEntry{}, next: map[string]runtimeCacheEntry{}, published: map[string]bool{}, verbose: o.Verbose, workers: o.Workers}
	if c.workers <= 0 {
		c.workers = defaultWorkers
	}
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
	phaseStart := time.Now()
	defer phase(o.Verbose, "BuildRuntime total", phaseStart)
	if err = c.validateImages(o); err != nil {
		return Result{}, err
	}
	phase(o.Verbose, "validate-images", phaseStart)
	linkStart := time.Now()
	var linkNames []string
	for _, name := range c.files {
		if clienttext.IsExport(name) {
			continue
		}
		if !runtimeSource(name) {
			continue
		}
		linkNames = append(linkNames, name)
		c.wanted[name] = true
	}
	if err = parallelEach(linkNames, c.workers, func(_ int, name string) error { return c.link(name) }); err != nil {
		return Result{}, err
	}
	phase(o.Verbose, "link-runtime-sources", linkStart)
	var textSources []string
	for _, name := range c.files {
		if clienttext.IsExport(name) {
			textSources = append(textSources, name)
		}
	}
	if len(textSources) > 0 {
		if err := c.generateBytes("localized-text-"+clienttext.Fingerprint(), textSources, func(write func(string, []byte) error) error {
			catalogs := clienttext.NewCatalogs()
			for _, name := range textSources {
				raw, err := os.ReadFile(filepath.Join(c.source, name))
				if err != nil {
					return err
				}
				keyed, texts, err := clienttext.CompileLocalizedDocument(name, raw)
				if err != nil {
					return err
				}
				clienttext.MergeCatalogs(catalogs, texts)
				if err := write(name, keyed); err != nil {
					return err
				}
			}
			for code, values := range catalogs {
				raw, err := json.Marshal(localization.Document{Version: localization.Version, Language: code, Strings: values})
				if err != nil {
					return err
				}
				if err := write(localization.Directory+"/"+code+".json", raw); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return Result{}, err
		}
	}
	imagesStart := time.Now()
	if err = c.compileImages(); err != nil {
		return Result{}, err
	}
	phase(o.Verbose, "compile-images", imagesStart)
	if contains(c.files, "ground_data.json") {
		if err = c.generate("battle-backdrops", []string{"ground_data.json"}, func(write func(string, any) error) error {
			var doc clientruntime.BackdropExport
			if e := c.readJSON("ground_data.json", &doc); e != nil {
				return e
			}
			values, e := doc.Decode()
			if e != nil {
				return e
			}
			return write(clientruntime.BattleBackdropsFile, values)
		}); err != nil {
			return Result{}, err
		}
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
	descriptor := struct {
		Version int      `json:"version"`
		Packs   []string `json:"packs"`
	}{runtimePackDescriptorVersion, []string{}}
	// Companion pack builds read the same finished cache and write independent
	// zips, so they run concurrently. Group builds skip contract validation and
	// rewriting: the runtime compiler already validated every image against the
	// contract, and concurrent identical contract writes would race. Build the
	// published core only after every companion and its descriptor are ready.
	packsStart := time.Now()
	type packOutcome struct {
		group, path string
		result      Result
		err         error
	}
	groups := []struct {
		name, path string
	}{
		{"world", base + "-world.build.zip"},
		{"sprites", base + "-sprites.build.zip"},
		{"audio", base + "-audio.build.zip"},
	}
	outcomes := make([]packOutcome, len(groups))
	var packsWg sync.WaitGroup
	for i, g := range groups {
		packsWg.Add(1)
		go func(i int, g struct{ name, path string }) {
			defer packsWg.Done()
			part := o
			part.Source = cache
			part.Output = g.path
			part.Contract = ""
			part.group = g.name
			r, e := Build(part)
			outcomes[i] = packOutcome{g.name, g.path, r, e}
		}(i, g)
	}
	packsWg.Wait()
	var result Result
	for i := range outcomes {
		if outcomes[i].err != nil {
			return result, outcomes[i].err
		}
	}
	phase(o.Verbose, "build-packs", packsStart)
	publishStart := time.Now()
	for _, outcome := range outcomes {
		result.Files += outcome.result.Files
		result.Reused += outcome.result.Reused
		result.SourceBytes += outcome.result.SourceBytes
		sum, e := hashFile(outcome.path)
		if e != nil {
			return result, e
		}
		published := base + "-" + outcome.group + "-" + sum[:16] + ".zip"
		if _, e = os.Stat(published); os.IsNotExist(e) {
			if e = os.Link(outcome.path, published); e != nil {
				// Copy on filesystems without hard links, then publish atomically.
				if e = copyAtomic(outcome.path, published); e != nil {
					return result, e
				}
			}
		} else if e != nil {
			return result, e
		}
		descriptor.Packs = append(descriptor.Packs, filepath.Base(published))
	}
	phase(o.Verbose, "publish-packs", publishStart)
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

// parallelEach runs work over names on a worker pool bounded by maxWorkers
// (and the core count). Index ownership follows the input order, so callers
// can write results[i] without locking; the first error cancels the
// remaining jobs, and workers keep draining the queue to unblock the
// producer.
func parallelEach(names []string, maxWorkers int, work func(int, string) error) error {
	if len(names) == 0 {
		return nil
	}
	workers := max(1, min(len(names), min(maxWorkers, runtime.NumCPU())))
	jobs := make(chan int)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := range jobs {
				if errs[w] == nil {
					errs[w] = work(i, names[i])
				}
			}
		}(w)
	}
	for i := range names {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func phase(verbose bool, name string, start time.Time) {
	if verbose {
		log.Printf("clientbundle: %s took %v", name, time.Since(start))
	}
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
	case "item_data.json", "formula_data.json", "skill_data.json", "animation_data.json", "npc_data.json", "scene_data.json", "talk_data.json", "mark_data.json":
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
	c.mu.Lock()
	old := c.old[key]
	c.mu.Unlock()
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
		c.mu.Lock()
		for name := range old.Outputs {
			c.wanted[name] = true
		}
		c.next[key] = old
		c.mu.Unlock()
		return nil
	}
	next := runtimeCacheEntry{Signature: signature, ImageCodec: compiledImageCodecVersion, Outputs: map[string]string{}}
	err = build(func(name string, b []byte) error {
		path := filepath.Join(c.root, filepath.FromSlash(name))
		// Shared image tiles are written only once, even when multiple groups
		// reference an identical content hash. Chunk names are content-addressed,
		// so an in-memory claim makes concurrent publishers skip the write; a
		// matching file on disk (previous build or a just-finished rename) is
		// accepted without rewriting.
		if strings.HasSuffix(name, clientimage.ChunkSuffix) {
			sum := digest(b)
			c.mu.Lock()
			if c.published[name] {
				c.wanted[name] = true
				next.Outputs[name] = sum
				c.mu.Unlock()
				return nil
			}
			c.published[name] = true
			c.mu.Unlock()
			if actual, err := hashFile(path); err == nil && actual == sum {
				c.mu.Lock()
				c.wanted[name] = true
				next.Outputs[name] = sum
				c.mu.Unlock()
				return nil
			}
			if err := writeChunk(path, b, sum); err != nil {
				return err
			}
			c.mu.Lock()
			next.Outputs[name] = sum
			c.wanted[name] = true
			c.mu.Unlock()
			return nil
		}
		if e := writeAtomic(path, b); e != nil {
			return e
		}
		c.mu.Lock()
		next.Outputs[name] = digest(b)
		c.wanted[name] = true
		c.mu.Unlock()
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
	c.mu.Lock()
	c.next[key] = next
	c.mu.Unlock()
	return nil
}

// writeChunk publishes a content-addressed image chunk, accepting an identical
// file already on disk (previous build or a concurrent publisher that finished
// its rename first) and retrying through transient locks such as real-time
// antivirus scans on newly created files.
func writeChunk(path string, data []byte, sum string) error {
	var last error
	for attempt := 0; attempt < 5; attempt++ {
		if last = writeAtomic(path, data); last == nil {
			return nil
		}
		if actual, err := hashFile(path); err == nil && actual == sum {
			return nil
		}
		time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
	}
	return fmt.Errorf("publish %s: %w", filepath.ToSlash(path), last)
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
		if name == "ground_data.json" {
			fmt.Fprintf(h, "ground-layout-%d\n", compiledGroundLayoutVersion)
		}
		sum, err := hashFile(filepath.Join(c.source, filepath.FromSlash(name)))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s:%s\n", name, sum)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
