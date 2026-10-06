package inventory

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/text"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
)

func presentationFixture() (*CompoundForm, *time.Time, *[][]byte) {
	env := &seui.Env{Pics: picdb.New(), Screen: surface.New(800, 600)}
	ui := seui.NewManager(env, &seui.Input{})
	bag := &Form{State: &State{Items: map[uint16]assets.NativeItem{7001: {Icon: 42}}, Bag: game.Inventory{{ID: 1, Count: 2}, {ID: 2, Count: 2}}}}
	bag.Env = env
	f := NewCompoundForm(env, bag)
	ui.Add(f)
	now := time.Unix(1000, 0)
	f.Now = func() time.Time { return now }
	var sent [][]byte
	f.Send = func(p []byte) error { sent = append(sent, bytes.Clone(p)); return nil }
	f.Show()
	f.SelectIngredient(1, 0)
	f.SelectIngredient(2, 1)
	icon := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			icon.SetNRGBA(x, y, color.NRGBA{R: 248, A: 255})
		}
	}
	env.Pics.Add("42", icon)
	return f, &now, &sent
}
func TestCompoundPresentationFlightAndAuthoritativeInventory(t *testing.T) {
	f, now, sent := presentationFixture()
	f.Synthesize()
	f.Synthesize()
	if len(*sent) != 1 {
		t.Fatal("duplicate request")
	}
	f.Inventory.State.Bag[1] = game.Item{ID: 7001, Count: 1}
	f.QueueResult(2)
	f.Result(7001, 1, 2)
	if f.Inventory.State.Bag[1].ID != 7001 || !f.Slots[1].item().Empty() {
		t.Fatal("must commit state and defer visual")
	}
	start, end, duration := f.resultPath()
	*now = now.Add(2800 * time.Millisecond)
	f.visual.Paint()
	a := f.Abs()
	if f.Env.Screen.Pix[(a.Y+start.Y)*800+a.X+start.X] == 0 {
		t.Fatal("missing launch icon")
	}
	*now = now.Add(duration / 2)
	clear(f.Env.Screen.Pix)
	f.visual.Paint()
	mid := image.Pt((start.X+end.X)/2, (start.Y+end.Y)/2)
	if f.Env.Screen.Pix[(a.Y+mid.Y+1)*800+a.X+mid.X+1] == 0 {
		t.Fatal("missing flight midpoint")
	}
	if !f.SelectIngredient(1, 0) {
		t.Fatal("flight blocked preparing next recipe")
	}
	*now = now.Add(duration/2 + time.Second)
	f.Refresh()
	if f.result != nil || f.Slots[1].item().ID != 7001 {
		t.Fatal("result not revealed at destination")
	}
	*now = now.Add(5 * time.Second)
	f.Refresh()
	if !f.SelectIngredient(2, 1) {
		t.Fatal("completed animation blocked synthesis")
	}
}
func TestCompoundPresentationCancellationDoesNotChangeItems(t *testing.T) {
	for _, mode := range []string{"hide", "replace", "reset"} {
		t.Run(mode, func(t *testing.T) {
			f, _, _ := presentationFixture()
			f.Synthesize()
			f.Inventory.State.Bag[1] = game.Item{ID: 7001, Count: 1}
			f.QueueResult(2)
			f.Result(7001, 1, 2)
			if mode == "replace" {
				f.Inventory.State.Bag[1] = game.Item{ID: 2, Count: 1}
				f.Refresh()
			} else if mode == "reset" {
				f.Reset()
			} else {
				f.Hide()
			}
			if f.result != nil || f.Inventory.State.Bag[1].Empty() {
				t.Fatal("cancel lost result or kept stale flight")
			}
		})
	}
}
func TestCompoundPresentationNoReplyRevealsCommittedReceipt(t *testing.T) {
	f, now, _ := presentationFixture()
	f.Synthesize()
	f.Inventory.State.Bag[1] = game.Item{ID: 7001, Count: 1}
	f.QueueResult(2)
	*now = now.Add(6 * time.Second)
	f.Paint()
	if f.pending || f.result != nil || f.Slots[1].item().ID != 7001 {
		t.Fatal("missing success reply hid inventory forever")
	}
}

func TestCompoundCauldronNativeFrameTiming(t *testing.T) {
	f, now, _ := presentationFixture()
	strip := image.NewNRGBA(image.Rect(0, 0, 1, 23))
	for y := 0; y < 23; y++ {
		strip.SetNRGBA(0, y, color.NRGBA{R: byte((y + 1) * 8), A: 255})
	}
	f.Env.Pics.Add("Compounding", strip)
	f.Synthesize()
	f.visual.Paint()
	a := f.Abs()
	index := (a.Y+compoundCauldronY-1)*800 + a.X + compoundCauldronX
	initial := f.Env.Screen.Pix[index]
	*now = now.Add(2800 * time.Millisecond)
	f.visual.Paint()
	if initial == 0 || f.Env.Screen.Pix[index] == initial {
		t.Fatal("cauldron did not advance to result frame")
	}
	*now = now.Add(2 * time.Second)
	f.visual.Paint()
	if f.Env.Screen.Pix[index] != initial {
		t.Fatal("cauldron did not finish native sequence")
	}
}

func TestCompoundTierRequestSignatures(t *testing.T) {
	for _, tc := range []struct {
		tier game.AlchemyTier
		code byte
	}{{game.AlchemyPrimary, 14}, {game.AlchemyJunior, 87}, {game.AlchemySuperior, 101}} {
		f, _, sent := presentationFixture()
		f.SkillLearned = func(uint16) bool { return true }
		if !f.SelectTier(tc.tier) {
			t.Fatal("tier selection")
		}
		f.Synthesize()
		if len(*sent) != 1 || !bytes.Equal((*sent)[0], []byte{23, tc.code, 2, 1, 2}) {
			t.Fatal("native tier request", *sent)
		}
		if f.SelectTier(game.AlchemyPrimary) {
			t.Fatal("tier changed during synthesis")
		}
	}
}

func TestCompoundNewAttemptInterruptsPresentation(t *testing.T) {
	f, now, sent := presentationFixture()
	f.Synthesize()
	f.Inventory.State.Bag[2] = game.Item{ID: 7001, Count: 1}
	f.QueueResult(3)
	f.Result(7001, 1, 3)
	*now = now.Add(time.Second)
	if !f.SelectIngredient(1, 0) || !f.SelectIngredient(2, 1) {
		t.Fatal("cannot prepare ingredients before result animation finishes")
	}
	if !f.Slots[2].item().Empty() {
		t.Fatal("result should still be animating")
	}
	f.Synthesize()
	if len(*sent) != 2 || !f.pending || f.sentAt != *now || f.result != nil || f.Slots[2].item().ID != 7001 {
		t.Fatal("new attempt did not reveal previous result and restart animation")
	}
	f.Synthesize()
	if len(*sent) != 2 {
		t.Fatal("duplicate outstanding request")
	}
}

func TestCompoundSelectionBackgroundAndRightClick(t *testing.T) {
	f, _, _ := presentationFixture()
	cell := f.Slots[0]
	font, err := clientassets.DecodeFont(make([]byte, (256+2*(clientassets.MaxGlyph+1))*clientassets.FontHeight))
	if err != nil {
		t.Fatal(err)
	}
	f.Env.Text = &text.Renderer{Font: font}
	cell.Paint()
	a := cell.Abs()
	pixel := a.Y*f.Env.Screen.W + a.X
	selected := f.Env.Screen.Pix[pixel]
	if selected == 0 {
		t.Fatal("missing selected background")
	}
	cell.RightUp(0, a.X, a.Y)
	if f.Selection[0] != 0 || !f.selectedItems[0].Empty() {
		t.Fatal("bag right-click did not deselect")
	}
	clear(f.Env.Screen.Pix)
	cell.Paint()
	if f.Env.Screen.Pix[pixel] != 0 {
		t.Fatal("deselection left red background")
	}
	if !f.SelectIngredient(1, 0) {
		t.Fatal("cannot reselect")
	}
	f.Ingredients[0].RightUp(0, 0, 0)
	if f.Selection[0] != 0 || !f.selectedItems[0].Empty() {
		t.Fatal("ingredient right-click did not deselect")
	}
}

func TestCompoundSingleLearnedTierHasNoChoice(t *testing.T) {
	for _, tc := range []struct {
		id   uint16
		tier game.AlchemyTier
		code byte
	}{
		{game.AlchemyPrimarySkill, game.AlchemyPrimary, 14},
		{game.AlchemyJuniorSkill, game.AlchemyJunior, 87},
		{game.AlchemySuperiorSkill, game.AlchemySuperior, 101},
	} {
		f, _, sent := presentationFixture()
		f.SkillLearned = func(id uint16) bool { return id == tc.id }
		f.Refresh()
		if f.Tier != tc.tier || f.AlchemyCommand() != tc.code || f.Junior.Visible || f.TierButton.Visible {
			t.Fatal("single learned skill requires automatic tier with no selector", tc)
		}
		for _, b := range f.TierOptions {
			if b.Visible {
				t.Fatal("single tier exposed menu")
			}
		}
		f.Synthesize()
		if (*sent)[0][1] != tc.code {
			t.Fatal("wrong automatic tier request")
		}
	}
}

func TestCompoundFailedSendPreservesPreviousPresentation(t *testing.T) {
	f, now, _ := presentationFixture()
	f.Synthesize()
	f.Inventory.State.Bag[2] = game.Item{ID: 7001, Count: 1}
	f.QueueResult(3)
	f.Result(7001, 1, 3)
	*now = now.Add(time.Second)
	previous := f.result
	started := f.sentAt
	f.SelectIngredient(1, 0)
	f.SelectIngredient(2, 1)
	f.Send = func([]byte) error { return errors.New("connection closed") }
	f.Synthesize()
	if f.result != previous || f.sentAt != started || f.pending || f.Selection[0] != 1 || f.Selection[1] != 2 {
		t.Fatal("failed send discarded old animation or prepared recipe")
	}
}

func TestCompoundTierMenuExcludesUnlearnedPrimary(t *testing.T) {
	f, _, _ := presentationFixture()
	f.SkillLearned = func(id uint16) bool { return id == game.AlchemyJuniorSkill || id == game.AlchemySuperiorSkill }
	f.Refresh()
	f.TierButton.Click()
	if f.Tier != game.AlchemyJunior || !f.TierButton.Visible || f.TierOptions[0].Visible || !f.TierOptions[1].Visible || !f.TierOptions[2].Visible {
		t.Fatal("menu must offer only learned skills")
	}
}
