package app

import (
	"slices"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/world"
)

type transformAppearance struct {
	login.RoleView
	slot  login.CharacterSlot
	calls int
}

func (r *transformAppearance) SetCharacter(slot *login.CharacterSlot) {
	r.slot = *slot
	r.calls++
}

func TestBreillatModelSwitchPackets(t *testing.T) {
	c, _, _ := enteredClient(t)
	// Independent bytes from the legacy-client/Gonline relay capture.
	c.dispatch(selfPacket(0x0044d132, 4, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "Dango03"))
	w := c.World
	body, preview := &transformAppearance{}, &transformAppearance{}
	w.Body, c.MainStatus.Portrait, c.Inventory.Preview = body, body, preview
	c.Stats.HP = 123
	skillRevision := c.SkillState.Revision
	c.dispatch([]byte{6, 2, 1})
	c.dispatch([]byte{5, 12, 50, 209, 68, 0, 13})
	if w.Player.Body != 4 || w.Player.Head != 3 || body.slot.Body != 4 || body.slot.Head != 3 || preview.slot.Head != 3 {
		t.Fatal("model switch did not immediately update world, portrait and inventory", w.Player, body.slot, preview.slot)
	}
	if c.World != w || c.MainStatus.Portrait != body || c.Stats.HP != 123 || !c.held {
		t.Fatal("model switch reset the world, vitals or event hold")
	}
	if c.SkillState.Revision != skillRevision {
		t.Fatal("model packet changed skill progress")
	}
	// Equipment and event completion follow the model packet, without AC3/AC12.
	c.dispatch([]byte{23, 11, 247, 85, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 17, 82, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 18, 39, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 201, 93, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	c.dispatch([]byte{20, 10})
	c.dispatch([]byte{6, 2, 0})
	c.dispatch([]byte{20, 8})
	if c.held || c.event.active || w.Player.Head != 3 || body.slot.Head != 3 || body.slot.Equipment[1] != 22007 {
		t.Fatal("equipment or event completion lost the model switch", body.slot)
	}
	for _, p := range [][]byte{{5, 12}, {5, 12, 50, 209, 68, 0}, {5, 12, 50, 209, 68, 0, 0}, {5, 12, 50, 209, 68, 0, 255}} {
		c.dispatch(p)
	}
	if w.Player.Head != 3 {
		t.Fatal("invalid model mutated appearance")
	}
	peerBody, teamBody := &transformAppearance{}, &transformAppearance{}
	w.Peers = map[uint32]*world.Peer{20002: {Player: world.Player{ID: 20002, Body: 1}, Role: peerBody}}
	c.teamAppearances = map[uint32]*world.Peer{20002: {Player: world.Player{ID: 20002, Body: 1}, Role: teamBody}}
	c.dispatch([]byte{5, 12, 34, 78, 0, 0, 13})
	if w.Peers[20002].Head != 3 || c.teamAppearances[20002].Head != 3 || peerBody.slot.Head != 3 || teamBody.slot.Head != 3 {
		t.Fatal("peer or cached teammate kept the old model")
	}
	c.dispatch([]byte{5, 12, 8, 43, 4})
	if c.SkillState.Learned[11016].Grade != 4 || w.Player.Head != 3 {
		t.Fatal("compact skill-grade packet was mistaken for a model change")
	}
}

func TestBreillatModelSwitchRendering(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.dispatch(selfPacket(0x0044d132, 4, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "Dango03"))
	human := c.World.Body.(*role.Human)
	human.Now = func() time.Time { return c.Now() }
	render := func(h login.RoleView) *surface.Surface {
		dst := surface.New(240, 300)
		h.DrawBody(dst, 120, 250, 11)
		h.DrawPortrait(dst, 120, 80, false)
		return dst
	}
	before := render(human)
	c.dispatch([]byte{5, 12, 50, 209, 68, 0, 13})
	got := render(human)
	expected := role.NewHuman(c.lib, c.items)
	expected.Now = func() time.Time { return c.Now() }
	expected.SetCharacter(&login.CharacterSlot{Body: 4, Head: 3, Color1: 444444444, Color2: 444444444})
	if slices.Equal(before.Pix, got.Pix) || !slices.Equal(got.Pix, render(expected).Pix) {
		t.Fatal("world sprite/portrait did not render Breillat immediately")
	}
}
