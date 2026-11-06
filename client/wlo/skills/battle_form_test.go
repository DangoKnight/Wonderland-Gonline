package skills

import (
	"fmt"
	"reflect"
	"testing"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestCharacterStuntsUseLearnedAliasInBothWindows(t *testing.T) {
	catalog, err := Load(login.NewAssets("../../../data"))
	if err != nil {
		t.Fatal(err)
	}
	for body, heads := range map[uint16]int{1: 1, 2: 2, 3: 4, 4: 8} {
		for head := 0; head < heads; head++ {
			t.Run(fmt.Sprintf("%d/%d", body, head), func(t *testing.T) {
				env := &seui.Env{Pics: picdb.New(), Screen: surface.New(800, 600)}
				seui.NewManager(env, &seui.Input{})
				id := game.StarterStunt(body, uint16(head))
				d := catalog.Definitions[id]
				if d.ID == 0 || d.TableOrder != 0 {
					t.Fatal("unexpected native stunt record", id, d.TableOrder)
				}
				learned := map[uint16]Progress{game.StarterStuntClientID: {Grade: 1}}
				browser := NewForm(env, catalog)
				browser.Targets = func() []Target { return []Target{{Skills: learned, Stunt: id}} }
				for tab := Physical; tab <= Life; tab++ {
					if category(d, tab) {
						browser.SetTab(tab)
						break
					}
				}
				if len(browser.rows) == 0 || browser.rows[len(browser.rows)-1] != game.StarterStuntClientID {
					t.Fatal("unique skill missing from browser", id, browser.rows)
				}
				f := NewBattleForm(env, browser)
				env.UI.Add(f)
				var requestedIcon string
				browser.LoadPictures = func(names ...string) { requestedIcon = names[0] }
				f.Show()
				if len(f.rows) != 1 || f.rows[0] != game.StarterStuntClientID || string(f.List.Items[0]) != d.Name {
					t.Fatal("unique skill missing or wrongly named", id, f.rows, f.List.Items)
				}
				if requestedIcon != d.IconName() {
					t.Fatal("wrong unique skill icon", id)
				}
				var selected uint16
				f.Use = func(skill uint16, _ byte) { selected = skill }
				f.List.OnSelect(0)
				f.Refresh()
				if selected != game.StarterStuntClientID || len(f.recentRows) != 1 {
					t.Fatal("stunt alias or recent selection lost", selected)
				}
				delete(learned, game.StarterStuntClientID)
				f.Refresh()
				if len(f.rows) != 0 || len(f.recentRows) != 0 {
					t.Fatal("unlearned stunt exposed")
				}
			})
		}
	}
}

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
