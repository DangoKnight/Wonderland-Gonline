package hud

import (
	"image"
	"image/color"
	"testing"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/text"
	"wonderland-gonline/internal/clientassets"
)

type mailTabBackend struct {
	surface.Backend
	draws int
}

func (b *mailTabBackend) DrawRect(int, int, image.Rectangle, *surface.Surface, bool) { b.draws++ }

func TestMailboxSentTabPaintsWithoutFramebufferReadback(t *testing.T) {
	pics := picdb.New()
	picture := image.NewNRGBA(image.Rect(0, 0, 80, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 80; x++ {
			picture.SetNRGBA(x, y, color.NRGBA{R: uint8(40 + y), G: 90, B: 150, A: 255})
		}
	}
	pics.Add("mail-test-tab", picture)
	backend := &mailTabBackend{}
	const nativeFontBytes = 256*15 + 13910*30
	font, err := clientassets.DecodeFont(make([]byte, nativeFontBytes))
	if err != nil {
		t.Fatal(err)
	}
	env := &seui.Env{Pics: pics, Screen: &surface.Surface{W: 800, H: 600, Backend: backend}, Text: &text.Renderer{Font: font}}
	tab := &mailSentTab{}
	tab.InitFixedButton(tab, env, nil)
	tab.Init("mail-test-tab", 200, 100, 0, 0, 0, false, 20, 80, 0)
	for state := byte(0); state < 3; state++ {
		tab.State = state
		tab.Paint()
		if tab.backgrounds[state] == nil {
			t.Fatal("label background not reconstructed")
		}
	}
	if backend.draws < 6 {
		t.Fatal("did not render tab and label background")
	}
	if env.Screen.Pix != nil {
		t.Fatal("created CPU framebuffer for GPU target")
	}
}
