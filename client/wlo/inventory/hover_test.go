package inventory

import (
	"image"
	"image/color"
	"slices"
	"testing"

	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/text"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
)

func TestInventoryHoverCoversWholeItemFootprint(t *testing.T) {
	font, err := clientassets.DecodeFont(make([]byte, (256+2*(clientassets.MaxGlyph+1))*clientassets.FontHeight))
	if err != nil {
		t.Fatal(err)
	}
	env := &seui.Env{Pics: picdb.New(), Screen: surface.New(800, 600), Text: &text.Renderer{Font: font}}
	seui.NewManager(env, &seui.Input{})
	state := &State{Items: map[uint16]assets.NativeItem{
		48016: {Icon: 42, Definition: game.ItemDefinition{Name: "Robinson Raft", CellWidth: 4, CellHeight: 3}},
		1:     {Definition: game.ItemDefinition{Name: "Other item"}},
	}}
	state.Bag[0] = game.Item{ID: 48016, Count: 1}
	state.Bag[4] = game.Item{ID: 1, Count: 2}
	f := NewForm(env, state, &world.Stats{})
	f.Visible = true
	icon := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	icon.SetNRGBA(1, 1, color.NRGBA{R: 248, G: 248, A: 255})
	env.Pics.Add("42", icon)
	before := state.Bag
	var expected []uint16
	for row := 0; row < 3; row++ {
		for col := 0; col < 4; col++ {
			env.Screen.Fill(image.Rect(0, 0, 800, 600), 0)
			f.Slots[row*BagColumns+col].Hint()
			if expected == nil {
				expected = slices.Clone(env.Screen.Pix)
			}
			if !slices.Equal(env.Screen.Pix, expected) || string(f.Slots[0].HintText) != "Robinson Raft" {
				t.Fatalf("covered cell (%d,%d) has different details/highlight", col, row)
			}
		}
	}
	a := f.Slots[0].Abs()
	if env.Screen.Pix[(a.Y+1)*800+a.X+1] != surface.RGB565(248, 248, 0) {
		t.Fatal("highlight covered item icon")
	}
	for row := 0; row < 3; row++ {
		for col := 0; col < 4; col++ {
			at := f.Slots[row*BagColumns+col].Abs().Add(image.Pt(10, 5))
			if env.Screen.Pix[at.Y*800+at.X] == 0 {
				t.Fatal("footprint cell not highlighted", row, col)
			}
		}
	}
	if state.Bag != before {
		t.Fatal("hover duplicated or changed item records")
	}
	if !f.Slots[0].HitTest(a.X+cellSize, a.Y+cellSize) || f.Slots[0].HitTest(a.X+4*cellStep, a.Y) {
		t.Fatal("item hit area misses its grid seams or includes its neighbor")
	}
	env.Screen.Fill(image.Rect(0, 0, 800, 600), 0)
	f.Slots[4].Hint()
	if string(f.Slots[4].HintText) != "Other item" {
		t.Fatal("neighbor resolved to raft")
	}
	at := f.Slots[4].Abs().Add(image.Pt(10, 5))
	if env.Screen.Pix[at.Y*800+at.X] == 0 {
		t.Fatal("single-cell item not highlighted")
	}
	env.Screen.Fill(image.Rect(0, 0, 800, 600), 0)
	f.Slots[19].Hint()
	if slices.ContainsFunc(env.Screen.Pix, func(p uint16) bool { return p != 0 }) {
		t.Fatal("empty cell showed item details")
	}
}
