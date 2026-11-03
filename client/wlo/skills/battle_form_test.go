package skills

import (
	"reflect"
	"testing"

	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/assets"
)

func TestBattleFormLearnedFilteringScrollingAndRecentActors(t *testing.T) {
	env := &seui.Env{Pics: picdb.New(), Screen: surface.New(800, 600)}
	seui.NewManager(env, &seui.Input{})
	catalog := &Catalog{Definitions: map[uint16]Definition{}}
	learned := map[uint16]Progress{}
	for id := uint16(101); id <= 110; id++ {
		catalog.Definitions[id] = Definition{Skill: assets.Skill{ID: id, Name: "Skill", Type: 1, TableOrder: id}}
		learned[id] = Progress{Grade: 1}
	}
	// Unlearned, non-combat and unordered definitions must not enter the list.
	learned[108] = Progress{}
	d := catalog.Definitions[109]
	d.Type = 8
	catalog.Definitions[109] = d
	d = catalog.Definitions[110]
	d.TableOrder = 0
	catalog.Definitions[110] = d
	browser := NewForm(env, catalog)
	browser.Targets = func() []Target { return []Target{{Skills: learned}, {Slot: 1, Skills: learned}} }
	f := NewBattleForm(env, browser)
	env.UI.Add(f)
	var actor byte
	var used uint16
	f.Use = func(id uint16, slot byte) { used, actor = id, slot }
	f.Show()
	if !reflect.DeepEqual(f.rows, []uint16{101, 102, 103, 104, 105, 106, 107}) || f.Recent.RowsShown != 3 || f.List.RowsShown != 5 {
		t.Fatal("battle list", f.rows, f.Recent.RowsShown, f.List.RowsShown)
	}
	f.Down.OnClick()
	f.Down.OnClick()
	f.Down.OnClick()
	if f.List.TopIndex != 2 {
		t.Fatal("scroll beyond final full page", f.List.TopIndex)
	}
	for _, i := range []int{0, 1, 2, 3, 1} {
		f.List.OnSelect(i)
	}
	f.Refresh()
	if !reflect.DeepEqual(f.recentRows, []uint16{102, 104, 103}) {
		t.Fatal("recent ordering/capacity", f.recentRows)
	}
	f.Selected = 1
	f.Show()
	if len(f.Recent.Items) != 0 {
		t.Fatal("pet inherited player recent skills")
	}
	f.List.OnSelect(0)
	if actor != 1 || used != 101 {
		t.Fatal("wrong skill actor", used, actor)
	}
	f.Selected = 0
	f.Show()
	delete(learned, 102)
	f.Refresh()
	if !reflect.DeepEqual(f.recentRows, []uint16{104, 103}) {
		t.Fatal("unlearned recent skill retained", f.recentRows)
	}
	f.Reset()
	f.Show()
	if len(f.Recent.Items) != 0 {
		t.Fatal("login reset retained recent skills")
	}
}
