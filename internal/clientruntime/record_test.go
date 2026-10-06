package clientruntime

import (
	"bytes"
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
