package minigame

import (
	"bytes"
	"testing"
	"time"
)

func TestArcadeNativePurchases(t *testing.T) {
	now := time.Unix(100, 0)
	for _, tc := range []struct {
		kind    byte
		request []byte
		reply   []byte
	}{
		{6, []byte{71, 6, 1}, []byte{6, 1, 2, 4}},
		{22, []byte{71, 22, 1}, []byte{22, 1, 3, 2}},
		{8, []byte{71, 8, 1, 0}, []byte{8, 1, 2, 3, 5, 7, 4}},
		{10, []byte{71, 10, 0}, []byte{10, 1, 2, 3, 5, 7, 4}},
		{19, []byte{71, 19, 1, 0}, []byte{19, 1, 2, 3, 5, 7, 4}},
	} {
		t.Run(string(rune('A'+tc.kind)), func(t *testing.T) {
			g := NewArcade(nil, tc.kind)
			g.Rand = func(int) int { t.Fatal("paid outcome used local RNG"); return 0 }
			var sent [][]byte
			g.Send = func(p []byte) { sent = append(sent, append([]byte(nil), p...)) }
			results := 0
			g.Result = func(bool) { results++ }
			r := g.buttonRect()
			g.Click(r.Min.X+1, r.Min.Y+1, now)
			g.Begin(now)
			if len(sent) != 0 {
				t.Fatal("opened game purchased")
			}
			g.Click(r.Min.X+1, r.Min.Y+1, now)
			if len(sent) != 0 || !g.confirming {
				t.Fatal("purchase missing confirmation")
			}
			g.Click(r.Min.X+1, r.Min.Y+1, now)
			g.Click(r.Min.X+1, r.Min.Y+1, now)
			if len(sent) != 1 || !bytes.Equal(sent[0], tc.request) {
				t.Fatal("request", sent)
			}
			g.Update(now.Add(time.Hour))
			if !g.pending || results != 0 || g.quantity != 0 {
				t.Fatal("invented result without server")
			}
			for _, invalid := range [][]byte{nil, {tc.kind}, {255, 1, 2, 3, 5, 7, 4}, {tc.kind, 99}, tc.reply[:len(tc.reply)-1]} {
				if g.Receive(invalid, now) {
					t.Fatal("accepted malformed reply", invalid)
				}
			}
			if !g.Receive(tc.reply, now) || g.Receive(tc.reply, now) {
				t.Fatal("reply correlation")
			}
			if g.quantity != tc.reply[len(tc.reply)-1] {
				t.Fatal("quantity", g.quantity)
			}
			if !g.egg() && g.reels != [3]byte{3, 5, 7} {
				t.Fatal("reel byte order", g.reels)
			}
			g.Update(now.Add(5 * time.Second))
			if g.animating || g.pending || results != 0 {
				t.Fatal("animation ended event or did not finish")
			}
			g.End(false)
			g.End(false)
			if results != 1 {
				t.Fatal("duplicate Leave result", results)
			}
			if g.Receive(tc.reply, now) {
				t.Fatal("closed game accepted reply")
			}
		})
	}
}

func TestArcadeDenialRetryAndInvalidReels(t *testing.T) {
	now := time.Unix(100, 0)
	g := NewArcade(nil, 8)
	g.Begin(now)
	r := g.buttonRect()
	var sent [][]byte
	g.Send = func(p []byte) { sent = append(sent, p) }
	buy := func() { g.Click(r.Min.X+1, r.Min.Y+1, now); g.Click(r.Min.X+1, r.Min.Y+1, now) }
	buy()
	if g.Receive([]byte{8, 1, 1, 0, 2, 3, 1}, now) {
		t.Fatal("invalid zero reel")
	}
	if g.Receive([]byte{8, 1, 11, 1, 2, 3, 1}, now) {
		t.Fatal("out-of-table prize")
	}
	if !g.Receive([]byte{8, 2}, now) || g.pending || g.status != "Not enough Points!" {
		t.Fatal("denial")
	}
	buy()
	if len(sent) != 2 || !g.pending {
		t.Fatal("retry", sent)
	}
}
