package app

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/internal/protocol"
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

// TestStormMovieEffects: the storm (123) tints the screen red in stages 1,
// 3 and 5 (effect 7), starts the sound table's entry 3 (BGM0003) as the
// music in stage 1, draws the passengers who walk to the cabin door only
// in their stages (17 and 18), says its lines in stages 2, 4, 5 and 14,
// and shows the "?" over the captain (S10238) in stage 2.
func TestStormMovieEffects(t *testing.T) {
	c, now, _ := enteredClient(t)
	var music []int
	c.dispatch(movieFrameFor(123, 1))
	if !c.MoviePlaying() {
		t.Fatal("no movie")
	}
	p := c.movie.p
	p.Music = func(i int) { music = append(music, i) }
	red := map[int]bool{}
	var passengers []int // NPCs present only in stages 17..18
	for i, a := range p.M.NPCs {
		if a.Start == 16 && a.Count == 2 {
			passengers = append(passengers, i)
		}
	}
	if len(passengers) == 0 {
		t.Fatalf("%d passengers", len(passengers))
	}
	shown := map[int]bool{}  // stage → a passenger was drawn
	bubble := map[int]bool{} // stage → the "?" picture was drawn
	var lineStages []int
	dir := os.Getenv("SNAPSHOT_DIR")
	saved := false
	for i := 0; i < 60*240 && c.MoviePlaying(); i++ {
		*now = now.Add(time.Second / 60)
		c.Frame()
		if !c.MoviePlaying() {
			break
		}
		if _, _, _, _, ok := p.Fill(); ok && p.Overlay == 7 {
			red[p.Stage] = true
			if dir != "" && !saved && p.Stage == 1 {
				save(t, c, filepath.Join(dir, "movie_storm_red.png"))
				saved = true
			}
		}
		for _, n := range passengers {
			if c.movie.npcs[n].Shown {
				shown[p.Stage] = true
			}
		}
		for _, ps := range p.Pictures {
			if ps.Pic.Name == "S10238" && ps.Drawn(p.Stage) {
				bubble[p.Stage] = true
			}
		}
		if c.Talk.Shown() && c.Talk.Drawn() {
			lineStages = append(lineStages, p.Stage)
			if dir != "" && len(lineStages) == 1 {
				save(t, c, filepath.Join(dir, "movie_storm_question.png"))
			}
			c.GroundClick(400, 300)
		}
	}
	if c.MoviePlaying() {
		t.Fatalf("movie still at stage %d", p.Stage)
	}
	if fmt.Sprint(lineStages) != "[2 4 5 14]" {
		t.Errorf("lines shown in stages %v, want [2 4 5 14]", lineStages)
	}
	if len(bubble) != 1 || !bubble[2] {
		t.Errorf("the captain's bubble drawn in stages %v, want 2", bubble)
	}
	for _, s := range []int{1, 3, 5} {
		if !red[s] {
			t.Errorf("stage %d not red (red stages %v)", s, red)
		}
	}
	if red[2] || red[4] {
		t.Errorf("red outside its stages: %v", red)
	}
	if len(music) == 0 || music[0] != 3 || c.tableEntry(3) != `Sound\BGM0003.wav` {
		t.Errorf("music %v, entry 3 %q", music, c.tableEntry(3))
	}
	for s := range shown {
		if s < 17 || s > 18 {
			t.Errorf("passenger drawn in stage %d", s)
		}
	}
	if !shown[17] || !shown[18] {
		t.Errorf("passengers drawn in stages %v", shown)
	}
}

// The authored Starter Beach rescue is two movies, containing ten then six
// dialogue lines; the server must receive an acknowledgment for each movie.
func TestMonkeyRescueMovies(t *testing.T) {
	c, now, _ := enteredClient(t)
	capture := wire(t, c)
	for _, scenario := range []struct {
		id    uint32
		lines int
	}{{12000, 10}, {11011, 6}} {
		c.dispatch(movieFrameFor(scenario.id, 2))
		if !c.MoviePlaying() {
			t.Fatal("monkey movie did not start", scenario.id)
		}
		lines := 0
		for i := 0; i < 1200 && c.MoviePlaying(); i++ {
			*now = now.Add(100 * time.Millisecond)
			c.Frame()
			if c.Talk.Shown() && c.Talk.Drawn() {
				lines++
				c.GroundClick(400, 300)
			}
		}
		if c.MoviePlaying() {
			t.Fatalf("movie %d stuck at stage %d", scenario.id, c.movie.p.Stage)
		}
		if lines != scenario.lines {
			t.Fatalf("movie %d dialogue lines: got %d want %d", scenario.id, lines, scenario.lines)
		}
		c.Frame()
	}
	packets := capture()
	if len(packets) != 2 || !bytes.Equal(packets[0], []byte{20, 6}) || !bytes.Equal(packets[1], []byte{20, 6}) {
		t.Fatal("expected one completion acknowledgment per movie", packets)
	}
}

func TestMonkeyMovieDepthAndIllustration(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.dispatch(movieFrameFor(12000, 2))
	if c.movie == nil || len(c.movie.npcs) != 1 || c.movie.npcs[0].Depth != 100 {
		t.Fatal("monkey authored depth missing")
	}
	if n := c.movie.npcs[0]; n.SortY() != n.Y+100 {
		t.Fatal("monkey sort did not include native depth")
	}
	c.endMovie()
	c.dispatch(movieFrameFor(11011, 2))
	mp := c.movie
	if mp == nil || mp.background == nil || mp.background.W != 832 || mp.background.H != 640 || len(mp.view.Scene.Objects) != 0 {
		t.Fatal("full-screen illustration replaced by map")
	}
	// The talk box occupies the bottom of the image; verify the artwork above it.
	*now = now.Add(100 * time.Millisecond)
	c.Frame()
	for _, at := range []image.Point{{100, 100}, {400, 100}, {700, 200}} {
		if got, want := c.Screen.Pix[at.Y*c.Screen.W+at.X], mp.background.Pix[at.Y*mp.background.W+at.X]; got != want {
			t.Fatalf("illustration at %v: got %x want %x", at, got, want)
		}
	}
	if path := os.Getenv("MONKEY_ILLUSTRATION_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
}
