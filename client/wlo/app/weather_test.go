package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/movie"
	"wonderland-gonline/client/wlo/weather"
)

// TestSceneWeather: Hilltop Hot Spring (scene 11089, weather byte 3)
// steams, Frost Peak (12469, 2) snows, Ship Deck has none; after a few
// seconds the steam is drawn over the map. With SNAPSHOT_DIR set the
// frames are saved for viewing.
func TestSceneWeather(t *testing.T) {
	c, now, _ := enteredClient(t)
	if c.World.WeatherKind != weather.None {
		t.Fatalf("Ship Deck weather %d", c.World.WeatherKind)
	}
	for _, m := range []struct {
		id   uint16
		kind weather.Kind
	}{{12469, weather.Snow}, {11089, weather.Steam}} {
		c.dispatch(selfPacket(10001, 2, m.id, 600, 500, 0, 444444444, 444444444, nil, "Dango"))
		if c.World.WeatherKind != m.kind {
			t.Fatalf("map %d weather %d, want %d", m.id, c.World.WeatherKind, m.kind)
		}
	}
	c.Frame()
	plain := append([]uint16(nil), c.Screen.Pix...)
	for i := 0; i < 200; i++ {
		*now = now.Add(weather.Frame)
		c.Frame()
	}
	differ := 0
	for i, v := range c.Screen.Pix {
		if v != plain[i] {
			differ++
		}
	}
	if differ == 0 {
		t.Fatal("no steam drawn")
	}
	if dir := os.Getenv("SNAPSHOT_DIR"); dir != "" {
		save(t, c, filepath.Join(dir, "hot_spring_steam.png"))
		for i := 0; i < 3; i++ {
			*now = now.Add(time.Second)
			c.Frame()
			save(t, c, filepath.Join(dir, "hot_spring_steam_"+string(rune('a'+i))+".png"))
		}
	}
}

// TestSnowMovie: 11012.sty's effect 4 turns on overlay 2, and the movie's
// view snows in place of the map's weather.
func TestSnowMovie(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.dispatch(movieFrameFor(11012, 1))
	if !c.MoviePlaying() {
		t.Fatal("no movie")
	}
	snowing := 0
	for i := 0; i < 60*240 && c.MoviePlaying(); i++ {
		*now = now.Add(time.Second / 60)
		c.Frame()
		if !c.MoviePlaying() {
			break
		}
		if c.movie.p.Overlay == movie.OverlaySnow {
			if c.movie.view.WeatherKind != weather.Snow {
				t.Fatalf("overlay 2 shows weather %d", c.movie.view.WeatherKind)
			}
			snowing++
		}
		if c.Talk.Shown() && c.Talk.Drawn() {
			c.GroundClick(400, 300)
		}
	}
	if snowing == 0 {
		t.Fatal("the movie never snowed")
	}
}
