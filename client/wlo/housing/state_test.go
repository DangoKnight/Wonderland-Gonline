package housing

import (
	"encoding/hex"
	"encoding/json"
	"image"
	"os"
	"reflect"
	"testing"
)

func captured(t *testing.T) map[string][]byte {
	t.Helper()
	b, err := os.ReadFile("testdata/legacy_tents.json")
	if err != nil {
		t.Fatal(err)
	}
	var encoded map[string]string
	if err := json.Unmarshal(b, &encoded); err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for k, v := range encoded {
		b, err := hex.DecodeString(v)
		if err != nil {
			t.Fatal(err)
		}
		out[k] = b
	}
	return out
}

func TestTentCapturedSignsReplaceAndClose(t *testing.T) {
	p := captured(t)
	var s State
	if !s.Receive(p["open"]) || len(s.Signs) != 1 {
		t.Fatal("open sign missing", s)
	}
	v := s.Signs[4617671]
	if v.Item != 36002 || v.X != 2442 || v.Y != 2015 {
		t.Fatal("native sign layout", v)
	}
	if !s.Receive(p["signs"]) || len(s.Signs) != 3 {
		t.Fatal("map-entry sign snapshot", s)
	}
	before := s.Signs
	if s.Receive(p["signs"][:len(p["signs"])-1]) || !reflect.DeepEqual(s.Signs, before) {
		t.Fatal("partial snapshot published")
	}
	if !s.Receive(p["closed"]) || len(s.Signs) != 2 {
		t.Fatal("closing tent retained its sign")
	}
	if !s.Receive([]byte{65, 3}) || len(s.Signs) != 0 {
		t.Fatal("empty snapshot retained old tents")
	}
}

func TestTentCapturedFurnitureMoves(t *testing.T) {
	p := captured(t)
	var s State
	if !s.Receive(p["tree"]) || s.Owner != 4617671 || len(s.Furniture) != 2 {
		t.Fatal("native furniture tree", s)
	}
	v := s.Furniture[1]
	if v.Item != 38049 || v.X != 42 || v.Y != 39 || v.Z != 1 || v.Parent != 0 || v.Floor != 0 {
		t.Fatal("native root offsets", v)
	}
	if !s.Receive(p["move"]) {
		t.Fatal("move rejected")
	}
	v = s.Furniture[1]
	if v.X != 32 || v.Y != 38 || v.Item != 38049 {
		t.Fatal("move lost furniture definition", v)
	}
	if s.Receive(p["tree"][:len(p["tree"])-1]) || s.Furniture[1] != v {
		t.Fatal("partial tree destroyed current furniture")
	}
	if !s.Receive([]byte{62, 2, 1, 0}) || len(s.Furniture) != 1 {
		t.Fatal("pickup retained furniture")
	}
}

func TestTentGeometryNativeProjection(t *testing.T) {
	if got := Pixel(42, 39, 1); got != image.Pt(909, 632) {
		t.Fatal(got)
	}
	if got := Pixel(32, 38, 1); got != image.Pt(639, 467) {
		t.Fatal(got)
	}
	for x := 29; x <= 44; x++ {
		for y := 27; y <= 50; y++ {
			p := Pixel(x, y, 1)
			if Cell(p.X, p.Y) != image.Pt(x, y) {
				t.Fatal(x, y, p)
			}
		}
	}
	if Pixel(42, 39, 2).Y != 602 || Room(1) != image.Rect(29, 38, 45, 51) {
		t.Fatal("native stacking or room bounds")
	}
}
