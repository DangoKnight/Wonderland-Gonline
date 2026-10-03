package login

import (
	"encoding/binary"
	"image"
	"math/rand/v2"
	"strconv"
	"time"

	"wonderland-go/client/wlo/seui"
	"wonderland-go/client/wlo/surface"
	"wonderland-go/internal/protocol"
)

// Character selection layout (TSe_SelectCharacter, constructor 0x400804;
// paint FUN_004017e8).
const (
	slotCount           = 2 // slots shown; the record table holds three
	slotRecordSlots     = 3
	slotPanelY          = 0x87
	emptyIconY          = 0x128
	selectRoleIconX     = 0x11a
	selectRoleIconY     = 0x50
	nameOffsetX         = 0x37
	nameOffsetY         = 0x6c
	nameWidth           = 100
	levelOffsetY        = 0x87
	careerOffsetX       = 0xac
	careerOffsetY       = 0x2d
	jobOffsetX          = 0xe3
	jobOffsetY          = 0x30
	statOffsetX         = 0xd9
	hpOffsetY           = 0x48
	spOffsetY           = 0x57
	expOffsetY          = 0x68
	goldOffsetY         = 0x79
	defaultDirection    = 0xb
	firstDirection      = 8
	lastDirection       = 0xf
	idleDelayMin        = 6000 * time.Millisecond
	idleDelayRandomMs   = 2000
	idleBlink           = 0x32 * time.Millisecond
	equipmentSlots      = 6
	slotRecordTail      = 0x36 // bytes of a record after the name
	loginReturn         = 0    // 63/0 from Previous; the server names 0 LoginSelect
	loginCancelCreation = 3    // 63/3 from character creation's step 0 (0x2d8dcd)
	// gmTag is the GM account check (FUN_0010e9cc on the account name);
	// it is not ported, so no account is treated as a GM.
)

// slotPanelX are the left and right panels' x positions.
var slotPanelX = [slotCount]int{0x28, 0x1a4}

// emptyIconX are Icon_Empty's x positions.
var emptyIconX = [slotCount]int{0xa6, 0x222}

// CharacterSlot is one 99-byte slot record (+0x170 + slot*99), filled by
// the roster packet (FUN_00402468). The HP and SP pairs are named by the
// role fields they feed: the drawer shows the second value first.
type CharacterSlot struct {
	Name      []byte // +0x170, at most 40 bytes
	Level     byte   // +0x199
	Reborn    byte   // +0x19a
	Job       byte   // +0x19b
	Element   byte   // +0x19c
	Value     [6]uint32
	Body      byte // +0x1b5
	BodyHigh  byte // +0x1b6
	Head      byte // +0x1b7
	HeadHigh  byte // +0x1b8
	Color1    uint32
	Color2    uint32
	Equipment [equipmentSlots + 1]uint16 // +0x1c1, by equipment slot 1..6
	Direction int32                      // +0x1cf
}

// Value indexes: +0x19d, +0x1a1, +0x1a5, +0x1a9, +0x1ad, +0x1b1.
const (
	slotHPMax = 0 // shown after the slash
	slotHP    = 1 // shown first
	slotSPMax = 2
	slotSP    = 3
	slotExp   = 4
	slotGold  = 5
)

func (c *CharacterSlot) occupied() bool { return c.Level != 0 }

// RoleView is the character sprite pair a slot shows: the body
// (FUN_00412c50) and the portrait (FUN_002586c8) of THuman objects. The
// sprite engine is not ported; a nil RoleView draws nothing.
type RoleView interface {
	SetCharacter(c *CharacterSlot)
	DrawBody(dst *surface.Surface, x, y int, direction int32)
	DrawPortrait(dst *surface.Surface, x, y int, blinking bool)
}

// Body and portrait positions of the four THuman objects (DAT_007a15b4..c0).
var (
	bodyAt     = [slotCount]image.Point{{0xd2, 0x1a9}, {0x24e, 0x1a9}}
	portraitAt = [slotCount]image.Point{{0x91, 0xcd}, {0x20d, 0xcd}}
)

// SelectCharacter is TSe_SelectCharacter.
type SelectCharacter struct {
	seui.Form
	G      *Globals
	Net    *Net
	Assets Assets
	Status *Status
	Notify Notifier
	Roles  [slotCount]RoleView

	EmptyBack  *surface.Surface // +0x15c Form_SelectRole_Empty
	EmptyBack2 *surface.Surface // +0x160
	InfoBack   *surface.Surface // +0x164 Form_SelectRole_Info
	InfoBack2  *surface.Surface // +0x168
	EmptyIcon  int              // +0x16c Icon_Empty
	SelectIcon int              // +0x334 Icon_SelectRole_1
	CareerIcon int

	Slots    [slotRecordSlots + 1]CharacterSlot // index 1..3
	Elements [slotRecordSlots + 1]*seui.ElementIcon
	Enter    [slotCount]*seui.Button          // +0x148, +0x150 Btn_Login
	Manage   [slotCount]*seui.Button          // +0x14c, +0x154 create or delete
	Prev     *seui.Button                     // +0x158
	Arrows   [2 * slotCount]*seui.FixedButton // +0x300..+0x30c

	idle     [slotCount]byte      // +0x311, +0x312
	idleAt   [slotCount]time.Time // +0x318, +0x320
	idleWait [slotCount]time.Duration
	Now      func() time.Time

	// Login is the account form (DAT_007a15a8), shown again by Previous.
	Login *IDPassword
	// Password is the password dialog (PTR_DAT_004ca5fc); Delete is the
	// slot it deletes (+0x310).
	Password *InputPassword
	Delete   byte
	// OnEnter runs after a character is chosen; the world is not ported.
	OnEnter func(slot byte)
}

// ContentLevel is DAT_00839d5c (FUN_004a3b50): 1..5 by the size of
// jma\001.jma; nonzero selects the "_R" pictures. The client takes the
// size from the 001 sprite pack's source_bytes (0 when unknown).
func ContentLevel(jma001Bytes int64) byte {
	switch n := jma001Bytes; {
	case n >= 207000000:
		return 5
	case n >= 190000000:
		return 4
	case n >= 165000000:
		return 3
	case n >= 140000000:
		return 2
	case n >= 110000000:
		return 1
	}
	return 0
}

func NewSelectCharacter(env *seui.Env, g *Globals, n *Net, a Assets, status *Status, content byte) *SelectCharacter {
	s := &SelectCharacter{G: g, Net: n, Assets: a, Status: status, Now: time.Now}
	s.InitForm(s, env)
	s.Name = "SelectCharacter"
	side := "_L"
	if content != 0 {
		side = "_R"
	}
	load := func(name string) *surface.Surface {
		m, _ := loadJPEG(a.SkinPicture("Menu", "Skins", "White", name))
		return m
	}
	s.EmptyBack = load("Form_SelectRole_Empty" + side + ".jpg")
	s.EmptyBack2 = load("Form_SelectRole_Empty_R.jpg")
	s.InfoBack = load("Form_SelectRole_Info" + side + ".jpg")
	s.InfoBack2 = load("Form_SelectRole_Info_R.jpg")
	s.EmptyIcon = env.Pics.Find("Icon_Empty")
	s.SelectIcon = -1
	if content != 0 {
		s.SelectIcon = env.Pics.Find("Icon_SelectRole_1")
	}
	s.CareerIcon = env.Pics.Find("Icon_Career_2")
	s.Shown = false

	seui.NewButton(env, s) // +0x144, created and never placed
	button := func(name string, left, top, tag int, visible bool) *seui.Button {
		b := seui.NewButton(env, s)
		b.Init(name, left, 0x14, 0x38, 0, 0, true, 0x14, 0x38, top)
		b.Tag = tag
		b.OnClickTag = s.clicked
		b.SetVisible(visible)
		return b
	}
	s.Enter[0] = button("Btn_Login", 0x137, 0x1b3, 1, false)
	s.Manage[0] = button("", 0x137, 0x1ca, 2, true)
	s.Enter[1] = button("Btn_Login", 0x2b3, 0x1b3, 3, false)
	s.Manage[1] = button("", 0x2b3, 0x1ca, 4, true)
	s.Prev = seui.NewButton(env, s)
	s.Prev.Init("Btn_Prev_1", 0x172, 0x14, 0x38, 0, 0, true, 0x14, 0x38, 500)
	s.Prev.OnClick = s.previous

	seui.NewFixedButton(env, s) // +0x2fc, unused
	arrows := [...]struct {
		name string
		left int
	}{{"Btn_ArrowL_1", 0x98}, {"Btn_ArrowR_1", 0xf9}, {"Btn_ArrowL_1", 0x214}, {"Btn_ArrowR_1", 0x275}}
	for i, a := range arrows {
		b := seui.NewFixedButton(env, s)
		b.Tag = i + 1
		b.OnClickTag = s.turn
		b.SetVisible(false)
		b.Init(a.name, a.left, 0x11, 0x12, 0, 0, true, 0x11, 0x12, 0x18b)
		s.Arrows[i] = b
	}

	seui.NewElementIcon(env, s) // +0x130, unused
	for i := 1; i <= slotRecordSlots; i++ {
		e := seui.NewElementIcon(env, s)
		e.Init("", (i-1)*0x17c+0x107, 0, 0, 0, 0, false, 0x19, 0x19, 0x9a)
		e.SetVisible(false)
		s.Elements[i] = e
	}
	for i := 1; i <= slotRecordSlots; i++ {
		s.clearSlot(byte(i))
	}
	return s
}

// slotIndex maps a record slot to its panel.
func slotIndex(slot byte) int { return int(slot) - 1 }

// clearSlot is FUN_00402368.
func (s *SelectCharacter) clearSlot(slot byte) {
	if slot < 1 || slot > slotRecordSlots {
		return
	}
	s.Slots[slot] = CharacterSlot{}
	if slot > slotCount {
		return
	}
	i := slotIndex(slot)
	s.Enter[i].SetVisible(false)
	s.Manage[i].Image = s.Env.Pics.Find("Btn_CreateCharacter")
	s.Arrows[2*i].SetVisible(false)
	s.Arrows[2*i+1].SetVisible(false)
	s.Elements[slot].SetVisible(false)
}

// setupSlot is FUN_004013d0.
func (s *SelectCharacter) setupSlot(slot byte) {
	if slot < 1 || slot > slotRecordSlots {
		return
	}
	c := &s.Slots[slot]
	s.Elements[slot].SetElement(c.Element, -1, false, 0, nil, -1, -1, -1)
	s.Elements[slot].SetVisible(true)
	if slot > slotCount {
		return
	}
	i := slotIndex(slot)
	s.Enter[i].SetVisible(true)
	s.Manage[i].Image = s.Env.Pics.Find("Btn_DeleteCharacter_1")
	s.Arrows[2*i].SetVisible(true)
	s.Arrows[2*i+1].SetVisible(true)
	if r := s.Roles[i]; r != nil {
		r.SetCharacter(c)
	}
}

// Roster is FUN_00402468, packet 63/1: the slot records, then the form
// opens and the login timer stops. s is the packet after its command.
func (s *SelectCharacter) Roster(p []byte) {
	for i := 1; i <= slotRecordSlots; i++ {
		s.clearSlot(byte(i))
	}
	s.Elements[1].SetVisible(false)
	s.Elements[2].SetVisible(false)
	if len(p) > 0 {
		p = p[1:]
	}
	for len(p) >= 2 {
		slot, n := p[0], int(p[1])
		if len(p) < n+slotRecordTail {
			break
		}
		if slot >= 1 && slot <= slotRecordSlots {
			s.Slots[slot] = parseSlot(p, n)
			s.setupSlot(slot)
		}
		p = p[n+slotRecordTail:]
	}
	s.Show()
	if s.Login != nil {
		s.Login.LoginTime = time.Time{}
	}
}

// parseSlot reads one record whose name is n bytes long.
func parseSlot(p []byte, n int) CharacterSlot {
	c := CharacterSlot{Name: append([]byte(nil), p[2:2+min(n, 40)]...)}
	b := p[n:]
	c.Level, c.Element = b[2], b[3]
	for i := range c.Value {
		c.Value[i] = binary.LittleEndian.Uint32(b[4+4*i:])
	}
	c.Body, c.BodyHigh, c.Head, c.HeadHigh = b[28], b[29], b[30], b[31]
	c.Color1 = binary.LittleEndian.Uint32(b[32:])
	c.Color2 = binary.LittleEndian.Uint32(b[36:])
	c.Reborn, c.Job = b[40], b[41]
	// The original looks each item up (FUN_003cf408, FUN_003cef18) and
	// stores it by its equipment slot; without the item table the six
	// IDs keep their packet order.
	for i := 0; i < equipmentSlots; i++ {
		c.Equipment[i+1] = binary.LittleEndian.Uint16(b[42+2*i:])
	}
	c.Direction = defaultDirection
	return c
}

// Show is slot +0x20 (FUN_00402a90): the idle timers restart.
func (s *SelectCharacter) Show() {
	if s.G.Mode == 2 {
		return
	}
	s.Form.Show()
	now := s.Now()
	for i := range s.idle {
		s.idle[i], s.idleAt[i] = 0, now
		s.idleWait[i] = idleDelayMin + time.Duration(rand.IntN(idleDelayRandomMs))*time.Millisecond
	}
}

// clicked is FUN_00401140, the tag handler of the slot buttons.
func (s *SelectCharacter) clicked(tag int) {
	switch tag {
	case 1, 3:
		slot := byte(1)
		if tag == 3 {
			slot = 2
		}
		// FUN_00401fd0 plays the character's voice first when its file
		// exists and enters after it; the voice is not ported.
		s.enter(slot)
	case 2, 4:
		slot := byte(1)
		if tag == 4 {
			slot = 2
		}
		if !s.Slots[slot].occupied() {
			s.Net.Send([]byte{protocol.CommandLogin, protocol.LoginSelectAlternate, slot})
			return
		}
		s.openDelete(slot)
	default:
		return
	}
	s.Hide()
}

// openDelete is the occupied-slot branch of FUN_00401140: the password
// dialog in mode 2 (FUN_0021a3f4) with OK at 0x402a0c and Cancel at
// FUN_00402a78; the selection then hides.
func (s *SelectCharacter) openDelete(slot byte) {
	if s.Password == nil {
		return
	}
	s.Delete = slot
	s.Password.SetMode(2)
	s.Password.OK.OnClick = s.deleteOK
	s.Password.Second.OnClick = s.deleteCancel
	s.Password.Show()
}

// deleteOK is 0x402a0c: the dialog is checked like creation's, then 35/2
// asks for the deletion and the selection returns.
func (s *SelectCharacter) deleteOK() {
	if s.Password.Check(true) != PwOK {
		return
	}
	s.Password.Hide()
	if s.Delete >= 1 && s.Delete <= slotCount {
		s.Net.Send(DeletePacket(s.Delete, s.Password.Password(), s.Password.Code()))
	}
	s.Show()
}

// deleteCancel is FUN_00402a78.
func (s *SelectCharacter) deleteCancel() {
	s.Password.Hide()
	s.Show()
}

// DeletePacket is 35/2 (send case 0x2d1a8e): the slot, then the password
// and the secret code, each with a length byte.
func DeletePacket(slot byte, password, code []byte) []byte {
	p := []byte{protocol.CommandCharacterSelection, protocol.CharacterSelectionDelete, slot, byte(len(password))}
	p = append(p, password...)
	p = append(p, byte(len(code)))
	return append(p, code...)
}

// Deletion results of 35/2 (receive case 0x2eab55).
const (
	deleteSucceeded = 1
	deleteFailed    = 4
	deleteErrorCode = 5
)

// DeleteResult is 35/2 from the server: 1 deleted (the slot is cleared),
// 2 and 3 a wrong code, 4 and 5 a failure.
func (s *SelectCharacter) DeleteResult(p []byte) {
	if len(p) < 1 {
		return
	}
	var text string
	switch p[0] {
	case deleteSucceeded:
		text = "Delete success"
		if len(p) > 1 {
			s.clearSlot(p[1])
		}
	case 2, 3:
		text = "Wrong Del Pwd"
	case deleteFailed:
		text = "Delete failed"
	case deleteErrorCode:
		text = "Delete failed, error code: "
		if len(p) > 1 {
			text += strconv.Itoa(int(p[1]))
		}
	default:
		return
	}
	if s.Notify != nil {
		s.Notify([]byte(text), deleteNotice)
	}
}

// deleteNotice is the notices' 0x7d0.
const deleteNotice = 2000 * time.Millisecond

// enter is FUN_00402274: 63/2 with the slot; the world is not ported.
func (s *SelectCharacter) enter(slot byte) {
	s.Net.Send([]byte{protocol.CommandLogin, protocol.LoginSelectAlternate, slot})
	if s.OnEnter != nil {
		s.OnEnter(slot)
	}
}

// previous is 0x401388: back to the account form.
func (s *SelectCharacter) previous() {
	s.Hide()
	s.Net.Send([]byte{protocol.CommandLogin, loginReturn})
	if s.Login != nil {
		s.Login.Show()
	}
}

// turn is 0x402920: the arrows turn a slot's character.
func (s *SelectCharacter) turn(tag int) {
	slot := byte(1 + (tag-1)/2)
	c := &s.Slots[slot]
	if !c.occupied() {
		return
	}
	left := tag%2 == 1
	switch {
	case left && c.Direction <= firstDirection:
		c.Direction = lastDirection
	case left:
		c.Direction--
	case c.Direction >= lastDirection:
		c.Direction = firstDirection
	default:
		c.Direction++
	}
}

// Paint is slot +0x10 (FUN_004017e8).
func (s *SelectCharacter) Paint() {
	scr := s.Env.Screen
	backs := [slotCount][2]*surface.Surface{{s.EmptyBack, s.InfoBack}, {s.EmptyBack2, s.InfoBack2}}
	for i := range slotCount {
		b := backs[i][0]
		if s.Slots[i+1].occupied() {
			b = backs[i][1]
		}
		if b != nil {
			scr.Draw(slotPanelX[i], slotPanelY, b, false)
		}
	}
	if s.SelectIcon != -1 {
		s.Env.Pics.Draw(scr, s.SelectIcon, selectRoleIconX, selectRoleIconY, true)
	}
	for i := range slotCount {
		s.paintSlot(i)
	}
	s.Panel.Paint()
}

func (s *SelectCharacter) paintSlot(i int) {
	c := &s.Slots[i+1]
	if !c.occupied() {
		s.Env.Pics.Draw(s.Env.Screen, s.EmptyIcon, emptyIconX[i], emptyIconY, true)
		return
	}
	x, y := slotPanelX[i], slotPanelY
	now := s.Now()
	switch s.idle[i] {
	case 0:
		if now.Sub(s.idleAt[i]) > s.idleWait[i] {
			s.idleAt[i], s.idle[i] = now, 1
		}
	case 1:
		if now.Sub(s.idleAt[i]) > idleBlink {
			s.idleAt[i], s.idle[i] = now, 0
			s.idleWait[i] = idleDelayMin + time.Duration(rand.IntN(idleDelayRandomMs))*time.Millisecond
		}
	}
	if r := s.Roles[i]; r != nil {
		r.DrawPortrait(s.Env.Screen, portraitAt[i].X, portraitAt[i].Y, s.idle[i] == 1)
		r.DrawBody(s.Env.Screen, bodyAt[i].X, bodyAt[i].Y, c.Direction)
	}
	s.Env.Text.Draw(x+nameOffsetX, y+nameOffsetY, 0, false, true, s.Env.Screen, c.Name, 0, nameWidth, 0, 0xffff, 2)
	st := s.Status
	st.Level(c.Level, image.Pt(x+nameOffsetX, y+levelOffsetY), false)
	if c.Job != 0 {
		s.Env.Pics.Draw(s.Env.Screen, s.CareerIcon, x+careerOffsetX, y+careerOffsetY, true)
	}
	st.Job(c.Job, image.Pt(x+jobOffsetX, y+jobOffsetY), false, false)
	sx := x + statOffsetX
	st.HP(c.Value[slotHP], c.Value[slotHPMax], image.Pt(sx, y+hpOffsetY), false)
	st.SP(uint16(c.Value[slotSP]), uint16(c.Value[slotSPMax]), image.Pt(sx, y+spOffsetY), false)
	st.Exp(c.Level, c.Value[slotExp], c.Reborn != 0, image.Pt(sx, y+expOffsetY), false)
	st.Gold(c.Value[slotGold], image.Pt(sx, y+goldOffsetY), false)
}
