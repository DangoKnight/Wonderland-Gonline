package clientruntime

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestRuntimeRecordVersionAndRoundTrip(t *testing.T) {
	value := Ground{Name: "10017.map", Terrain: Terrain{Width: 800, Height: 600, GridWidth: 2, GridHeight: 2, Cells: []byte{1, 2, 3, 4}}}
	data, err := Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Ground
	if err = Decode(bytes.NewReader(data), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Name != value.Name || !bytes.Equal(decoded.Terrain.Cells, value.Terrain.Cells) {
		t.Fatal("terrain changed")
	}
	data[4] = 2
	if err = Decode(bytes.NewReader(data), &decoded); err == nil {
		t.Fatal("unsupported schema accepted")
	}
	if err = Decode(bytes.NewReader(data[:3]), &decoded); err == nil {
		t.Fatal("truncated record accepted")
	}
}

func TestTerrainGridSurvivesCompilation(t *testing.T) {
	// Authored export bytes exercise the raw field mapping as well as gob.
	raw := []byte(`{"name":"11083.map","terrain":{"optional_grid":{"unknown_flag":1,"rows":1,"columns":1,"cells":[{"unknown_u16_0":0,"unknown_u16_2":20000,"unknown_u8_4":0,"unknown_u8_5":4,"unknown_u8_6":1,"unknown_u32_7":4294967229}]}}}`)
	var value Ground
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	b, err := Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Ground
	if err = Decode(bytes.NewReader(b), &decoded); err != nil {
		t.Fatal(err)
	}
	g := decoded.Terrain.TileGrid
	if g == nil || g.Rows != 1 || g.Columns != 1 || len(g.Cells) != 1 || g.Cells[0].Wall != 20000 || g.Cells[0].Depth != 4294967229 {
		t.Fatal("terrain scenery discarded during compilation", g)
	}
}
