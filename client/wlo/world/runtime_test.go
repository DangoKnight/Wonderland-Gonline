package world

import (
	"image"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/clientfs"
)

func TestCompiledWorldMatchesEditable(t *testing.T) {
	bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE")
	if bundle == "" {
		t.Skip("set WONDERLAND_TEST_CLIENT_BUNDLE")
	}
	root, closePacks, err := clientfs.Mount(bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer closePacks()
	loose := login.NewAssets(filepath.Join("..", "..", "..", "data"))
	compiled := login.NewAssets(root)
	for _, id := range []uint16{10017, 11016} {
		want, err := LoadScene(loose, id)
		if err != nil {
			t.Fatal(err)
		}
		got, err := LoadScene(compiled, id)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Ground, want.Ground) || !reflect.DeepEqual(got.Zones, want.Zones) || !reflect.DeepEqual(got.Objects, want.Objects) {
			t.Fatalf("compiled scene differs for %d", id)
		}
		if len(got.Layers) != len(want.Layers) {
			t.Fatal("compiled layer count differs")
		}
		for i, layer := range got.Layers {
			original := want.Layers[i]
			if layer.Resource != original.Resource || layer.X != original.X || layer.Y != original.Y || layer.Image.W != original.Image.W || layer.Image.H != original.Image.H {
				t.Fatal("compiled layer placement differs")
			}
			if layer.Image.Tiles == nil || layer.Image.Pix != nil {
				t.Fatal("compiled background eagerly decoded full canvas")
			}
		}
		for _, camera := range []image.Point{{}, {X: 100, Y: 100}, {X: max(0, want.Width-800), Y: max(0, want.Height-600)}} {
			editableFrame, compiledFrame := surface.New(800, 600), surface.New(800, 600)
			want.Draw(editableFrame, camera.X, camera.Y)
			got.Draw(compiledFrame, camera.X, camera.Y)
			if !reflect.DeepEqual(editableFrame.Pix, compiledFrame.Pix) {
				t.Fatalf("compiled map %d differs at camera %v", id, camera)
			}
		}
		wantEvents, err := MapRecord(loose, id)
		if err != nil {
			t.Fatal(err)
		}
		gotEvents, err := MapRecord(compiled, id)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotEvents, wantEvents) {
			t.Fatalf("compiled events differ for %d", id)
		}
	}
	want, err := MapScenes(loose)
	if err != nil {
		t.Fatal(err)
	}
	got, err := MapScenes(compiled)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("compiled scene index differs")
	}
}

func BenchmarkFirstTerrainRecord(b *testing.B) {
	bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE")
	if bundle == "" {
		b.Skip("set WONDERLAND_TEST_CLIENT_BUNDLE")
	}
	root, closePacks, err := clientfs.Mount(bundle)
	if err != nil {
		b.Fatal(err)
	}
	defer closePacks()
	loose := login.NewAssets(filepath.Join("..", "..", "..", "data"))
	compiled := login.NewAssets(root)
	for _, tc := range []struct {
		name   string
		assets login.Assets
	}{{"Editable", loose}, {"Compiled", compiled}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				grounds.Lock()
				grounds.path = ""
				grounds.entries = nil
				grounds.Unlock()
				if _, err := groundRecord(tc.assets, 10017); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkWorldBackgroundDrawing(b *testing.B) {
	bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE")
	if bundle == "" {
		b.Skip("set WONDERLAND_TEST_CLIENT_BUNDLE")
	}
	root, closePacks, err := clientfs.Mount(bundle)
	if err != nil {
		b.Fatal(err)
	}
	defer closePacks()
	for _, test := range []struct {
		name   string
		assets login.Assets
	}{
		{"Editable", login.NewAssets(filepath.Join("..", "..", "..", "data"))},
		{"CompiledTiles", login.NewAssets(root)},
	} {
		b.Run(test.name, func(b *testing.B) {
			scene, err := LoadScene(test.assets, 10017)
			if err != nil {
				b.Fatal(err)
			}
			dst := surface.New(800, 600)
			scene.Draw(dst, 700, 500)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				scene.Draw(dst, 700, 500)
			}
		})
	}
}
