package app

import (
	"bytes"
	"image"
	"testing"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
)

func TestInstanceArrowStateSlices(t *testing.T) {
	c, _, _ := enteredClient(t)
	for _, parent := range []seui.Control{c.Team.Instances, c.Team.Instances.New} {
		count := 0
		for _, control := range parent.Base().Children {
			b, ok := control.(*seui.FixedButton)
			if !ok || b.Image != c.Env.Pics.Find("Btn_ArrowL_5") && b.Image != c.Env.Pics.Find("Btn_ArrowR_5") {
				continue
			}
			count++
			if b.Width != 17 || b.Height != 17 {
				t.Fatalf("arrow size %dx%d", b.Width, b.Height)
			}
			for state := byte(0); state < 3; state++ {
				c.Screen.Fill(image.Rect(0, 0, 800, 600), 0x4208)
				want := c.Screen.Clone()
				at := b.Abs()
				c.Env.Pics.DrawRect(want, b.Image, at.X, at.Y, image.Rect(0, int(state)*17, 17, (int(state)+1)*17), true)
				b.State = state
				b.Paint()
				if !bytes.Equal(c.Screen.RGBA().Pix, want.RGBA().Pix) {
					t.Fatalf("arrow state %d leaks or shifts a row", state)
				}
				want.Close()
			}
		}
		if count != 2 {
			t.Fatalf("found %d arrows", count)
		}
	}
}

func TestJoinTeamPlayerAndPetHover(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	p := c.World.Player
	p.ID = 20002
	p.X += 90
	p.Name = []byte("Peer")
	human := role.NewHuman(c.lib, c.items)
	peer := &world.Peer{Player: p, Role: human}
	world.Dress(human, p)
	c.World.Peers = map[uint32]*world.Peer{p.ID: peer}
	c.World.Draw()
	cx, cy := c.World.Camera()
	find := func(hit func(int, int) bool) image.Point {
		for y := p.Y - cy - 250; y < p.Y-cy+50; y++ {
			for x := p.X - cx - 150; x < p.X-cx+150; x++ {
				if x >= 0 && y >= 0 && x < 800 && y < 600 && hit(x, y) {
					return image.Pt(x, y)
				}
			}
		}
		t.Fatal("no opaque actor pixel")
		return image.Point{}
	}
	at := find(func(x, y int) bool { return human.HitBody(p.X-cx, p.Y-cy, p.Direction, x, y) })
	c.setJoinTeamTarget(true)
	c.World.Hover(at.X, at.Y)
	c.World.Draw()
	if !human.Lit || c.World.TeamTargetAt(at.X, at.Y) != p.ID {
		t.Fatal("player hover not lit or selectable")
	}
	lit := c.Screen.RGBA()
	c.World.Hover(-1, -1)
	c.World.Draw()
	if human.Lit || bytes.Equal(lit.Pix, c.Screen.RGBA().Pix) {
		t.Fatal("player highlight did not clear or change pixels")
	}
	template := c.npcTemplates[17162]
	pet := role.NewNPC(c.lib, template.Look, template.Colors)
	c.World.SetCompanion(p.ID, 17162, []byte("S.Monkey"), pet)
	c.World.Companions[p.ID].Info = template
	playerAt := find(func(x, y int) bool {
		return human.HitBody(p.X-cx, p.Y-cy, p.Direction, x, y) &&
			!pet.HitSprite(p.X-cx, p.Y-cy+template.SpriteDrop(), 8+int(p.Direction)%8, x, y)
	})
	c.World.Hover(playerAt.X, playerAt.Y)
	c.World.Draw()
	if !human.Lit || !pet.Lit || c.World.TeamTargetAt(playerAt.X, playerAt.Y) != p.ID {
		t.Fatal("player hover did not highlight the player and following pet together")
	}
	petAt := find(func(x, y int) bool {
		return pet.HitSprite(p.X-cx, p.Y-cy+template.SpriteDrop(), 8+int(p.Direction)%8, x, y)
	})
	c.World.Hover(petAt.X, petAt.Y)
	c.World.Draw()
	if !pet.Lit || !human.Lit || c.World.TeamTargetAt(petAt.X, petAt.Y) != p.ID {
		t.Fatal("pet hover did not highlight the player and following pet together")
	}
	c.GroundClick(petAt.X, petAt.Y)
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{13, 1, 34, 78, 0, 0}) {
		t.Fatalf("pet join request %v", got)
	}
	c.World.Draw()
	if pet.Lit || human.Lit {
		t.Fatal("selection highlight survived mode exit")
	}
}
