package world

import (
	"image"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
)

type transitionTestBody struct {
	login.RoleView
	hole image.Point
}

func (b *transitionTestBody) HitBody(x, y int, _ int32, px, py int) bool {
	return image.Pt(px, py).In(image.Rect(x-12, y-60, x+12, y)) && image.Pt(px, py) != b.hole
}
func (b *transitionTestBody) Draw(dst *surface.Surface, x, y, action int) {
	b.DrawBody(dst, x, y, int32(action))
}
func (b *transitionTestBody) Bounds(x, y, _ int) image.Rectangle {
	return image.Rect(x-12, y-60, x+12, y)
}
func (b *transitionTestBody) FirstAnchorY() (int, bool) { return 60, true }
func (b *transitionTestBody) HitSprite(x, y, action, px, py int) bool {
	return b.HitBody(x, y, int32(action), px, py)
}

func (b *transitionTestBody) DrawBody(dst *surface.Surface, x, y int, _ int32) {
	for row := y - 60; row < y; row++ {
		for column := x - 12; column < x+12; column++ {
			if image.Pt(column, row) != b.hole {
				dst.Fill(image.Rect(column, row, column+1, row+1), 0xf800)
			}
		}
	}
}

func TestTransitionEffectsOverlayPlayersTransparently(t *testing.T) {
	for _, light := range []LightKind{SmallLight, LargeLight} {
		for _, actor := range []string{"player", "peer", "NPC", "pet"} {
			for _, offset := range []int{-12, 0, 12} {
				scr := surface.New(800, 600)
				now := time.Unix(100, 0)
				w := &World{Env: &seui.Env{Screen: scr, Pics: picdb.New()},
					Scene: &Scene{Width: 800, Height: 600}, HideScene: true, HideNames: true,
					Cinematic: true, CameraAt: &image.Point{}, Now: func() time.Time { return now },
					Player: Player{ID: 1, X: 200, Y: 200 + offset},
					Areas:  []Area{{Light: light, LightAt: image.Pt(200, 200)}}}
				w.Draw()
				baseline := append([]uint16(nil), scr.Pix...)
				bounds := image.Rect(188, 140+offset, 212, 200+offset)
				hole := image.Pt(-1, -1)
				for y := bounds.Min.Y; y < bounds.Max.Y && hole.X < 0; y++ {
					for x := bounds.Min.X; x < bounds.Max.X; x++ {
						if baseline[y*scr.W+x] != 0 {
							hole = image.Pt(x, y)
							break
						}
					}
				}
				if hole.X < 0 {
					t.Fatal("marker fixture has no overlap", light, offset)
				}
				body := &transitionTestBody{hole: hole}
				switch actor {
				case "peer":
					w.Peers = map[uint32]*Peer{2: {Player: Player{ID: 2, X: 200, Y: 200 + offset}, Role: body}}
				case "NPC":
					w.NPCs = map[uint16]*NPC{1: {X: 200, Y: 200 + offset, Shown: true, Painter: body}}
				case "pet":
					w.Companions = map[uint32]*Companion{1: {Painter: body, initialized: true, position: Player{X: 200, Y: 200 + offset}}}
				default:
					w.Body = body
				}
				scr.Fill(image.Rect(0, 0, scr.W, scr.H), 0)
				w.Draw()
				changed := 0
				for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
					for x := bounds.Min.X; x < bounds.Max.X; x++ {
						got := scr.Pix[y*scr.W+x]
						if image.Pt(x, y) == hole {
							if got != baseline[y*scr.W+x] {
								t.Fatal("transparent sprite pixel changed the effect")
							}
							continue
						}
						if got != 0xf800 {
							changed++
						}
						if got&0xf800 == 0 {
							t.Fatalf("light %d actor %s offset %d: effect erased body at %d,%d", light, actor, offset, x, y)
						}
					}
				}
				if changed == 0 {
					t.Fatal("effect was hidden behind the body", light, actor, offset)
				}
			}
		}
	}
}

func TestTransitionExtraTransparencyOnlyOnModels(t *testing.T) {
	for _, kind := range []LightKind{SmallLight, LargeLight} {
		t.Run(map[LightKind]string{SmallLight: "pyramid", LargeLight: "gateway"}[kind], func(t *testing.T) {
			ordinary, masked, again := surface.New(300, 300), surface.New(300, 300), surface.New(300, 300)
			for _, dst := range []*surface.Surface{ordinary, masked, again} {
				dst.Fill(image.Rect(0, 0, 300, 300), 0x8400)
			}
			// Only the left half overlaps a model; a transparent gap must keep
			// the original effect, just like terrain or an ordinary item.
			overModel := func(x, y int) bool { return x < 150 && y%3 != 0 }
			if kind == SmallLight {
				p := newExitIndicator()
				p.render(0)
				p.draw(ordinary, 150, 200)
				p.drawOverModels(masked, 150, 200, overModel)
				p.draw(again, 150, 200)
			} else {
				p := newGatewayLight()
				p.render(0)
				p.draw(ordinary, 150, 200)
				p.drawOverModels(masked, 150, 200, overModel)
				p.draw(again, 150, 200)
			}
			changed := 0
			for i, original := range ordinary.Pix {
				if again.Pix[i] != original {
					t.Fatal("model masking mutated the shared animation")
				}
				if masked.Pix[i] != original {
					if !overModel(i%300, i/300) {
						t.Fatal("extra transparency affected terrain, items or sprite gaps")
					}
					changed++
				}
			}
			if changed == 0 {
				t.Fatal("no extra transparency over models")
			}
		})
	}
}
