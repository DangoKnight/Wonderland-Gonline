package app

import (
	"image"
	"strconv"
	"time"
	"wonderland-gonline/client/wlo/notebook"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/localization"
)

const monsterBookPreviewAction = 13 // TTH_CellectionForm FUN_001db4e0: actor +0x121 = 0x0d.

func (c *Client) initNotebook() {
	c.loadNotebookPictures()
	c.notebookState.Reset()
	c.Notebook = notebook.New(c.Env, &c.notebookState)
	c.starBook = newStarBook(c)
	c.Notebook.Star = c.openStarBook
	c.Inventory.OpenNotebook = func() {
		if !c.canOpenModule() {
			return
		}
		c.loadNotebook()
		c.Notebook.Show()
	}
	previews := map[uint16]*role.NPC{}
	c.Notebook.Monster.Preview = func(id uint16, x, y int) {
		if c.lib == nil {
			return
		}
		if c.npcTemplates == nil {
			c.npcTemplates, _ = world.NPCTemplates(c.Assets)
		}
		t, ok := c.npcTemplates[uint32(id)]
		if !ok {
			return
		}
		sprite := previews[id]
		if sprite == nil {
			sprite = role.NewNPC(c.lib, t.Look, t.Colors)
			sprite.Now = func() time.Time { return c.Now() }
			previews[id] = sprite
		}
		sprite.Draw(c.Screen, x, y, monsterBookPreviewAction)
	}
}
func (c *Client) loadNotebookPictures() {
	c.loadPictures("Form_NoteBook", "form_taskview_1", "Form_Cellection_1", "Btn_MissionBook", "Btn_MonsterBook", "Btn_StarBook", "Btn_Close_1", "Btn_Close_s_1", "Btn_ArrowL_8", "Btn_ArrowR_8", "Arrow_L3", "Arrow_R3", "icon_Check", "Btn_Minimize_1")
	for i := 1; i <= 4; i++ {
		c.loadPictures("Icon_Element_" + strconv.Itoa(i) + "_1")
	}
}
func (c *Client) loadNotebook() {
	c.loadNotebookPictures()
	if c.notebookLanguage != localization.CurrentLanguage() || c.notebookState.Marks == nil {
		if err := c.notebookState.Load(c.Assets.DataPath("mark_data.json"), c.Assets.DataPath("npc_data.json")); err != nil {
			if c.Chat != nil {
				c.Chat.Notice(err.Error())
			}
		} else {
			c.notebookLanguage = localization.CurrentLanguage()
		}
	}
}

type starBookForm struct {
	seui.Form
	client *Client
	opened time.Time
	music  string
	sky    int
}

func newStarBook(c *Client) *starBookForm {
	f := &starBookForm{client: c, sky: -1}
	f.InitForm(f, c.Env)
	f.Name = "CH_StarSystem"
	f.Draggable, f.Dockable = false, false
	f.Init("", 0, 0, 0, 0, 0, false, 600, 800, 0)
	close := seui.NewFixedButton(c.Env, f)
	close.Init("Btn_Minimize_1", 780, 18, 19, 0, 0, true, 18, 19, 10)
	close.OnClick = f.Hide
	c.UI.Add(f)
	return f
}
func (c *Client) openStarBook() {
	c.loadPictures("Star_Sky", "Btn_Minimize_1")
	if c.Pics.Find("Star_Sky") < 0 {
		if compiled, err := c.Assets.CompiledPicture(constellationSkyArchive, constellationSkyPicture); err == nil {
			_ = c.Pics.AddCompiled(constellationSkyPicture, compiled)
		} else if picture, err := c.Assets.LoadPicture(constellationSkyArchive, constellationSkyPicture); err == nil {
			c.Pics.Add(constellationSkyPicture, picture)
		}
	}
	for id, owned := range c.constellations {
		if owned {
			c.loadPictures("Star_" + strconv.Itoa(id))
		}
	}
	f := c.starBook
	f.sky = c.Pics.Find("Star_Sky")
	f.opened = c.Now()
	if c.World != nil {
		c.World.StopWalk()
	}
	if c.Music != nil {
		c.Music.mu.Lock()
		f.music = c.Music.current
		c.Music.mu.Unlock()
		c.Music.Play(constellationMusic)
	}
	f.Show()
	c.UI.SetModal(true, f)
}
func (f *starBookForm) Hide() {
	if f.Env.UI.Modal == f {
		f.Env.UI.SetModal(false, f)
	}
	was := f.Visible
	f.Form.Hide()
	if was && f.client.Music != nil {
		if f.music != "" {
			f.client.Music.Play(f.music)
		} else {
			f.client.playMapMusic()
		}
	}
}
func (f *starBookForm) KeyDown(key uint16, _ byte) {
	if key == 0x1b {
		f.Hide()
	}
}

// Full-screen StarSystem is outside TSe_Form's 575-pixel module clamp.
func (f *starBookForm) Update(in *seui.Input) { f.Left, f.Top = 0, 0; f.Panel.Update(in) }
func (f *starBookForm) Paint() {
	c := f.client
	c.Screen.Fill(image.Rect(0, 0, 800, 600), 0)
	w, h := c.Pics.Size(f.sky)
	if w > 0 && h > 0 {
		offset := int(c.Now().Sub(f.opened)/(20*time.Millisecond)) % w
		for y := -20; y < 600; y += h {
			for x := offset - w; x < 800; x += w {
				c.Pics.Draw(c.Screen, f.sky, x, y, false)
			}
		}
	}
	// Only received constellation ownership is rendered. Position fallback uses
	// stable catalog slots when native executable coordinates are unavailable.
	for id, owned := range c.constellations {
		if !owned {
			continue
		}
		pic := c.Pics.Find("Star_" + strconv.Itoa(id))
		w, h := c.Pics.Size(pic)
		if w == 0 || h < 2 {
			continue
		}
		x, y := 35+(id-1)%8*96, 35+(id-1)/8*48
		c.Pics.DrawRect(c.Screen, pic, x-w/2, y-h/4, image.Rect(0, h/2, w, h), true)
	}
}
