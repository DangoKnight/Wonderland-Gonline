package sprites

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"wonderland-gonline/internal/clientfs"
)

func TestCompiledSpriteMatchesEditable(t *testing.T) {
	bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE")
	if bundle == "" {
		t.Skip("set WONDERLAND_TEST_CLIENT_BUNDLE")
	}
	root, closePacks, err := clientfs.Mount(bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer closePacks()
	loose := NewManager(nil, []string{filepath.Join("..", "..", "..", "data", "sprites")})
	defer loose.Close()
	packed := NewManager([]string{filepath.Join(root, "sprites")}, nil)
	defer packed.Close()
	for _, name := range []string{"002c", "001"} {
		wa, err := loose.Archive(name)
		if err != nil {
			t.Fatal(err)
		}
		ga, err := packed.Archive(name)
		if err != nil {
			t.Fatal(err)
		}
		if ga.native != nil || ga.runtime == nil {
			t.Fatal("compiled sprites opened native source")
		}
		wa.Sprite(0)
		loose.WaitNative()
		want := wa.Sprite(0)
		got := ga.Sprite(0)
		if got == nil || want == nil || ga.SourceBytes != wa.SourceBytes || !reflect.DeepEqual(got.Palette, want.Palette) || !reflect.DeepEqual(got.Animations, want.Animations) || len(got.Frames) != len(want.Frames) {
			t.Fatalf("compiled sprite metadata differs for %s", name)
		}
		for i := range want.Frames {
			wf, gf := &want.Frames[i], &got.Frames[i]
			if wf.OffsetX != gf.OffsetX || wf.OffsetY != gf.OffsetY || wf.AnchorY != gf.AnchorY {
				t.Fatal("compiled frame placement differs")
			}
			wi, err := wf.Indices(loose)
			if err != nil {
				t.Fatal(err)
			}
			gi, err := gf.Indices(packed)
			if err != nil {
				t.Fatal(err)
			}
			if (wi == nil) != (gi == nil) {
				t.Fatal("compiled frame recoloring differs")
			}
			if wi != nil {
				for y := 0; y < wi.Rect.Dy(); y++ {
					a := wi.Pix[wi.PixOffset(wi.Rect.Min.X, wi.Rect.Min.Y+y):][:wi.Rect.Dx()]
					b := gi.Pix[gi.PixOffset(gi.Rect.Min.X, gi.Rect.Min.Y+y):][:gi.Rect.Dx()]
					if !reflect.DeepEqual(a, b) {
						t.Fatal("compiled indices differ")
					}
				}
			}
			wp, err := wf.Image(loose)
			if err != nil {
				t.Fatal(err)
			}
			gp, err := gf.Image(packed)
			if err != nil {
				t.Fatal(err)
			}
			if (wp == nil) != (gp == nil) {
				t.Fatal("compiled frame missing")
			}
			if wp != nil {
				for y := 0; y < wp.Rect.Dy(); y++ {
					a := wp.Pix[wp.PixOffset(wp.Rect.Min.X, wp.Rect.Min.Y+y):][:wp.Rect.Dx()*4]
					b := gp.Pix[gp.PixOffset(gp.Rect.Min.X, gp.Rect.Min.Y+y):][:gp.Rect.Dx()*4]
					if !reflect.DeepEqual(a, b) {
						t.Fatal("compiled frame pixels differ")
					}
				}
			}
		}
	}
}
