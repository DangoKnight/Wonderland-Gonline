package app

import (
	"bytes"
	"image"
	"os"
	"strings"
	"testing"

	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/movie"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
)

func TestDialogueVoicesAndOGG(t *testing.T) {
	c, _, _ := enteredClient(t)
	if bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE"); bundle != "" {
		useCompiledPetAssets(t, c, bundle)
		c.npcTemplates, _ = world.NPCTemplates(c.Assets)
	}
	c.World.Player.Body, c.World.Player.Head = 2, 0
	var played []string
	c.Env.Sound = func(path string) {
		if strings.HasSuffix(path, ".ogg") {
			played = append(played, path)
		}
	}
	c.movieSay(&moviePlay{}, movie.Line{Talk: 20038, Speaker: movie.PlayerTemplate})
	c.movieSay(&moviePlay{}, movie.Line{Talk: 20039, Speaker: 17162})
	want := []string{`..\audio\odd\20038_1002.ogg`, `..\audio\odd\20039.ogg`}
	if len(played) != len(want) {
		t.Fatalf("voices %v", played)
	}
	sounds := &Sounds{Root: c.Assets.Media}
	for i, path := range played {
		if path != want[i] {
			t.Fatalf("voice %q, want %q", path, want[i])
		}
		if pcm := sounds.PCM(path); len(pcm) == 0 {
			t.Fatalf("no decoded audio for %s", path)
		}
	}
}

func TestPetRecruitmentAnnouncement(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	var sounds []string
	c.Env.Sound = func(path string) { sounds = append(sounds, path) }
	before := [game.MaxPets]inventory.UsePet{}
	c.InventoryState.Pets[0] = inventory.UsePet{ID: 17162, Name: []byte("S.Monkey")}
	c.mapReady = false
	c.announceNewPets(before)
	if len(c.petAnnouncements) != 0 || len(sounds) != 0 {
		t.Fatal("announced initial roster")
	}
	c.mapReady = true
	c.event.active = true
	c.announceNewPets(before)
	c.eventTick()
	if c.Talk.Shown() {
		t.Fatal("announcement interrupted the event")
	}
	c.eventResume()
	c.eventTick()
	if !c.Talk.Shown() || string(bytes.Join(c.Talk.Lines(), nil)) != "S.Monkey joins the party" {
		t.Fatalf("announcement %q", c.Talk.Lines())
	}
	c.advanceTalk()
	c.eventTick()
	if c.Talk.Shown() || len(sent()) != 0 {
		t.Fatal("announcement sent event acknowledgment")
	}
	before = c.InventoryState.Pets
	c.InventoryState.Pets[1] = c.InventoryState.Pets[0]
	c.InventoryState.Pets[0] = inventory.UsePet{}
	c.announceNewPets(before)
	if len(c.petAnnouncements) != 0 || len(sounds) != 1 || sounds[0] != `sound\wav0152.wav` {
		t.Fatalf("duplicate notice/sound %v", sounds)
	}
}

func TestNPCTooFar(t *testing.T) {
	c, _, legs := enteredClient(t)
	sent := wire(t, c)
	n := &world.NPC{X: c.World.Player.X + npcReach + 1, Y: c.World.Player.Y}
	c.clickNPC(n)
	if len(*legs) != 0 || len(sent()) != 0 || c.event.active || c.pendingNPC != nil {
		t.Fatal("distant interaction walked or sent click")
	}
	if !c.Notices.shown || len(c.Notices.items) == 0 || string(c.Notices.items[len(c.Notices.items)-1].text) != "Too far" {
		t.Fatal("range notice missing")
	}
}

func TestMonkeyDustUsesAuthoredLight(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.dispatch(movieFrameFor(12000, 2))
	mp := c.movie
	if mp == nil {
		t.Fatal("monkey movie missing")
	}
	for _, ps := range mp.p.Pictures {
		if ps.Pic.Level() == movie.LevelKeyed {
			continue
		}
		mp.p.Stage = ps.Pic.Start() + 1
		mp.p.Pictures = []*movie.PictureState{ps}
		img := c.picture(mp, ps.Pic)
		if img == nil {
			t.Fatalf("missing picture %s", ps.Pic.Name)
		}
		rows, cols := ps.Pic.Grid()
		w, h := img.W/cols, img.H/rows
		pt := ps.Point()
		// Put the picture entirely on screen, over an independently chosen ground.
		mp.p.Camera = image.Pt(pt.X-w/2, pt.Y-h)
		c.Screen.Fill(image.Rect(0, 0, c.Screen.W, c.Screen.H), 0x4208)
		want := c.Screen.Clone()
		defer want.Close()
		r := image.Rect(0, (ps.Frame-1)*h, w, ps.Frame*h)
		want.DrawLight(0, 0, r, img, ps.Pic.Level())
		c.drawPictures(mp)
		if !bytes.Equal(c.Screen.RGBA().Pix, want.RGBA().Pix) {
			t.Fatal("dust lost authored additive light")
		}
		keyed := want.NewCompatible(c.Screen.W, c.Screen.H)
		defer keyed.Close()
		keyed.Fill(image.Rect(0, 0, keyed.W, keyed.H), 0x4208)
		keyed.DrawRect(0, 0, r, img, true)
		if bytes.Equal(c.Screen.RGBA().Pix, keyed.RGBA().Pix) {
			t.Fatal("fixture does not distinguish additive from opaque drawing")
		}
		return
	}
	t.Fatal("monkey movie has no authored light picture")
}
