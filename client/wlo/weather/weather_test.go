package weather

import (
	"image"
	"image/color"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/surface"
)

// testLayer has stand-in pictures of the skin's sizes (frames stacked
// vertically) and a fixed seed.
func testLayer() *Layer {
	db := picdb.New()
	for name, size := range map[string]image.Point{
		"icon_leaves": {24, 120}, "icon_Snow1": {24, 96}, "icon_Snow2": {24, 96},
		"icon_steam_1": {113, 1030}, "icon_steam_2": {119, 1130}, "icon_steam_3": {150, 1260}, "icon_steam_4": {107, 1320},
		"Icon_Bubble_1": {28, 400}, "Icon_Bubble_2": {14, 200},
	} {
		m := image.NewRGBA(image.Rectangle{Max: size})
		for i := range m.Pix {
			m.Pix[i] = 0xff
		}
		m.Set(0, 0, color.RGBA{0, 0xff, 0, 0xff})
		db.Add(name, m)
	}
	r := &Rand{Seed: 1}
	return New(db, r.Intn)
}

func live(l *Layer, k Kind) []particle {
	var out []particle
	for _, p := range l.pools[k] {
		if p.live {
			out = append(out, p)
		}
	}
	return out
}

// TestLeavesFall: a leaf starts at the top of the view within its 800
// pixels, one per 900 ms, falls toward 600 pixels below and is freed on
// landing; at most eight fall at once.
func TestLeavesFall(t *testing.T) {
	l := testLayer()
	dst := surface.New(800, 600)
	cam := image.Pt(1000, 2000)
	now := time.Unix(100, 0)
	l.Under(dst, Leaves, cam, now)
	ps := live(l, Leaves)
	if len(ps) != 1 {
		t.Fatalf("%d leaves after the first frame", len(ps))
	}
	p := ps[0]
	if p.x < cam.X || p.x >= cam.X+spawnWidth || p.y != cam.Y || p.ty != cam.Y+fallDepth || p.fh != 24 {
		t.Fatalf("leaf %+v", p)
	}
	if d := p.tx - p.x; d%driftColumn != 0 || d < -4*driftColumn || d > 4*driftColumn {
		t.Fatalf("drift %d", d)
	}
	for i := 1; i <= 30; i++ {
		l.Under(dst, Leaves, cam, now.Add(time.Duration(i)*Frame))
	}
	ps = live(l, Leaves)
	if len(ps) != 2 {
		t.Fatalf("%d leaves after 900 ms", len(ps))
	}
	if ps[0].y <= cam.Y {
		t.Fatalf("leaf did not fall: %+v", ps[0])
	}
	for i := 31; i <= 2000; i++ {
		l.Under(dst, Leaves, cam, now.Add(time.Duration(i)*Frame))
		if n := len(live(l, Leaves)); n > specs[Leaves].slots {
			t.Fatalf("%d leaves", n)
		}
		for _, p := range live(l, Leaves) {
			if p.y > cam.Y+fallDepth {
				t.Fatalf("leaf past its target: %+v", p)
			}
		}
	}
	if len(live(l, Leaves)) == 0 {
		t.Fatal("no leaf falling")
	}
	l.Over(dst, Leaves, cam)
	for _, v := range dst.Pix {
		if v != 0 {
			return
		}
	}
	t.Fatal("over pass drew no leaf")
}

// TestMoveStep: the step is round(elapsed × speed) from the last frame
// change, along the longer axis, with the shorter one in proportion.
func TestMoveStep(t *testing.T) {
	at := time.Unix(0, 0)
	p := particle{x: 0, y: 0, tx: 50, ty: 600, speed: 0.02, at: at, every: time.Second, frames: 5}
	p.move(at.Add(100 * time.Millisecond))
	if p.y != 2 || p.x != 0 {
		t.Fatalf("after 100 ms at (%d, %d), want (0, 2)", p.x, p.y)
	}
	p.move(at.Add(500 * time.Millisecond))
	if p.y != 12 || p.x != 1 {
		t.Fatalf("after 500 ms at (%d, %d), want (1, 12)", p.x, p.y)
	}
	p = particle{x: 0, y: 0, tx: 3, ty: 4, speed: 0.02, at: at, every: time.Second, frames: 5}
	p.move(at.Add(300 * time.Millisecond))
	if !p.landed || p.x != 3 || p.y != 4 {
		t.Fatalf("near target: %+v", p)
	}
}

// TestSteamAndBubblesRise: steam starts 500..600 pixels below the view's
// top and rises to it; bubbles start 800..900 below and rise above it.
func TestSteamAndBubblesRise(t *testing.T) {
	l := testLayer()
	dst := surface.New(800, 600)
	now := time.Unix(100, 0)
	l.Under(dst, Steam, image.Point{}, now)
	l.Under(dst, Bubbles, image.Point{}, now)
	s, b := live(l, Steam), live(l, Bubbles)
	if len(s) != 1 || s[0].y < steamRise || s[0].y >= steamRise+steamSpread || s[0].ty != 0 || s[0].fh*10 > 1320 {
		t.Fatalf("steam %+v", s)
	}
	if len(b) != 1 || b[0].y < bubbleRise || b[0].tx != b[0].x || b[0].ty > 0 {
		t.Fatalf("bubble %+v", b)
	}
	want := 0.02 // Icon_Bubble_1, 100-pixel frames
	if b[0].fh != 100 {
		want = 0.015
	}
	if b[0].speed != want {
		t.Fatalf("bubble of frame height %d at %v px/ms, want %v", b[0].fh, b[0].speed, want)
	}
}

// TestSceneKind follows FUN_00334120's table.
func TestSceneKind(t *testing.T) {
	want := []Kind{None, Leaves, Snow, Steam, Rain, Leaves2, Bubbles, Ribbons, None}
	for b, k := range want {
		if got := SceneKind(byte(b)); got != k {
			t.Errorf("SceneKind(%d) = %d, want %d", b, got, k)
		}
	}
}
