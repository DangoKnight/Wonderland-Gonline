package hud

import (
	"image"
	"image/color"
	"testing"
	"time"

	"wonderland-go/client/wlo/picdb"
	"wonderland-go/client/wlo/seui"
	"wonderland-go/client/wlo/surface"
)

func TestChatEmoticonRenderedFrames(t *testing.T) {
	pics := picdb.New()
	icon := image.NewNRGBA(image.Rect(0, 0, 16, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 16; x++ {
			ink := color.NRGBA{R: 255, A: 255}
			if y >= 16 {
				ink = color.NRGBA{B: 255, A: 255}
			}
			icon.SetNRGBA(x, y, ink)
		}
	}
	pics.Add("icon_expre_7", icon)
	screen := surface.New(80, 40)
	now := time.Unix(0, 0)
	l := &ChatLog{Now: func() time.Time { return now }}
	l.Env = &seui.Env{Pics: pics, Screen: screen}
	l.drawChatText(10, 20, ChatLine{Text: []byte(":D:D")})
	for _, x := range []int{10, 26, 41} {
		if screen.Pix[18*80+x] != 0xf800 {
			t.Fatal("native icon position/advance incorrect")
		}
	}
	if screen.Pix[18*80+42] != 0 {
		t.Fatal("icon exceeded its text width")
	}
	now = now.Add(210 * time.Millisecond)
	l.drawChatText(10, 20, ChatLine{Text: []byte(":D")})
	if screen.Pix[18*80+10] != 0x001f {
		t.Fatal("chat icon did not animate on the seventh game frame")
	}
}

func TestChatEmoticonEditorPreview(t *testing.T) {
	pics := picdb.New()
	icon := image.NewNRGBA(image.Rect(0, 0, 16, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 16; x++ {
			ink := color.NRGBA{R: 255, A: 255}
			if y >= 16 {
				ink = color.NRGBA{B: 255, A: 255}
			}
			icon.SetNRGBA(x, y, ink)
		}
	}
	pics.Add("icon_expre_7", icon)
	env := &seui.Env{Pics: pics, Screen: surface.New(800, 600)}
	seui.NewManager(env, &seui.Input{})
	b := NewInputBar(env)
	now := time.Unix(0, 0)
	b.Message.Now = func() time.Time { return now }
	b.Message.SetText([]byte(":D:D"))
	b.Message.Paint()
	x, y := b.Message.TextX, b.Top+b.Message.TextY
	for _, px := range []int{x, x + 16, x + 31} {
		if env.Screen.Pix[y*800+px] != 0xf800 {
			t.Fatal("editor failed to replace native codes with preview icons")
		}
	}
	now = now.Add(210 * time.Millisecond)
	b.Message.Paint()
	if env.Screen.Pix[y*800+x] != 0x001f {
		t.Fatal("editor preview failed to animate")
	}
	if string(b.Message.Text) != ":D:D" || b.Message.Caret != 4 {
		t.Fatal("rendering changed the wire text or caret")
	}
}
