package app

import (
	"image"
	"os"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/team"
	"wonderland-gonline/client/wlo/world"
)

func TestTeamApplicantPresentation(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	p := c.World.Player
	p.ID, p.Name = 20002, []byte("Test0002")
	c.World.Peers = map[uint32]*world.Peer{p.ID: {Player: p}}
	c.rememberPlayer(p.ID, p.Name)
	var at image.Point
	var action int
	draw := c.Team.DrawPlayerBody
	c.Team.DrawPlayerBody = func(id uint32, x, y, pose int) {
		if id != p.ID {
			t.Fatal("request displays the wrong applicant")
		}
		at, action = image.Pt(x, y), pose
		draw(id, x, y, pose)
	}
	c.dispatch([]byte{13, 1, 34, 78, 0, 0})
	*now = now.Add(time.Second)
	c.Frame()
	modal := c.UI.Modal
	if modal == nil || modal.Base().Width != 346 || modal.Base().Height != 185 {
		t.Fatal("request does not use the reference panel dimensions")
	}
	if at != modal.Base().Abs().Add(image.Pt(42, 100)) || action != 13 {
		t.Fatal("applicant body must stand at the left, independently of its world pose", at, action)
	}
	for _, child := range modal.Base().Children {
		if label, ok := child.(*seui.Editor); ok && string(label.Text) != "Please let me join you!" {
			t.Fatal("applicant name must be separate from the request message", string(label.Text))
		}
	}
	if path := os.Getenv("TEAM_APPLICANT_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
	acceptTeamPrompt(t, c)
}

func TestTeamMemberNativeGauges(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.TeamState.Leader, c.TeamState.Members = c.World.Player.ID, []uint32{c.World.Player.ID, 20002}
	c.TeamState.Vitals[20002] = &team.Member{ID: 20002, Stats: world.Stats{Level: 9, Element: 3, HP: 50, MaxHP: 100, SP: 25, MaxSP: 100}}
	c.Team.Left, c.Team.Top = 430, 0
	c.Team.Show()
	statsOf := c.Team.StatsOf
	c.Team.StatsOf = nil
	c.Team.Paint()
	want := c.Screen.Clone()
	defer want.Close()
	a := c.Team.Abs()
	for _, gauge := range []struct {
		frame, fill string
		y, divisor  int
	}{{"Panel34_1", "Panel31_1", 284, 2}, {"Panel35_1", "Panel32_1", 298, 4}} {
		frame, fill := c.Env.Pics.Find(gauge.frame), c.Env.Pics.Find(gauge.fill)
		fw, fh := c.Env.Pics.Size(frame)
		w, h := c.Env.Pics.Size(fill)
		if fw == 0 || fh == 0 || w == 0 || h == 0 {
			t.Fatal("missing native teammate gauge artwork", gauge.frame)
		}
		x, y := a.X+130, a.Y+gauge.y
		c.Env.Pics.Draw(want, frame, x, y, true)
		c.Env.Pics.DrawRect(want, fill, x+2, y+2, image.Rect(0, 0, w/gauge.divisor, h), true)
	}
	c.Team.StatsOf = statsOf
	c.Team.Paint()
	for _, y := range []int{284, 298} {
		for dy := 0; dy < 9; dy++ {
			for dx := 0; dx < 65; dx++ {
				i := (a.Y+y+dy)*c.Screen.W + a.X + 130 + dx
				if c.Screen.Pix[i] != want.Pix[i] {
					t.Fatalf("teammate gauge differs from native artwork at %d,%d", dx, dy)
				}
			}
		}
	}
	if path := os.Getenv("TEAM_MEMBER_SNAPSHOT"); path != "" {
		c.Frame()
		savePNG(t, path, c)
	}
}
