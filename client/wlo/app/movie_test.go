package app

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-go/internal/protocol"
)

// movieFrame is the server's kind-5 frame: play movie id in mode.
func movieFrameFor(id uint32, mode byte) []byte {
	p := []byte{protocol.CommandEvent, 1, 0, 0, 0, 1, eventKindMovie, 0, 0, 0, mode}
	p = binary.LittleEndian.AppendUint32(p, id)
	return append(p, 0, 0, 0)
}

// TestBeachMovieEvent: kind 5 with 12008 plays the beach rescue: the HUD
// is hidden, the lines show in the talk window and close on a click, and
// the end acknowledges the step with 20/6.
func TestBeachMovieEvent(t *testing.T) {
	c, now, _ := enteredClient(t)
	sent := wire(t, c)
	c.dispatch(movieFrameFor(12008, 1))
	if !c.MoviePlaying() {
		t.Fatal("no movie")
	}
	lines := 0
	dir := os.Getenv("SNAPSHOT_DIR")
	for i := 0; i < 60*120 && c.MoviePlaying(); i++ {
		*now = now.Add(time.Second / 60)
		c.Frame()
		if c.Talk.Shown() && c.Talk.Drawn() {
			lines++
			if lines == 1 && dir != "" {
				for j := 0; j < 6; j++ {
					*now = now.Add(time.Second / 60)
					c.Frame()
				}
				save(t, c, filepath.Join(dir, "movie_beach_line1.png"))
			}
			c.GroundClick(400, 300)
		}
	}
	if c.MoviePlaying() {
		t.Fatalf("movie still at stage %d", c.movie.p.Stage)
	}
	if lines != 12 {
		t.Fatalf("%d lines shown", lines)
	}
	c.Frame()
	got := sent()
	if len(got) == 0 || !bytes.Equal(got[len(got)-1], []byte{protocol.CommandEvent, protocol.EventAcknowledge}) {
		t.Fatalf("sent %x, want 20/6 at the end", got)
	}
}
