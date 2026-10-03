package hud

import (
	"reflect"
	"testing"
)

// TestParseTalk: tags are removed; #s plays its effect, #n inserts the
// player's name, #e breaks the line, #R and #B style what they enclose,
// and an opener without its closer stays text.
func TestParseTalk(t *testing.T) {
	for _, c := range []struct {
		src, text string
		sounds    []string
		marks     []byte
	}{
		{"#swav1541/#sMeow~", "Meow~", []string{"wav1541"}, nil},
		{"I'm #n/#n.", "I'm Dango.", nil, nil},
		{"East. #e/#e South.", "East. \r South.", nil, nil},
		{"level #R#B20/#R/#B or", "level 20 or", nil, []byte{0, 0, 0, 0, 0, 0, markRed | markBold, markRed | markBold, 0, 0, 0}},
		{"#F3/#FAh?", "Ah?", nil, nil},
		{"50% #off", "50% #off", nil, nil},
	} {
		got := parseTalk([]byte(c.src), []byte("Dango"))
		if string(got.text) != c.text || !reflect.DeepEqual(got.sounds, c.sounds) {
			t.Errorf("%q: %q sounds %v", c.src, got.text, got.sounds)
		}
		if c.marks != nil && !reflect.DeepEqual(got.marks, c.marks) {
			t.Errorf("%q: marks %v", c.src, got.marks)
		}
		if len(got.marks) != len(got.text) {
			t.Errorf("%q: %d marks for %d bytes", c.src, len(got.marks), len(got.text))
		}
	}
}

// TestWrapTalk: #e's break starts a new line.
func TestWrapTalk(t *testing.T) {
	text := []byte("Towards East:KaMa Cave. \r Towards South:Welling Village.")
	got := wrapTalk(text)
	if len(got) != 2 || string(text[got[1][0]:got[1][1]]) != " Towards South:Welling Village." {
		t.Fatalf("lines %v", got)
	}
}
