package app

import (
	"bytes"
	"encoding/binary"
	"fmt"
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
