package login

import (
	"encoding/binary"
	"image"
	"math"
	"math/rand/v2"
	"strconv"
	"time"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/protocol"
)

// Character creation is TRE_CreateCharacter (VMT 0x212fcc, constructor
// FUN_00215d38, PTR_DAT_004ca398), a TSe_FixedForm. Packet 1/3 opens it
// (case 0x2ded1a); its steps (FUN_00219bfc) are
//
//	1 portrait carousel and name (9/2 checks the name, 9/3 answers)
//	2 attribute points
//	3 element
//	4 colours
//	5 the password dialog (TRe_InputPwAndDarkPw), when the account has no
//	  secret code yet
//
// and step 0 cancels with 63/3. OK on step 4, or the dialog's OK, sends
// 9/1.
const (
	screenWidth  = 800
	screenHeight = 600
	noticeTime   = 2000 * time.Millisecond // PTR_DAT_004c9e40 slot +0x9c, 0x7d0

	rolePicCount   = 14
	createMaxStep  = 5
	startingPoints = 5
	statCount      = 5
	elementCount   = 4
	partCount      = 4
	maxColorDigit  = 9
	nameMaxLen     = 10
	defaultAction  = 0xb // the body faces the viewer (+0x121)
	firstTurn      = 8
	lastTurn       = 0xf
	portraitShown  = 2 // the portrait object's action

	// Wizard buttons (+0xf0 + i·4) and their click tags (FUN_00217a24).
	wizOK         = 1
	wizNext       = 2
	wizPrev       = 3
	wizDefault    = 4
	wizTurnLeft   = 5
	wizTurnRight  = 6
	wizardButtons = 6

	backgroundX      = 0x1e
	backgroundXStep4 = 0x7e
	backgroundY      = 0x4d
	contentShift     = 0x3a // y offset with content level (PTR_DAT_004c9b64)
	titleX, titleY   = 0x11a, 0x50

	textInk       = 0x528a
	textWrap      = 100
	nameCharWidth = 8

	scrollMSPerPixel = 0.02 // FUN_0021ada4: pixels per elapsed millisecond
	scrollMargin     = 100  // +0x2fc is the widest right edge less 100
	pickedRow        = 2    // the selected picture is the pressed row
	blinkLength      = 50 * time.Millisecond
	blinkWaitBase    = 6000
	blinkWaitRandom  = 2000
	arrowFrameEvery  = 300 * time.Millisecond

	digitWidth, digitHeight = 0xb, 0xc
	digitsX                 = 0x1ba
	pointsY                 = 0x101
	statY, statStep         = 0x12b, 0x18
	bonusX, bonusY          = 0x1ea, 0x12a

	elementMarkX, elementMarkStep, elementMarkY = 0x126, 0x3c, 0xf4
	partMarkX, partMarkStep, partMarkY          = 0xa7, 0x34, 0x106
	barBackX, barX, barRowY, barRowStep         = 0xde, 0xe0, 0x159, 0x1f
	barDigitWidth, barHeight                    = 0xc, 0xf

	hintCharsPerLine, hintLineHeight = 0x14, 0x14
	hintPen, hintFill                = 0xff0000, 0xf98b3d
)

// Notices of the wizard and of 9/3.
var (
	noticeEnterName    = []byte("Enter name")
	noticeNameTooLong  = []byte("Name is too long")
	noticeIllegalWords = []byte("Don't use Illegal words")
	noticeAllocate     = []byte("Allocate all points")
	noticeNameInUse    = []byte("Name already in use")       // 0x2f03a0
	noticeIllegalName  = []byte("Illegal name, try another") // 0x2f03c4
)

// Colour digit groups each part edits (+0xd8, +0xdb, +0xde, +0xfc).
var partDigits = [partCount + 1]int{0, 3, 6, 9, 0x27}

// rolePick is one carousel entry (+0x312 + i·9).
type rolePick struct {
	Index byte
	At    image.Point
}

type CreateCharacter struct {
	seui.FixedForm
	G       *Globals
	Net     *Net
	Assets  Assets
	Notify  Notifier
	Content byte
	Now     func() time.Time
	Rand    func(n int) int

	// Select is PTR_DAT_004ca634, shown again on cancel; Password is
	// DAT_006c6f40. StopVoices is FUN_00405138(0).
	Select     *SelectCharacter
	Password   *InputPassword
	StopVoices func()

	Background   *surface.Surface                     // +0xe8
	Step         byte                                 // +0xec
	Buttons      [wizardButtons + 1]*seui.FixedButton // +0xf0 + i·4
	Role         *RoleImage                           // +0x10c
	HasCode      bool                                 // +0x110
	ShowName     bool                                 // +0x111
	NameAt       image.Point                          // +0x114
	ShowArt      bool                                 // +0x11c
	ArtAt        image.Point                          // +0x120
	ElementIcon  *seui.ElementIcon                    // +0x128
	InfoPic      int                                  // +0x12c
	InfoAt       image.Point                          // +0x130
	Logo         int                                  // +0x138
	StatRects    [statCount + 1]image.Rectangle       // +0x13c + i·0x10
	Title        int                                  // +0x19c
	NameField    *seui.Editor                         // +0x1a0
	NameAccepted bool                                 // +0x1a4
	RolePics     [rolePicCount + 1]*seui.FixedButton  // +0x1a8 + i·4
	Selected     byte                                 // +0x1e4
	Waiting      bool                                 // +0x1e5 a 9/2 is unanswered
	Points       byte                                 // +0x1e6
	Stats        [statCount + 1]byte                  // +0x1e7: STR, CON, INT, WIS, AGI
	StatArrows   [2*statCount + 1]*seui.FixedButton   // +0x1f0
	DigitsWhite  int                                  // +0x21c Num_White3_2
	DigitsYellow int                                  // +0x220 Num_Yellow3_1
	Elements     [elementCount + 1]*seui.Panel        // +0x224
	ElementMark  int                                  // +0x238 Panel28
	ColorArrows  [7]*seui.FixedButton                 // +0x23c
	Bars         [3]int                               // +0x258 Color_R, Color_G, Color_B
	BarBack      int                                  // +0x264 Panel24
	PartMark     int                                  // +0x268 Panel25
	Part         byte                                 // +0x26c
	Parts        [partCount + 1]*seui.Panel           // +0x284
	Color1       uint32                               // +0x298
	Color2       uint32                               // +0x29c
	PrevChar     *seui.FixedButton                    // +0x2a0
	NextChar     *seui.FixedButton                    // +0x2a4
	Scroll       int                                  // +0x2a8
	PicLeft      [rolePicCount + 1]int                // +0x2c0 initial positions
	Wrap         int                                  // +0x2fc
	Remaining    int                                  // +0x300
	scrollAt     time.Time                            // +0x308
	Current      byte                                 // +0x310
	Previous     byte                                 // +0x311
	Picks        [rolePicCount + 1]rolePick           // +0x312, by left edge
	VisibleFrom  int                                  // +0x39c
	VisibleTo    int                                  // +0x3a0
	ScrollDir    byte                                 // +0x3a4: 1 previous, 2 next
	blink        byte                                 // +0x3a5
	blinkAt      time.Time                            // +0x3a8
	blinkWait    time.Duration                        // +0x3b0
}

// NewCreateCharacter is FUN_00215d38.
func NewCreateCharacter(env *seui.Env, g *Globals, n *Net, a Assets, content byte) *CreateCharacter {
	c := &CreateCharacter{G: g, Net: n, Assets: a, Content: content, Now: time.Now, Rand: rand.IntN}
	c.InitFixedForm(c, env)
	c.Name = "TRE_CreateCharacter"

	seui.NewFixedButton(env, c) // +0xf0, created and never placed
	wizard := [wizardButtons + 1]struct {
		name      string
		left, top int
		h, w      int
	}{
		{},
		{"btn_ok_1", 0, 0, 0x14, 0x38},
		{"Btn_Next", 0, 0, 0x14, 0x38},
		{"Btn_Prev_1", 0, 0, 0x14, 0x38},
		{"Btn_Default", 0xf0, 0x133, 0x14, 0x38},
		{"Btn_ArrowL_1", 0, 0, 0x11, 0x12},
		{"Btn_ArrowR_1", 0, 0, 0x11, 0x12},
	}
	for i := 1; i <= wizardButtons; i++ {
		b := seui.NewFixedButton(env, c)
		b.Tag = i
		b.SetVisible(false)
		b.OnClickTag = c.clicked
		w := wizard[i]
		b.Init(w.name, w.left, w.h, w.w, 0, 0, true, w.h, w.w, w.top)
		c.Buttons[i] = b
	}

	c.Role = NewRoleImage(env, nil) // not a child: only the form paints it
	c.Role.Init("", 0, 0, 0, 0, 0, false, 0x5a, 0x28, 0)
	c.Role.Body.Edit = [3]int{NeutralDigit, NeutralDigit, NeutralDigit}
	c.Role.Body.Action = defaultAction

	c.ElementIcon = seui.NewElementIcon(env, c)
	c.ElementIcon.Init("", 0, 0, 0, 0, 0, false, 0x19, 0x19, 0)
	c.Logo = env.Pics.Find("Icon_LoginLogo_1")
	for i := 1; i <= statCount; i++ {
		top := (i-1)*0x18 + 0x128
		c.StatRects[i] = image.Rect(0x147, top, 0x147+0x48, top+0x11)
	}
	c.Title = -1
	if content != 0 {
		c.Title = env.Pics.Find("Icon_CreateRole_1")
	}

	c.NameField = seui.NewEditor(env, c)
	c.NameField.Init("Panel27", 0x264, 0, 0, 0, 0, false, 0x16, 0x5d, 0xcf)
	c.NameField.SetMargins(2, 2, 2, 2)
	c.NameField.SetColor(0xffff)
	c.NameField.Color2 = 0
	c.NameField.SetMaxLen(nameMaxLen)
	c.NameField.Name = "CreateRoleName"

	c.PrevChar = seui.NewFixedButton(env, c)
	c.PrevChar.Init("Btn_ArrowL_1", 0x26, 0x11, 0x12, 0, 0, true, 0x11, 0x12, 0x112)
	c.PrevChar.OnClick = c.prevChar
	c.PrevChar.SetAnimation("Btn_ArrowL_10", 0, 2, 1, arrowFrameEvery, 0x11, 0x12, 0)
	c.PrevChar.SetHint([]byte("Prev Char"))
	c.NextChar = seui.NewFixedButton(env, c)
	c.NextChar.Init("Btn_ArrowR_1", 0x1f7, 0x11, 0x12, 0, 0, true, 0x11, 0x12, 0x112)
	c.NextChar.OnClick = c.nextChar
	c.NextChar.SetAnimation("Btn_ArrowR_10", 0, 2, 1, arrowFrameEvery, 0x11, 0x12, 0)
	c.NextChar.SetHint([]byte("Next Char"))

	c.Current = 1
	c.buildCarousel()

	seui.NewFixedButton(env, c) // +0x1f0, unused
	for i := 1; i <= 2*statCount; i++ {
		b := seui.NewFixedButton(env, c)
		top := (i-1)/2*statStep + 0x129
		if i%2 == 0 {
			b.Init("Btn_ArrowR_4", 0x1d3, 0x11, 0x11, 0, 0, true, 0x11, 0x11, top)
		} else {
			b.Init("Btn_ArrowL_4", 0x19b, 0x11, 0x11, 0, 0, true, 0x11, 0x11, top)
		}
		b.OnClickTag = c.statArrow
		b.Tag = i
		b.SetVisible(false)
		c.StatArrows[i] = b
	}
	c.Points = startingPoints
	c.DigitsWhite = env.Pics.Find("Num_White3_2")
	c.DigitsYellow = env.Pics.Find("Num_Yellow3_1")

	seui.NewPanel(env, c) // +0x224, unused
	for i := 1; i <= elementCount; i++ {
		p := seui.NewPanel(env, c)
		p.Init("Icon_Element_"+strconv.Itoa(i)+"_2", (i-1)*0x3c+0x12d, 0, 0, 0, 0, false, 0x11, 0x11, 0xf9)
		p.Hover = false
		p.Tag = i
		p.OnClickTag = c.chooseElement
		p.SetVisible(false)
		c.Elements[i] = p
	}
	c.ElementMark = env.Pics.Find("Panel28")

	seui.NewFixedButton(env, c) // +0x23c, unused
	colorArrows := [7]struct {
		name      string
		left, top int
	}{{}, {"Btn_ArrowR_2", 0x151, 0x158}, {"Btn_ArrowL_2", 0xc9, 0x158},
		{"Btn_ArrowR_2", 0x151, 0x177}, {"Btn_ArrowL_2", 0xc9, 0x177},
		{"Btn_ArrowR_2", 0x151, 0x197}, {"Btn_ArrowL_2", 0xc9, 0x197}}
	for i := 1; i < len(colorArrows); i++ {
		b := seui.NewFixedButton(env, c)
		b.OnClickTag = c.colorArrow
		b.Tag = i
		b.SetVisible(false)
		a := colorArrows[i]
		b.Init(a.name, a.left, 0x11, 0x12, 0, 0, true, 0x11, 0x12, a.top)
		c.ColorArrows[i] = b
	}

	seui.NewPanel(env, c) // +0x284, unused
	for i := 1; i <= partCount; i++ {
		p := seui.NewPanel(env, c)
		p.Init("Icon_Part_"+strconv.Itoa(i), (i-1)*0x34+0xac, 0, 0, 0, 0, false, 0x1c, 0x2c, 0x10b)
		p.Tag = i
		p.OnClickTag = c.choosePart
		p.Hover = false
		p.SetVisible(false)
		c.Parts[i] = p
	}
	for i, name := range [3]string{"Color_R", "Color_G", "Color_B"} {
		c.Bars[i] = env.Pics.Find(name)
	}
	c.BarBack = env.Pics.Find("Panel24")
	c.PartMark = env.Pics.Find("Panel25")
	c.Part = 1

	c.reset()
	c.VisibleFrom = c.PrevChar.Right() - 10
	c.VisibleTo = c.NextChar.Left + 10
	return c
}

// buildCarousel is the middle of FUN_00215d38: the pictures are created
// from the back (largest y, then largest x) to the front, then the entries
// are ordered by x (then y) for the arrows.
func (c *CreateCharacter) buildCarousel() {
	for i := 1; i <= rolePicCount; i++ {
		c.Picks[i] = rolePick{Index: byte(i), At: rolePicAt[i]}
	}
	c.sortPicks(func(a, b rolePick) bool { return a.At.Y < b.At.Y || a.At.Y == b.At.Y && a.At.X < b.At.X })
	c.Wrap = 0
	seui.NewFixedButton(c.Env, c) // +0x1a8, unused
	for k := 1; k <= rolePicCount; k++ {
		p := c.Picks[k]
		b := seui.NewFixedButton(c.Env, c)
		b.Tag = int(p.Index)
		b.OnClickTag = c.selectRoleTag
		b.SetVisible(false)
		b.SetToggle(false)
		name := "Btn_RolePic_" + strconv.Itoa(int(p.Index))
		w, h := c.Env.Pics.Size(c.Env.Pics.Find(name))
		h /= 3
		b.Init(name, p.At.X-w/2, h, w, 0, 0, true, h, w, p.At.Y+0xf-h)
		c.RolePics[p.Index] = b
		c.PicLeft[p.Index] = b.Left
		c.Wrap = max(c.Wrap, b.Right())
	}
	c.Wrap -= scrollMargin
	c.sortPicks(func(a, b rolePick) bool { return b.At.X < a.At.X || a.At.X == b.At.X && b.At.Y < a.At.Y })
}

// sortPicks is the constructor's bubble sort: swap(a, b) says whether
// neighbours a, b are out of order.
func (c *CreateCharacter) sortPicks(swap func(a, b rolePick) bool) {
	for i := 1; i < rolePicCount; i++ {
		for k := 1; k <= rolePicCount-i; k++ {
			if swap(c.Picks[k], c.Picks[k+1]) {
				c.Picks[k], c.Picks[k+1] = c.Picks[k+1], c.Picks[k]
			}
		}
	}
}

// reset is FUN_00219084.
func (c *CreateCharacter) reset() {
	c.Step = 0
	c.NameAccepted = false
	c.Role.Body.Element = 1
	c.NameField.SetText(nil)
	c.Selected = 1
	c.selectRole(1)
	c.Scroll = 0
	c.ScrollDir = 0
	c.Stats = [statCount + 1]byte{}
	c.Points = startingPoints
	c.Part = 1
	c.resetColors()
}

// resetColors is FUN_00219118.
func (c *CreateCharacter) resetColors() {
	b := &c.Role.Body
	b.Edit = [3]int{NeutralDigit, NeutralDigit, NeutralDigit}
	b.NeutralColors()
	c.Role.Portrait.NeutralColors()
}

// equipStarter is FUN_00215c24: the preview wears the role's starter items.
func (c *CreateCharacter) equipStarter() {
	b := &c.Role.Body
	b.Items = [7]uint16{}
	r := RoleIndex(b.Type, b.Head)
	if r == 0 || c.Role.Painter == nil {
		return
	}
	for _, id := range starterItems[r] {
		if id != 0 {
			if s := c.Role.Painter.EquipSlot(id); s < byte(len(b.Items)) {
				b.Items[s] = id
			}
		}
	}
}

func (c *CreateCharacter) selectRoleTag(tag int) { c.selectRole(byte(tag)) }

// selectRole is FUN_0021838c: a portrait chooses the body type and head,
// the large picture and the description.
func (c *CreateCharacter) selectRole(i byte) {
	if i < 1 || i > rolePicCount {
		return
	}
	c.Selected = i
	r := creationRoles[i]
	b, p := &c.Role.Body, &c.Role.Portrait
	b.Look, b.Head, b.Type = 0, r.Head, r.Type
	p.Type = r.Type
	c.ArtAt = r.Art
	c.InfoPic = c.Env.Pics.Find(r.Info)
	c.InfoAt = r.InfoAt
	c.equipStarter()
	p.Action, p.Head = portraitShown, b.Head
	p.Frame = 0
	b.Action = defaultAction
}

// Show is slot +0x20 (FUN_00215bcc).
func (c *CreateCharacter) Show() {
	c.animate()
	c.FixedForm.Show()
	c.blink, c.blinkAt = 0, c.Now()
	c.blinkWait = time.Duration(c.Rand(blinkWaitRandom)+blinkWaitBase) * time.Millisecond
	c.Waiting = false
	c.PrevChar.StartAnimation()
	c.NextChar.StartAnimation()
}

// Hide is slot +0x24 (FUN_00217944): the wizard starts over.
func (c *CreateCharacter) Hide() {
	c.FixedForm.Hide()
	c.PrevChar.StopAnimation()
	c.NextChar.StopAnimation()
	c.reset()
}

// Open is the 1/3 case (0x2ded1a): the selection form closes and the
// wizard opens on step 1. hasCode is the packet's flag (+0x110): the
// account already has a secret code, so step 5 is skipped.
func (c *CreateCharacter) Open(hasCode bool) {
	c.G.CharacterActive = true
	if c.Select != nil {
		c.Select.Hide()
	}
	c.HasCode = hasCode
	c.step(true)
	c.Show()
}

func (c *CreateCharacter) notify(text []byte) {
	if c.Notify != nil {
		c.Notify(text, noticeTime)
	}
}

func (c *CreateCharacter) show(b seui.Control, left, top int) {
	g := b.Base()
	g.Left, g.Top = left, top
	g.Visible = true
}

// step is FUN_00219bfc: forward or back one step.
func (c *CreateCharacter) step(forward bool) {
	for _, b := range c.Buttons[1:] {
		b.SetVisible(false)
	}
	c.NameField.SetVisible(false)
	for _, b := range c.RolePics[1:] {
		b.SetVisible(false)
	}
	for _, b := range c.ColorArrows[1:] {
		b.SetVisible(false)
	}
	for _, p := range c.Parts[1:] {
		p.SetVisible(false)
	}
	for _, p := range c.Elements[1:] {
		p.SetVisible(false)
	}
	for _, b := range c.StatArrows[1:] {
		b.SetVisible(false)
	}
	c.PrevChar.SetVisible(false)
	c.NextChar.SetVisible(false)
	c.ShowName, c.ShowArt = false, false
	c.Role.SetVisible(false)
	c.Role.ShowPortrait = true
	c.Role.Portrait.Action = portraitShown
	c.Role.Portrait.X, c.Role.Portrait.Y = 0x24c, 0x147
	c.ElementIcon.SetVisible(false)

	next := c.Step
	if forward {
		if next < createMaxStep {
			next++
		}
	} else if next != 0 {
		next--
	}
	c.equipStarter()
	c.Role.Body.Action = defaultAction

	var background string
	switch next {
	case 0:
		c.Background = nil
		c.Net.Send([]byte{protocol.CommandLogin, loginCancelCreation})
		c.Hide()
		if c.Select != nil {
			c.Select.Show()
		}
		c.G.CharacterActive = false
		return
	case 1:
		background = "Form_CreateRole_1.jpg"
		c.NameField.SetVisible(true)
		for _, b := range c.RolePics[1:] {
			b.SetVisible(true)
		}
		c.show(c.Buttons[wizPrev], 0x253, 0x1bf)
		c.show(c.Buttons[wizNext], 700, 0x1bf)
		c.show(c.Buttons[wizTurnLeft], 0x26e, 0x170)
		c.show(c.Buttons[wizTurnRight], 0x2c7, 0x170)
		c.show(c.Role, 0x294, 0x131)
		c.PrevChar.SetVisible(true)
		c.NextChar.SetVisible(true)
	case 2:
		background = "Form_CreateRole_2.jpg"
		c.show(c.Buttons[wizPrev], 0x253, 0x1bf)
		c.show(c.Buttons[wizNext], 700, 0x1bf)
		c.show(c.Role, 0x294, 0x131)
		c.ShowName, c.NameAt = true, image.Pt(0x283, 0x195)
		c.ShowArt = true
		for _, b := range c.StatArrows[1:] {
			b.SetVisible(true)
		}
	case 3:
		background = "Form_CreateRole_3.jpg"
		for _, p := range c.Elements[1:] {
			p.SetVisible(true)
		}
		c.show(c.Buttons[wizPrev], 0x253, 0x1bf)
		c.show(c.Buttons[wizNext], 700, 0x1bf)
		c.show(c.Role, 0x294, 0x131)
		c.ShowName, c.NameAt = true, image.Pt(0x283, 0x195)
		c.ShowArt, c.ArtAt = true, image.Pt(0x85, 0x1d1)
		c.chooseElement(1) // FUN_00218190(form, 1): step 3 starts on element 1
		// FUN_00217a24(form, 4) runs while the old step is current: coming
		// back from step 4 resets the colours.
		c.clicked(wizDefault)
		c.show(c.ElementIcon, 700, 0x121)
	case 4:
		background = "Form_CreateRole_4.jpg"
		c.show(c.Buttons[wizPrev], 0x1d4, 0x1bf)
		if !c.HasCode {
			c.show(c.Buttons[wizNext], 0x23a, 0x1bf)
		} else {
			c.show(c.Buttons[wizOK], 0x23a, 0x1bf)
		}
		c.show(c.Buttons[wizDefault], 0xf0, 0x132)
		c.show(c.Buttons[wizTurnLeft], 0x1ee, 0x171)
		c.show(c.Buttons[wizTurnRight], 0x247, 0x171)
		c.show(c.Role, 0x213, 0x131)
		c.Role.Portrait.X, c.Role.Portrait.Y = 0x1cc, 0x148
		c.ShowName, c.NameAt = true, image.Pt(0x226, 0x195)
		for _, b := range c.ColorArrows[1:] {
			b.SetVisible(true)
		}
		for _, p := range c.Parts[1:] {
			p.SetVisible(true)
		}
		c.Role.Body.Items = [7]uint16{}
		c.show(c.ElementIcon, 0x23e, 0x121)
	case 5:
		if c.HasCode || c.Password == nil {
			return
		}
		c.Password.SetMode(1)
		c.Password.OK.OnClick = c.passwordOK
		c.Password.Second.OnClick = c.passwordBack
		c.Password.Show()
	default:
		return
	}
	c.Background = nil
	if background != "" {
		c.Background, _ = loadJPEG(c.Assets.SkinPicture("Menu", "Skins", "White", background))
	}
	c.Step = next
	c.animate()
}

// clicked is FUN_00217a24, ignored while a name check is unanswered.
func (c *CreateCharacter) clicked(tag int) {
	if c.Waiting {
		return
	}
	switch tag {
	case wizOK:
		if c.Step == 4 {
			c.EncodeColors()
			c.Net.Send(c.CreatePacket())
			c.Hide()
			if c.StopVoices != nil {
				c.StopVoices()
			}
		}
	case wizNext:
		switch c.Step {
		case 1:
			name := c.NameField.Text
			switch {
			case len(name) < 1:
				c.notify(noticeEnterName)
			case len(name) > nameMaxLen:
				c.notify(noticeNameTooLong)
			case !NameAllowed(name, c.G.PlayerID):
				c.notify(noticeIllegalWords)
			default:
				c.Net.Send(c.NamePacket())
				c.Waiting = true
			}
		case 2:
			if c.Points == 0 {
				c.step(true)
			} else {
				c.notify(noticeAllocate)
			}
		case 3, 4:
			c.step(true)
		}
	case wizPrev:
		if c.Step == 3 {
			c.selectRole(c.Selected)
		}
		if c.Step >= 1 && c.Step <= 5 {
			c.step(false)
		}
	case wizDefault:
		if c.Step == 4 {
			c.resetColors()
		}
	case wizTurnLeft, wizTurnRight:
		if c.Step < 1 || c.Step > 4 {
			return
		}
		a := &c.Role.Body.Action
		if tag == wizTurnLeft {
			*a--
			if *a < firstTurn {
				*a = lastTurn
			}
		} else {
			*a++
			if *a > lastTurn {
				*a = firstTurn
			}
		}
	}
}

// NameResult is 9/3 (case at 0x2e1884): 0 accepts the name and moves on,
// 1 and 2 refuse it.
func (c *CreateCharacter) NameResult(status byte) {
	switch status {
	case 0:
		c.NameAccepted, c.Waiting = true, false
		c.step(true)
	case 1:
		c.NameAccepted, c.Waiting = false, false
		c.notify(noticeNameInUse)
	case 2:
		c.NameAccepted, c.Waiting = false, false
		c.notify(noticeIllegalName)
	}
}

// statArrow is 0x217970: odd tags take a point back, even tags spend one.
func (c *CreateCharacter) statArrow(tag int) {
	stat := (tag + 1) / 2
	if tag%2 != 0 {
		if c.Stats[stat] > 0 {
			c.Stats[stat]--
			c.Points++
		}
		return
	}
	if c.Points > 0 {
		c.Points--
		c.Stats[stat]++
	}
}

// chooseElement is FUN_00218190.
func (c *CreateCharacter) chooseElement(e int) {
	c.Role.Body.Element = byte(e)
	c.InfoPic = c.Env.Pics.Find("ElementInfo_" + strconv.Itoa(e))
	c.InfoAt = image.Pt(0x12d, 0x11b)
	c.ElementIcon.SetElement(byte(e), -1, false, 0, nil, -1, -1, -1)
}

// choosePart is 0x218278: the arrows edit the part's three digits.
func (c *CreateCharacter) choosePart(tag int) {
	if tag < 1 || tag > partCount {
		return
	}
	c.Part = byte(tag)
	b := &c.Role.Body
	d := partDigits[tag]
	b.Edit = [3]int{int(b.Colors[d]), int(b.Colors[d+1]), int(b.Colors[d+2])}
}

// colorArrow is 0x217eb4: tags 1/2 raise/lower red, 3/4 green, 5/6 blue,
// then the part's digits follow on both objects.
func (c *CreateCharacter) colorArrow(tag int) {
	b := &c.Role.Body
	if tag >= 1 && tag <= 6 {
		ch := &b.Edit[(tag-1)/2]
		if tag%2 == 1 && *ch < maxColorDigit {
			*ch++
		} else if tag%2 == 0 && *ch > 0 {
			*ch--
		}
	}
	if c.Part < 1 || c.Part > partCount {
		return
	}
	d := partDigits[c.Part]
	for k, v := range b.Edit {
		b.Colors[d+k] = byte(v)
		c.Role.Portrait.Colors[d+k] = byte(v)
	}
}

func (c *CreateCharacter) prevChar() {
	c.Previous = c.Current
	c.Current--
	if c.Current == 0 {
		c.Current = rolePicCount
	}
	c.ScrollDir = 1
	c.startScroll()
}

func (c *CreateCharacter) nextChar() {
	c.Previous = c.Current
	c.Current++
	if c.Current > rolePicCount {
		c.Current = 1
	}
	c.ScrollDir = 2
	c.startScroll()
}

// startScroll is FUN_0021af94: the distance that brings the current entry's
// picture to its stop, going right (1) or left (2) around the wrap.
func (c *CreateCharacter) startScroll() {
	k := c.Current
	if k < 1 || k > rolePicCount {
		return
	}
	if c.ScrollDir == 0 {
		c.Remaining, c.scrollAt = 0, time.Time{}
		return
	}
	left := c.RolePics[c.Picks[k].Index].Left
	stop := rolePicStops[k]
	switch c.ScrollDir {
	case 1:
		if stop < left {
			c.Remaining = abs(stop + c.Wrap - left)
		} else {
			c.Remaining = stop - left
		}
	case 2:
		if left < stop {
			c.Remaining = -abs(c.Wrap - stop + left)
		} else {
			c.Remaining = -abs(stop - left)
		}
	}
	c.scrollAt = c.Now()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// elapsed is FUN_00403910's measure: whole milliseconds since t.
func elapsedMS(now, t time.Time) int64 {
	return int64(math.RoundToEven(float64(now.Sub(t)) / float64(time.Millisecond)))
}

// animate is FUN_0021ada4: the portrait blinks, and on step 1 the
// carousel moves. The step grows with the time since the move started.
func (c *CreateCharacter) animate() {
	now := c.Now()
	switch c.blink {
	case 0:
		if elapsedMS(now, c.blinkAt) >= c.blinkWait.Milliseconds() {
			c.blinkAt, c.blink = now, 1
		}
	case 1:
		if elapsedMS(now, c.blinkAt) >= blinkLength.Milliseconds() {
			c.blinkAt = now
			c.blinkWait = time.Duration(c.Rand(blinkWaitRandom)+blinkWaitBase) * time.Millisecond
			c.blink = 0
		}
	}
	if c.Step != 1 {
		return
	}
	w := c.Wrap
	if c.Scroll >= w {
		c.Scroll = c.Scroll - w - w
	} else if c.Scroll <= -w {
		c.Scroll = w - abs(c.Scroll+w)
	}
	d := 0
	if c.Remaining != 0 {
		d = int(math.Ceil(float64(elapsedMS(now, c.scrollAt)) * scrollMSPerPixel))
		d = min(d, abs(c.Remaining))
		if c.Remaining < 0 {
			c.Remaining += d
			d = -d
		} else {
			c.Remaining -= d
		}
	}
	for _, b := range c.RolePics[1:] {
		b.Left += d
		if b.Left > w {
			b.Left -= w
		} else if b.Left < 0 {
			b.Left += w
		}
		b.Visible = true
		if b.Left < c.VisibleFrom || b.Right() > c.VisibleTo {
			b.Visible = false
		}
	}
}

// Update is slot +0x18 (FUN_002198a8): the fixed form, the animation, then
// the attribute hint under the pointer.
func (c *CreateCharacter) Update(in *seui.Input) {
	if !c.Visible {
		return
	}
	c.FixedForm.Update(in)
	c.animate()
	if i := c.hoveredStat(in); i != 0 {
		r := c.StatRects[i]
		seui.WrappedTooltip(c.Env, r.Max.X+2, r.Min.Y, []byte(statHints[i]), hintPen, hintFill, hintLineHeight, hintCharsPerLine)
	}
}

// hoveredStat is FUN_00215cdc: the attribute label under the pointer on
// step 2.
func (c *CreateCharacter) hoveredStat(in *seui.Input) int {
	if c.Step != 2 {
		return 0
	}
	for i := 1; i <= statCount; i++ {
		if image.Pt(in.X, in.Y).In(c.StatRects[i]) {
			return i
		}
	}
	return 0
}

// Paint is slot +0x10 (FUN_00219180).
func (c *CreateCharacter) Paint() {
	scr, pics := c.Env.Screen, c.Env.Pics
	if c.Background != nil {
		y := backgroundY
		if c.Content != 0 {
			y += contentShift
		}
		switch c.Step {
		case 1, 2, 3:
			scr.Draw(backgroundX, y, c.Background, false)
		case 4:
			scr.Draw(backgroundXStep4, y, c.Background, false)
		}
	}
	if c.Logo != -1 {
		w, _ := pics.Size(c.Logo)
		pics.Draw(scr, c.Logo, screenWidth-w, 0, true)
	}
	if c.Title != -1 && c.Content != 0 {
		pics.Draw(scr, c.Title, titleX, titleY, true)
	}
	if c.Role.Visible {
		c.Role.Blinking = c.blink == 1
		c.Role.Paint()
	}
	switch c.Step {
	case 2:
		c.digits(c.DigitsYellow, int(c.Points), digitsX, pointsY)
		for i := 1; i <= statCount; i++ {
			c.digits(c.DigitsWhite, int(c.Stats[i]), digitsX, (i-1)*statStep+statY)
		}
		pics.Draw(scr, c.InfoPic, c.InfoAt.X, c.InfoAt.Y, true)
		r := RoleIndex(c.Role.Body.Type, c.Role.Body.Head)
		for i := 1; i <= statCount; i++ {
			if v := statBonus[r][i-1]; v != 0 {
				text := []byte("+ " + strconv.Itoa(int(v)))
				c.Env.Text.Draw(bonusX, (i-1)*statStep+bonusY, 0, false, true, scr, text, 0, textWrap, 0, textInk, 0)
			}
		}
	case 3:
		x := (int(c.Role.Body.Element)-1)*elementMarkStep + elementMarkX
		pics.Draw(scr, c.ElementMark, x, elementMarkY, true)
		pics.Draw(scr, c.InfoPic, c.InfoAt.X, c.InfoAt.Y, true)
	case 4:
		x := (int(c.Part)-1)*partMarkStep + partMarkX
		pics.Draw(scr, c.PartMark, x, partMarkY, true)
		for i := 0; i < 3; i++ {
			y := i*barRowStep + barRowY
			backY, barY := y+1, y+3
			if i == 2 {
				backY, barY = y+2, y+4
			}
			pics.Draw(scr, c.BarBack, barBackX, backY, true)
			r := image.Rect(0, 0, c.Role.Body.Edit[i]*barDigitWidth, barHeight)
			pics.DrawRect(scr, c.Bars[i], barX, barY, r, true)
		}
	}
	if c.ShowArt {
		if b := c.RolePics[c.Selected]; b != nil {
			r := image.Rect(0, pickedRow*b.Height, b.Width, (pickedRow+1)*b.Height)
			pics.DrawRect(scr, b.Image, c.ArtAt.X-b.Width/2, c.ArtAt.Y-b.Height, r, true)
		}
	}
	if c.ShowName {
		name := c.NameField.Text
		x := c.NameAt.X - len(name)*nameCharWidth/2
		c.Env.Text.Draw(x, c.NameAt.Y, 0, false, true, scr, name, 0, textWrap, 0, textInk, 0)
	}
	c.GrBasic.Paint()
}

// digits is FUN_00218e08: each decimal digit is a row of the picture, all
// drawn at the same place.
func (c *CreateCharacter) digits(pic, v, x, y int) {
	if pic == -1 {
		return
	}
	for _, d := range strconv.Itoa(v) {
		n := int(d - '0')
		r := image.Rect(0, n*digitHeight, digitWidth, (n+1)*digitHeight)
		c.Env.Pics.DrawRect(c.Env.Screen, pic, x, y, r, true)
	}
}

// EncodeColors forms the two colour values from the digit block (step 4's
// OK, 0x218ed4): digits 0..8, and 9..11, 39..44.
func (c *CreateCharacter) EncodeColors() {
	d := c.Role.Body.Colors
	num := func(ds ...byte) uint32 {
		var v uint32
		for _, x := range ds {
			v = v*10 + uint32(x)
		}
		return v
	}
	c.Color1 = num(d[0], d[1], d[2], d[3], d[4], d[5], d[6], d[7], d[8])
	c.Color2 = num(d[9], d[10], d[11], d[0x27], d[0x28], d[0x29], d[0x2a], d[0x2b], d[0x2c])
}

// passwordOK is 0x218ed4: the dialog's OK on step 5.
func (c *CreateCharacter) passwordOK() {
	if c.Password.Check(true) != PwOK {
		return
	}
	c.EncodeColors()
	c.Net.Send(c.CreatePacket())
	c.Password.Hide()
	c.Hide()
	if c.StopVoices != nil {
		c.StopVoices()
	}
}

// passwordBack is 0x219070: Previous returns to step 4.
func (c *CreateCharacter) passwordBack() {
	c.step(false)
	c.Password.Hide()
}

// NamePacket is 9/2 (0x2c3c86): the name, unprefixed.
func (c *CreateCharacter) NamePacket() []byte {
	p := []byte{protocol.CommandCharacterCreation, protocol.CharacterCreationCreate}
	return append(p, c.NameField.Text...)
}

// CreatePacket is 9/1 (0x2c396c): body type, a zero byte, head, look, the
// two colour values, element and points in the order STR, AGI, WIS, INT,
// CON. Without a secret code the password and the code follow, each with
// a length byte.
func (c *CreateCharacter) CreatePacket() []byte {
	b := &c.Role.Body
	p := []byte{protocol.CommandCharacterCreation, protocol.CharacterCreationWireCode1,
		b.Type, 0, b.Head, byte(b.Look)}
	p = binary.LittleEndian.AppendUint32(p, c.Color1)
	p = binary.LittleEndian.AppendUint32(p, c.Color2)
	p = append(p, b.Element, c.Stats[1], c.Stats[5], c.Stats[4], c.Stats[3], c.Stats[2])
	if !c.HasCode && c.Password != nil {
		pw, code := c.Password.Password(), c.Password.Code()
		p = append(p, byte(len(pw)))
		p = append(p, pw...)
		p = append(p, byte(len(code)))
		p = append(p, code...)
	}
	return p
}
