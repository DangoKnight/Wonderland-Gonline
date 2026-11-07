package app

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/game"
)

func TestNotepadInventoryAndNativeState(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.dispatch([]byte{5, 4})
	sent := wire(t, c)
	c.InventoryState.Bag[0] = game.Item{ID: game.NotepadItemID, Count: 1}
	before := c.InventoryState.Bag
	c.Inventory.Show()
	c.Inventory.Use(1, false)
	if c.Notebook == nil || !c.Notebook.Visible || c.Inventory.Visible || c.InventoryState.Bag != before {
		t.Fatal("notepad must open without consumption")
	}
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{23, 96, 1}) {
		t.Fatal("notepad refresh request", got)
	}
	c.dispatch([]byte{24, 1, 204, 50, 1})
	c.dispatch([]byte{53, 9, 17, 39, 0, 0})
	if c.notebookState.Active[13004] != 1 || !c.notebookState.Discovered[10001] {
		t.Fatal("notebook packets ignored")
	}
	c.openStarBook()
	if !c.starBook.Visible || c.UI.Modal != c.starBook {
		t.Fatal("star book missing")
	}
	c.starBook.KeyDown(27, 0)
	if c.starBook.Visible || c.UI.Modal != nil {
		t.Fatal("star book did not close")
	}
}

func TestNotebookScreens(t *testing.T) {
	dir := os.Getenv("NOTEBOOK_SNAPSHOTS")
	if dir == "" {
		t.Skip("optional visual verification")
	}
	c, _, _ := enteredClient(t)
	c.loadNotebook()
	c.UI.HideAll()
	if c.Pics.Find("Form_NoteBook") < 0 || c.Pics.Find("form_taskview_1") < 0 || c.Pics.Find("Form_Cellection_1") < 0 {
		t.Fatal("missing native notebook skins")
	}
	c.notebookState.Apply([]byte{24, 1, 204, 50, 2})
	c.notebookState.Apply([]byte{24, 5, 18, 0, 1})
	for id, m := range c.notebookState.Monsters {
		if m.Index > 0 && m.Index < 30 {
			c.notebookState.Discovered[id] = true
		}
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	save := func(name string) {
		c.Screen.Fill(image.Rect(0, 0, 800, 600), 0x4328)
		c.UI.Draw()
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, c.Screen.RGBA()); err != nil {
			t.Fatal(err)
		}
	}
	c.Notebook.Show()
	save("menu")
	c.Notebook.Hide()
	c.Notebook.Quest.Selected = 0
	c.Notebook.Quest.Show()
	save("quests")
	c.Notebook.Quest.Hide()
	c.Notebook.Monster.Selected = 0
	c.Notebook.Monster.Show()
	save("monsters")
	c.Notebook.CloseAll()
	c.openStarBook()
	save("stars")
	t.Logf("star bounds: %v, sky: %d", c.starBook.Rect(), c.starBook.sky)
	c.starBook.Hide()
}
