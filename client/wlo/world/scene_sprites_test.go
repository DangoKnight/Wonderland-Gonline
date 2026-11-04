package world

import (
	"image"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
)

func TestSceneSpritesRespectNativeObjectDepth(t *testing.T) {
	for _, depth := range []int{103, 110, 120, 121} {
		back, front := surface.New(4, 4), surface.New(4, 4)
		back.Fill(image.Rect(0, 0, 4, 4), 1)
		front.Fill(image.Rect(0, 0, 4, 2), 2)
		front.Pix[1] = 0 // A hole in the foreground must reveal the sprite.
		w := &World{Env: &seui.Env{Screen: surface.New(800, 600), Pics: picdb.New()},
			Scene: &Scene{Width: 800, Height: 600, Objects: []Object{
				{X: 100, Y: 100, Depth: 5, Kind: ObjectSorted, Frames: 1, Image: back},
				{X: 100, Y: 100, Depth: 20, Kind: ObjectSorted, Frames: 1, Image: front},
			}}, Now: func() time.Time { return time.Unix(100, 0) }, HideNames: true, Cinematic: true, CameraAt: &image.Point{}}
		w.DrawWithSprites([]SceneSprite{{X: 100, Y: 110, Depth: depth - 110, Draw: func(dst *surface.Surface, x, y int) {
			if x != 100 || y != 110 {
				t.Fatal("incorrect camera placement")
			}
			dst.Fill(image.Rect(100, 100, 104, 104), 7)
		}}})
		for y := range 4 {
			for x := range 4 {
				want := uint16(7)
				if depth < 105 {
					want = 1
				}
				if depth <= 120 && y < 2 && (x != 1 || y != 0) {
					want = 2
				}
				if got := w.Env.Screen.Pix[(100+y)*800+100+x]; got != want {
					t.Fatalf("depth %d pixel %d,%d: got %d want %d", depth, x, y, got, want)
				}
			}
		}
	}
}
