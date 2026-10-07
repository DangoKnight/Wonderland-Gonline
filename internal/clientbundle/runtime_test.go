package clientbundle

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/clientruntime"
	"wonderland-gonline/internal/spritepack"
)

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

	put(t, filepath.Join(source, "ground_data.json"), []byte(`{"entries":[{"name":"10017.map","terrain":{"width":800,"height":600,"grid_width":2,"grid_height":2,"cells_hex":"01020304"}}]}`))
	put(t, filepath.Join(source, "eve_data.json"), []byte(`{"maps":[{"id":10017,"scene":12,"decoded_hex":"010203"}]}`))
	put(t, filepath.Join(source, "wem_data.json"), []byte(`{"entries":[{"name":"100.wem","decoded_hex":"000102"}]}`))
	put(t, filepath.Join(source, "database_data.json"), []byte(`{"private":"do not ship"}`))
	put(t, filepath.Join(source, "item_data.json"), []byte(`{"source_bytes":42,"items":[]}`))
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

	for _, archive := range []string{"odd", "odd_d01"} {
		got, err := clientfs.ReadFile(filepath.Join(root, "audio", archive, "20038_1002.ogg"))
		if err != nil || !bytes.Equal(got, voice) {
			t.Fatalf("voice %s did not survive runtime packaging: %q %v", archive, got, err)
		}
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
