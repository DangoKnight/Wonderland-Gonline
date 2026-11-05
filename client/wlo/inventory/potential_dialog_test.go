package inventory

import (
	"bytes"
	"strings"
	"testing"

	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
)

func TestInventoryPotentialButtonOpensModuleWithoutPills(t *testing.T) {
	env := &seui.Env{Pics: picdb.New()}
	seui.NewManager(env, &seui.Input{})
	f := NewForm(env, &State{}, &world.Stats{})
	env.UI.Add(f)
	f.Show()
	f.Potential.OnClick()
	if f.UseDialog == nil || !f.UseDialog.Visible || env.UI.Modal != f.UseDialog || f.UseDialog.Confirm.Visible {
		t.Fatal("button must open potential with no pills and disable confirmation")
	}
	f.UseDialog.Hide()
	f.State.Pets[0] = UsePet{ID: 17004}
	f.Selected = 1
	f.State.Bag[2] = game.Item{ID: game.ItemPotentialPill, Count: 2}
	f.Potential.OnClick()
	if f.UseDialog.target != 1 || !f.UseDialog.Confirm.Visible {
		t.Fatal("selected pet or pills missing")
	}
	f.SetMode(2)
	if f.Potential.Visible {
		t.Fatal("potential button visible in bag-only layout")
	}
}

func TestPotentialSkillWarningRequiresExplicitConfirmation(t *testing.T) {
	f, sent := useFixture(game.ItemPotentialPill)
	f.Env = &seui.Env{Pics: picdb.New()}
	seui.NewManager(f.Env, &seui.Input{})
	f.PotentialSkillsAtRisk = func() []string { return []string{"Sword Awn Attack", "Poison Spell"} }
	f.openUse(1, f.State.Bag[0])
	d := f.UseDialog
	d.SelectTarget(0)
	d.confirm()
	w := d.warning
	if w == nil || len(*sent) != 0 || f.Env.UI.Modal != w || !strings.Contains(w.text, "Poison Spell") {
		t.Fatal("missing warning or pill sent before approval")
	}
	w.KeyDown(0x1b, 0)
	if len(*sent) != 0 || d.warning != nil || f.Env.UI.Modal != d {
		t.Fatal("cancel consumed pill or lost modal")
	}
	d.confirm()
	d.warning.Confirm.OnClick()
	if len(*sent) != 1 || !bytes.Equal((*sent)[0], []byte{23, 126, 1, 0}) {
		t.Fatal("confirmation did not send exactly one request", *sent)
	}
	if f.State.Bag[0].Count != 3 {
		t.Fatal("pill consumed locally")
	}
	f.PotentialReply([]byte{23, 213, 1, 0, 4})
	d.confirm()
	d.Hide()
	if d.warning != nil || f.Env.UI.Modal != nil {
		t.Fatal("closing potential left warning active")
	}
}

func TestPotentialDialogCountBonusesAndPendingSelection(t *testing.T) {
	f, sent := useFixture(game.ItemPotentialPill)
	f.Env = &seui.Env{Pics: picdb.New()}
	seui.NewManager(f.Env, &seui.Input{})
	f.State.Bag[4] = game.Item{ID: game.ItemPotentialPill, Count: 2}
	f.State.Bag[5] = game.Item{ID: game.ItemPotentialPill, Count: 50, Locked: true}
	f.State.Pets[2] = UsePet{ID: 12178, Potential: 2, Stats: world.Stats{STR: 8, CON: 10, INT: 6, WIS: 4, AGI: 5}}
	f.openUse(1, f.State.Bag[0])
	d := f.UseDialog
	if d.Confirm.Visible || d.pillCount() != 5 || d.Targets[1].target != 3 {
		t.Fatal("initial selection, usable count or sparse roster")
	}
	d.SelectTarget(3)
	if d.base != ([5]uint16{6, 8, 4, 2, 3}) || !d.Confirm.Visible {
		t.Fatal("base attributes must exclude existing potential", d.base)
	}
	d.confirm()
	d.SelectTarget(0)
	d.confirm()
	if len(*sent) != 1 || !bytes.Equal((*sent)[0], []byte{23, 126, 1, 3}) || d.target != 3 || d.Confirm.Visible || !d.Visible {
		t.Fatal("pending request changed target, duplicated or closed window", *sent)
	}
	f.State.Apply([]byte{23, 9, 1, 1})
	f.PotentialReply([]byte{23, 213, 1, 3, 3})
	d.syncTargets()
	if d.pillCount() != 4 || !d.Confirm.Visible || d.base[0] != 6 || f.State.Pets[2].Potential != 3 {
		t.Fatal("result changed base stats or failed to refresh count")
	}
	f.State.Pets[2].ID = 14156
	d.confirm()
	if d.selected || len(*sent) != 1 {
		t.Fatal("replacement pet reused the old selection")
	}
	d.Hide()
	if f.Env.UI.Modal != nil || d.Visible {
		t.Fatal("cancel kept modal input lock")
	}
}
