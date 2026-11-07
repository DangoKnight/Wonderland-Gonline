package clientbundle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/clientruntime"
	"wonderland-gonline/internal/localization"
	"wonderland-gonline/internal/spritepack"
)

func TestRuntimeGroundUpgradeRebuildsCachedMaps(t *testing.T) {
	o, source := fixture(t)
	put(t, filepath.Join(source, "ground_data.json"), []byte(`{"entries":[{"name":"11083.map","terrain":{"width":800,"height":600,"grid_width":2,"grid_height":2,"cells_hex":"00000000","optional_grid":{"unknown_flag":1,"rows":1,"columns":1,"cells":[{"unknown_u16_2":20000,"unknown_u8_5":4,"unknown_u8_6":1,"unknown_u32_7":883}]}}}]}`))
	if _, err := BuildRuntime(o); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(source)
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(filepath.Dir(o.Output), ".client-runtime-"+digest([]byte(abs))[:12])
	cacheFile := filepath.Join(cache, ".cache.json")
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatal(err)
	}
	var entries map[string]runtimeCacheEntry
	if err = json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	name := filepath.ToSlash(clientruntime.MapPath(11083))
	var ground clientruntime.Ground
	if err = clientruntime.Read(filepath.Join(cache, filepath.FromSlash(name)), &ground); err != nil {
		t.Fatal(err)
	}
	if ground.Terrain.TileGrid == nil {
		t.Fatal("initial compile discarded native grid")
	}
	// Reproduce a pre-grid compiler's output, including its valid checksum and
	// unchanged source signature. Source timestamps cannot detect this upgrade.
	ground.Terrain.TileGrid = nil
	encoded, err := clientruntime.Encode(ground)
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(cache, filepath.FromSlash(name)), encoded)
	legacyHash := sha256.New()
	sourceHash, err := hashFile(filepath.Join(source, "ground_data.json"))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(legacyHash, "runtime-2\nground_data.json:%s\n", sourceHash)
	entry := entries["ground"]
	entry.Signature = fmt.Sprintf("%x", legacyHash.Sum(nil))
	entry.Outputs[name] = digest(encoded)
	entries["ground"] = entry
	data, err = json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	put(t, cacheFile, data)
	if _, err = BuildRuntime(o); err != nil {
		t.Fatal(err)
	}
	root, closeBundle, err := clientfs.Mount(o.Output)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBundle()
	if err = clientruntime.Read(filepath.Join(root, clientruntime.MapPath(11083)), &ground); err != nil {
		t.Fatal(err)
	}
	if ground.Terrain.TileGrid == nil || len(ground.Terrain.TileGrid.Cells) != 1 || ground.Terrain.TileGrid.Cells[0].Wall != 20000 {
		t.Fatal("stale pre-grid map reused during compiler upgrade")
	}
}

func TestRuntimePacksCompileEditAndReuse(t *testing.T) {
	o, source := fixture(t)
	voice := []byte("OggS-native-dialogue-fixture")
	for _, archive := range []string{"odd", "odd_d01"} {
		if err := os.MkdirAll(filepath.Join(source, "audio", archive), 0755); err != nil {
			t.Fatal(err)
		}
		put(t, filepath.Join(source, "audio", archive, "20038_1002.ogg"), voice)
	}
	put(t, filepath.Join(source, "audio", "odd_index.json"), []byte(`{"source":"private installation path"}`))

	put(t, filepath.Join(source, "ground_data.json"), []byte(`{"entries":[{"name":"10017.map","decoded_hex":"1400000014000000000100010000000000006450015b0a0014002c019001","terrain":{"width":800,"height":600,"grid_width":2,"grid_height":2,"cells_hex":"01020304"}}]}`))
	put(t, filepath.Join(source, "eve_data.json"), []byte(`{"maps":[{"id":10017,"scene":12,"decoded_hex":"010203"}]}`))
	put(t, filepath.Join(source, "wem_data.json"), []byte(`{"entries":[{"name":"100.wem","decoded_hex":"000102"}]}`))
	put(t, filepath.Join(source, "database_data.json"), []byte(`{"private":"do not ship"}`))
	put(t, filepath.Join(source, "item_data.json"), []byte(`{"source_bytes":42,"items":[]}`))
	markText := []byte(`{"records":[{"fields":{"id":10022},"name":{"text":"Story of Red Hood"}}]}`)
	put(t, filepath.Join(source, "mark_data.json"), markText)
	dir := filepath.Join(source, "sprites", "001")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(dir, "editable.json"), []byte(`{"version":1,"sprites":[{"index":0,"name":"1000.jmp","frames":[],"animations":[]}]}`))
	if _, err := BuildRuntime(o); err != nil {
		t.Fatal(err)
	}
	coreInfo, _ := os.Stat(o.Output)
	if _, err := BuildRuntime(o); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(o.Output)
	if !os.SameFile(coreInfo, after) {
		t.Fatal("unchanged core rewritten")
	}
	root, closePacks, err := clientfs.Mount(o.Output)
	if err != nil {
		t.Fatal(err)
	}
	defer closePacks()

	for _, archive := range []string{"odd", "odd_d01"} {
		got, err := clientfs.ReadFile(filepath.Join(root, "audio", archive, "20038_1002.ogg"))
		if err != nil || !bytes.Equal(got, voice) {
			t.Fatalf("voice %s did not survive runtime packaging: %q %v", archive, got, err)
		}
	}
	gotMarks, err := clientfs.ReadFile(filepath.Join(root, "mark_data.json"))
	catalog, loadErr := localization.Load(root, localization.English)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	gotMarks, err = catalog.Expand(gotMarks)
	if err != nil || !bytes.Contains(gotMarks, []byte(`"text":"Story of the Hood Girl"`)) {
		t.Fatal("quest-mark text did not survive runtime packaging", err)
	}
	if _, err := clientfs.Stat(filepath.Join(root, "audio", "odd_index.json")); !os.IsNotExist(err) {
		t.Fatal("source audio index shipped", err)
	}
	var g clientruntime.Ground
	if err = clientruntime.Read(filepath.Join(root, clientruntime.MapPath(10017)), &g); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(g.Terrain.Cells, []byte{1, 2, 3, 4}) || g.Terrain.CellsHex != "" {
		t.Fatal("walk grid was not compiled")
	}
	var backdrops map[uint16]clientassets.GroundBackdrops
	if err = clientruntime.Read(filepath.Join(root, clientruntime.BattleBackdropsFile), &backdrops); err != nil || backdrops[10017].At(10, 20) != 91 || backdrops[10017].At(0, 0) != 80 {
		t.Fatal("native backdrop regions were not packaged", backdrops, err)
	}
	var events map[uint16]uint16
	if err = clientruntime.Read(filepath.Join(root, clientruntime.EventIndex), &events); err != nil || events[10017] != 12 {
		t.Fatal(events, err)
	}
	p, err := spritepack.ReadRuntime(filepath.Join(root, "sprites", "001"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = spritepack.ReadRuntimeSprite(filepath.Join(root, "sprites", "001"), p, 0); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ground_data.json", "eve_data.json", "wem_data.json", "database_data.json", "sprites/001/editable.json", "sprites/001/sprites.json", ".cache.json"} {
		if _, err = clientfs.Stat(filepath.Join(root, filepath.FromSlash(name))); !os.IsNotExist(err) {
			t.Fatalf("source-only export shipped: %s (%v)", name, err)
		}
	}
	closePacks()
	// A source edit replaces just its generated records and the affected pack.
	put(t, filepath.Join(source, "ground_data.json"), []byte(`{"entries":[{"name":"10017.map","terrain":{"width":800,"height":600,"grid_width":2,"grid_height":2,"cells_hex":"04030201"}}]}`))
	if _, err = BuildRuntime(o); err != nil {
		t.Fatal(err)
	}
	root, closePacks, err = clientfs.Mount(o.Output)
	if err != nil {
		t.Fatal(err)
	}
	defer closePacks()
	if err = clientruntime.Read(filepath.Join(root, clientruntime.MapPath(10017)), &g); err != nil || g.Terrain.Cells[0] != 4 {
		t.Fatal(g, err)
	}
	// Corrupt source must leave the published pack intact.
	info, _ := os.Stat(o.Output)
	put(t, filepath.Join(source, "ground_data.json"), []byte(`{`))
	if _, err = BuildRuntime(o); err == nil {
		t.Fatal("invalid source accepted")
	}
	after, _ = os.Stat(o.Output)
	if !os.SameFile(info, after) {
		t.Fatal("failed compile replaced core")
	}
}
func TestMissingCompanionPackFailsMount(t *testing.T) {
	o, source := fixture(t)
	put(t, filepath.Join(source, "item_data.json"), []byte(`{}`))
	if _, err := BuildRuntime(o); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(o.Output)
	if err != nil {
		t.Fatal(err)
	}
	var descriptor struct {
		Packs []string `json:"packs"`
	}
	for _, f := range z.File {
		if f.Name == PacksName {
			r, _ := f.Open()
			err = json.NewDecoder(r).Decode(&descriptor)
			r.Close()
		}
	}
	z.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(filepath.Dir(o.Output), descriptor.Packs[0])); err != nil {
		t.Fatal(err)
	}
	if _, closePacks, err := clientfs.Mount(o.Output); err == nil {
		closePacks()
		t.Fatal("missing companion accepted")
	}
}

func TestRuntimeRejectsFrameOutsideSheetWithoutNativeSource(t *testing.T) {
	o, source := fixture(t)
	dir := filepath.Join(source, "sprites", "001")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	picture(t, filepath.Join(dir, "sheet.png"), 2, 2, color.NRGBA{R: 255, A: 255})
	put(t, filepath.Join(dir, "editable.json"), []byte(`{"version":1,"sprites":[{"index":0,"name":"1.jmp","frames":[{"sheet":"sheet.png","rect":{"x":0,"y":0,"width":3,"height":2},"canvas_width":3,"canvas_height":2}],"animations":[]}]}`))
	if _, err := BuildRuntime(o); err == nil {
		t.Fatal("out of bounds frame accepted without native source")
	}
}

func TestPoolWorkersRespectsRequestCoresAndMemory(t *testing.T) {
	cores := runtime.NumCPU()
	for _, request := range []int{0, 1, defaultWorkers, 1 << 20} {
		if got := poolWorkers(request, largeImageDecodeBudget); got < 1 || got > cores {
			t.Fatalf("poolWorkers(%d) = %d, want 1..%d", request, got, cores)
		}
	}
	if got := poolWorkers(1, largeImageDecodeBudget); got != 1 {
		t.Fatalf("poolWorkers(1) = %d, want 1", got)
	}
	// An oversized per-worker budget must still leave one worker.
	if got := poolWorkers(cores, 1<<40); got != 1 {
		t.Fatalf("poolWorkers with oversized budget = %d, want 1", got)
	}
}
