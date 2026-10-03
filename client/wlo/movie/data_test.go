package movie

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wonderland-go/client/wlo/login"
)

var assets = login.NewAssets(filepath.Join("..", "..", "..", "data"))

func needAssets(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(assets.MediaPath("sty", styleExport)); err != nil {
		t.Skip("movie export not installed")
	}
}

// TestBeachMovie: 12008.sty (the beach rescue) has the player and Robinson
// (template 12032) for 22 stages, the camera on Map10003, and twelve lines
// alternating between them; the player says 20306 at stage 14.
func TestBeachMovie(t *testing.T) {
	needAssets(t)
	m, err := Load(assets, 12008)
	if err != nil {
		t.Fatal(err)
	}
	if m.Last != 22 || m.Player.Template != PlayerTemplate || len(m.Player.Keys) != 23 {
		t.Fatalf("last %d player %+v", m.Last, m.Player)
	}
	if len(m.NPCs) != 1 || m.NPCs[0].Template != 12032 || m.NPCs[0].Keys[1].X != 795 || m.NPCs[0].Keys[1].Y != 2272 {
		t.Fatalf("NPCs %+v", m.NPCs)
	}
	if m.Camera.Scene != "Map10003" || m.Camera.Keys[0].X != 785 || m.Camera.Keys[0].Y != 2221 {
		t.Fatalf("camera %+v", m.Camera)
	}
	if len(m.Lines) != 12 {
		t.Fatalf("%d lines", len(m.Lines))
	}
	found := false
	for _, l := range m.Lines {
		if l.Stage == 15 && l.Speaker == PlayerTemplate && l.Talk == 20306 {
			found = true
		}
	}
	if !found {
		t.Fatalf("lines %+v", m.Lines)
	}
}

// TestAllMovies: every exported movie joins into whole keyframes.
func TestAllMovies(t *testing.T) {
	needAssets(t)
	if _, err := entry(assets, "123.sty"); err != nil {
		t.Fatal(err)
	}
	bad := 0
	for name := range styles.raw {
		if !strings.HasSuffix(name, ".sty") {
			continue
		}
		if _, err := decode(styles.raw[name]); err != nil {
			bad++
			t.Errorf("%s: %v", name, err)
		}
	}
	if bad > 0 {
		t.Fatalf("%d of %d movies failed", bad, len(styles.raw))
	}
}

// TestStormLines: the storm's lines are records 1..4 of [speaker, talk,
// stage, delay]: the captain asks what is the matter in stage 2, after the
// first red stage, and the crewman reports in stage 4, as the video shows.
func TestStormLines(t *testing.T) {
	needAssets(t)
	m, err := Load(assets, 123)
	if err != nil {
		t.Fatal(err)
	}
	want := []Line{{Stage: 2, Delay: 500, Speaker: 14003, Talk: 30115}, {Stage: 4, Delay: 800, Speaker: 14002, Talk: 30159},
		{Stage: 5, Delay: 500, Speaker: 14003, Talk: 30160}, {Stage: 14, Delay: 500, Speaker: 14003, Talk: 30249}}
	if len(m.Lines) != len(want) {
		t.Fatalf("lines %+v", m.Lines)
	}
	for i, l := range want {
		if m.Lines[i] != l {
			t.Fatalf("line %d %+v, want %+v", i, m.Lines[i], l)
		}
	}
}
