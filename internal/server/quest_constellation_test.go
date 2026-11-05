package server

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestInstalledQuestConstellationRewards(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	s, c, _ := eventFixture(t)
	s.Assets, s.World = catalog, world.New(catalog)
	for _, mark := range []uint32{13087, 12011, 13071, 13173, 15145, 13203, 13099, 16045, 13151} {
		c.character.Quests[mark] = game.Quest{ID: mark, State: game.InProgress, Step: 1}
	}
	if got := s.storyConstellations(c.character); !bytes.Equal(got, []byte{15, 19, 10, 1, 3, 4, 5, 6, 7, 8, 9, 19, 20}) {
		t.Fatal("incomplete native constellation rewards", got)
	}
	// Both bear-quest stars share one receipt, and removed receipts grant none.
	q := c.character.Quests[12011]
	q.State = game.Completed
	c.character.Quests[12011] = q
	if got := s.storyConstellations(c.character); !bytes.Equal(got, []byte{15, 19, 8, 1, 5, 6, 7, 8, 9, 19, 20}) {
		t.Fatal("removed constellation receipt remained visible", got)
	}
	for i, r := range catalog.Story.Rules {
		if r.Kind == assets.StoryConstellation && r.Value == 5 {
			catalog.Story.Rules[i].Value = 15
		}
	}
	s.World = world.New(catalog)
	if got := s.storyConstellations(c.character); !bytes.Equal(got, []byte{15, 19, 8, 1, 6, 7, 8, 9, 15, 19, 20}) {
		t.Fatal("constellations ignored SQL content definition", got)
	}
}
