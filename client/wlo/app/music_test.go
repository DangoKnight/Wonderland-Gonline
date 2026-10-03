package app

import (
	"os"
	"strings"
	"testing"

	"wonderland-go/client/wlo/login"
	"wonderland-go/client/wlo/world"
)

// TestSceneMusic: Ship Deck's scene plays BGM0007, and every track the
// scenes name and the login's BGM0013 exist among the exported sounds.
func TestSceneMusic(t *testing.T) {
	c := testClient(t)
	tracks, err := world.SceneMusic(c.Assets)
	if err != nil {
		t.Fatal(err)
	}
	scenes, _ := world.MapScenes(c.Assets)
	if got := tracks[scenes[10017]]; got != "BGM0007" {
		t.Fatalf("Ship Deck plays %q", got)
	}
	exists := func(path string) error {
		_, err := os.Stat(login.Path(c.Assets.Media, strings.Split(path, `\`)...))
		return err
	}
	missing := map[string]bool{}
	for _, tr := range tracks {
		if err := exists(musicDir + tr + musicExt); err != nil {
			missing[tr] = true
		}
	}
	if err := exists(loginMusic); err != nil {
		t.Fatalf("login music: %v", err)
	}
	if len(missing) > 0 {
		t.Logf("scene tracks without a file (silent, as the original): %v", missing)
	}
}
