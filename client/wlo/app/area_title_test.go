package app

import (
	"image"
	"image/draw"
	"image/png"
	"os"
	"slices"
	"testing"
	"time"
)

func TestAreaTitleContinuousRevealAndFade(t *testing.T) {
	title := areaTitle{name: "Kelan Village"}
	if !title.makeArtwork() {
		t.Fatal("could not create title artwork")
	}
	first := slices.Clone(title.render(220 * time.Millisecond).Pix)
	second := slices.Clone(title.render(228 * time.Millisecond).Pix)
	if slices.Equal(first, second) {
		t.Fatal("ripple is stepped instead of advancing between native animation frames")
	}
	// Rendering intervening frames must not change the same-time result.
	for elapsed := time.Duration(0); elapsed < 220*time.Millisecond; elapsed += time.Second / 144 {
		title.render(elapsed)
	}
	if !slices.Equal(first, title.render(220*time.Millisecond).Pix) {
		t.Fatal("title animation depends on render frequency")
	}
	// Remove ring light for this assertion: only the text should move, and its
	// occupied rows must stay fixed while the distortion travels horizontally.
	envelope := title.rippleEnvelope
	title.rippleEnvelope = make([]float64, len(envelope))
	title.render(350 * time.Millisecond)
	translucent := false
	for i := 3; i < len(title.pixels.Pix); i += 4 {
		alpha := title.pixels.Pix[i]
		if alpha == 255 {
			t.Fatal("text became fully opaque before the inner ring arrived")
		}
		translucent = translucent || alpha > 0
	}
	if !translucent {
		t.Fatal("outer ripple did not begin revealing translucent text")
	}
	hold := slices.Clone(title.render(areaTitleReveal + 100*time.Millisecond).Pix)
	if slices.Equal(hold, title.render(areaTitleReveal+200*time.Millisecond).Pix) {
		t.Fatal("text stopped waving after the ripple reveal")
	}
	for y := 0; y < title.canvas.H; y++ {
		emptyMaskRow := true
		for _, alpha := range title.mask.Pix[y*title.mask.Stride : (y+1)*title.mask.Stride] {
			emptyMaskRow = emptyMaskRow && alpha == 0
		}
		if emptyMaskRow {
			for _, pixel := range title.canvas.Pix[y*title.canvas.W : (y+1)*title.canvas.W] {
				if pixel != 0 {
					t.Fatal("horizontal text wave moved into another row", y)
				}
			}
		}
	}
	title.rippleEnvelope = envelope
	bluePixels := func(p []uint16) int {
		count := 0
		for _, pixel := range p {
			r, b := (pixel>>11)&31, pixel&31
			if b > r {
				count++
			}
		}
		return count
	}
	if bluePixels(title.render(areaTitleDuration-250*time.Millisecond).Pix) == 0 {
		t.Fatal("ripples disappeared too early")
	}
	if bluePixels(title.render(areaTitleDuration-20*time.Millisecond).Pix) != 0 {
		t.Fatal("ripples did not dissolve before the animation ended")
	}
	for elapsed := 100 * time.Millisecond; elapsed < areaTitleReveal; elapsed += 100 * time.Millisecond {
		canvas := title.render(elapsed)
		for y := 0; y < canvas.H; y++ {
			if canvas.Pix[y*canvas.W] != 0 || canvas.Pix[(y+1)*canvas.W-1] != 0 {
				t.Fatal("ripple reached a hard side edge", elapsed)
			}
		}
		for x := 0; x < canvas.W; x++ {
			if canvas.Pix[x] != 0 || canvas.Pix[(canvas.H-1)*canvas.W+x] != 0 {
				t.Fatal("ripple reached a hard top/bottom edge", elapsed)
			}
		}
	}
	brightness := func(p []uint16) int {
		n := 0
		for _, v := range p {
			n += int(v & 31)
		}
		return n
	}
	full := brightness(title.render(areaTitleReveal + areaTitleHold/2).Pix)
	fading := brightness(title.render(areaTitleReveal + areaTitleHold + areaTitleFade/2).Pix)
	if full == 0 || fading >= full || fading == 0 || title.render(areaTitleDuration) != nil {
		t.Fatal("title hold/fade lifetime incorrect")
	}
	if out := os.Getenv("AREA_TITLE_FRAMES"); out != "" {
		frames := image.NewRGBA(image.Rect(0, 0, title.canvas.W*4, title.canvas.H))
		for i, elapsed := range []time.Duration{100 * time.Millisecond, 220 * time.Millisecond, areaTitleReveal, areaTitleReveal + areaTitleHold + areaTitleFade/2} {
			draw.Draw(frames, image.Rect(i*title.canvas.W, 0, (i+1)*title.canvas.W, title.canvas.H), title.render(elapsed).RGBA(), image.Point{}, draw.Src)
		}
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, frames); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAreaTitleOpacityAcrossRipples(t *testing.T) {
	for _, tc := range []struct{ distance, opacity float64 }{
		{1, 0}, {0, 0}, {-9, .25}, {-18, .5}, {-27, .75}, {-36, 1}, {-54, 1},
	} {
		if got := areaTitleTextOpacity(tc.distance); got != tc.opacity {
			t.Fatal("incorrect opacity between ripple fronts", tc, got)
		}
	}
}

func TestAreaTitleMapEntryLifecycle(t *testing.T) {
	c, now, _ := enteredClient(t)
	// Independent AC12 places Dango in Kelan at the reference screenshot's position.
	c.dispatch([]byte{12, 0x11, 0x27, 0, 0, 0xe0, 0x2e, 0x67, 9, 0xb7, 8})
	if !c.areaTitle.pending || c.areaTitle.name != "Kelan Village" {
		t.Fatal("map change did not queue its English title", c.areaTitle.name)
	}
	if c.areaTitle.identity != uint32(10002)<<16 {
		t.Fatal("native Kelan title group was not loaded")
	}
	c.areaTitleTick(*now)
	if !c.areaTitle.started.IsZero() {
		t.Fatal("title started before map readiness")
	}
	c.dispatch([]byte{5, 4})
	c.areaTitleTick(*now)
	started := c.areaTitle.started
	if started.IsZero() {
		t.Fatal("ready map did not start title")
	}
	*now = now.Add(220 * time.Millisecond)
	c.Frame()
	if out := os.Getenv("AREA_TITLE_SNAPSHOT"); out != "" {
		savePNG(t, out, c)
	}
	// Repositioning on the same map and repeated readiness are not area entries.
	c.dispatch([]byte{12, 0x11, 0x27, 0, 0, 0xe0, 0x2e, 0x67, 9, 0xb7, 8})
	c.dispatch([]byte{5, 4})
	c.areaTitleTick(*now)
	if !c.areaTitle.started.Equal(started) {
		t.Fatal("same-area placement restarted title")
	}
	*now = now.Add(areaTitleDuration)
	c.areaTitleTick(*now)
	if !c.areaTitle.started.IsZero() || c.areaTitle.pending {
		t.Fatal("title outlived its timeout")
	}
	c.queueAreaTitle()
	if c.areaTitle.pending {
		t.Fatal("same title group replayed without leaving the area")
	}
	c.disconnected()
	if c.areaTitle.identity != 0 {
		t.Fatal("title state leaked into another login")
	}
}

func TestAreaTitleNativeTransitionRules(t *testing.T) {
	c, now, _ := enteredClient(t)
	for _, tc := range []struct {
		name  string
		mapID uint16
		show  bool
	}{
		{"North Island entry", 60001, true},
		{"North Island to Kelan", 12000, true},
		{"Kelan to house", 12002, false},
		{"house to Kelan", 12000, false},
		{"Kelan to North Island", 60001, true},
		{"same North Island title group", 60000, false},
		{"Ship Deck", 10001, false},
		{"Cabin", 10002, false},
		{"return to Ship Deck", 10001, false},
		{"another civilian house", 12004, false},
		{"another house to Kelan", 12000, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c.World.Player.Map = tc.mapID
			c.World.SceneName = c.sceneNames[c.mapScenes[tc.mapID]]
			c.queueAreaTitle()
			if c.areaTitle.pending != tc.show {
				t.Fatalf("title queued=%v want %v", c.areaTitle.pending, tc.show)
			}
			// Expiring a presentation must retain the previous scene category.
			category := c.areaTitle.category
			c.areaTitle.started = *now
			c.areaTitleTick(now.Add(areaTitleDuration))
			if c.areaTitle.category != category {
				t.Fatal("expiry lost transition category")
			}
		})
	}
}
