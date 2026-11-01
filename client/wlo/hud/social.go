package hud

import (
	"image"
	"strconv"
	"time"

	"wonderland-gonline/client/wlo/seui"
)

// Social is the main menu's Social window (constructor FUN_0027c6a4, a
// TSe_FixedForm held at DAT_006c70d4; paint FUN_0027b41c, show
// FUN_00278b34): 0x164 × 0x1ed at (200, 31), with the Friend, Emote and Set Up
// tabs. The Friend tab (FUN_0027eeb4, mode 2) lists five friends a page
// (FUN_0027b5e8): the portrait, name, level, element, presence, job,
// nickname, guild, a mail icon and a tick box per row, with page arrows,
// Send Mail and Del Friend (the mail form is MailSend). The Marry tab
// (married players only) and VIP marks are not ported.
type Social struct {
	seui.FixedForm
	Tabs                [socialTabs]*seui.FixedButton // +0x170, +0x164, +0x10c
	Marry               *seui.FixedButton             // +0x118
	Close, CloseCorner  *seui.FixedButton             // +0x160, +0x15c
	SendMail, DelFriend *seui.FixedButton             // +0x120, +0x124
	PrevPage, NextPage  *seui.FixedButton             // +0x440, +0x444
	rows                [socialRows]socialRow
	Friends             []Friend // the friend table (DAT_0072a390), the GM entry first
	Page                int      // +0x390
	Ticked              int      // +0x108
	Tab                 int
	digits              int
	DrawPortrait        func(f Friend, x, y int)
	DrawLevel           func(level byte, x, y int)
	Presence            func(id uint32) byte
	Notice              func(text string)
	Confirm             func(text []byte, yes func())
	Remove              func(id uint32)
	emote               emoteTab
	// Emote plays an expression code, Pose an action cell, Phrase puts a
	// Ctrl phrase in the chat bar; EmotesChanged saves slots and phrases.
	Emote         func(code byte)
	Pose          func(cell int)
	Phrase        func(text []byte)
	EmotesChanged func()
	setup         setupTab
	// Set Up hooks: SaveNickname sends AC10:1, SaveProfile AC10:2 with
	// the sun sign (the app computes the social code), RequestFriends
	// AC10:3, SetStatus the Online/Busy choice; Now is the clock for the
	// date clamps.
	SaveNickname   func(nickname []byte)
	SaveProfile    func(sign, blood, year, month, day byte)
	RequestFriends func()
	SetStatus      func(busy bool)
	Now            func() time.Time
	// OpenMail opens the mail form for one friend (a row's mail icon,
	// LAB_00279248) or the ticked friends (Send Mail, LAB_0027b050).
	OpenMail func(mode byte, to Friend, recipients []Friend)
}

// Friend is one record of the friend table (0x43 bytes, filled by 14/5,
// FUN_002dde1c case 0xe/5): the fields in the order the packet gives them.
type Friend struct {
	ID       uint32 // +0
	Mail     bool   // +4: an unread mail from the friend
	Name     []byte // +6
	Level    byte   // +0x15, drawn as "LV n"
	Job      byte   // +0x16, the job icon (1..6)
	Field17  byte   // +0x17, copied to the role's +0x1f99; not drawn here
	Element  byte   // +0x18
	Body     byte   // +0x19
	Head     byte   // +0x1a
	Color1   uint32 // +0x1b
	Color2   uint32 // +0x1f
	Nickname []byte // +0x23
	Guild    []byte // +0x32
	Ticked   bool   // +0x41
	VIP      byte   // +0x42
}

// The GM entry (FUN_003c4130): the client keeps record 0 as ID 100,
// "GM中心" in Big5, whatever the list says.
var (
	GMFriendID   uint32 = 100
	GMFriendName        = []byte{'G', 'M', 0xa4, 0xa4, 0xa4, 0xdf}
)

const (
	socialTabs     = 3
	SocialFriend   = 0
	SocialEmote    = 1
	SocialSetUp    = 2
	socialRows     = 5
	socialWidth    = 0x164
	socialHeight   = 0x1ed
	socialPicture  = "Form_Emotion_1"
	friendPicture  = "Form_Friend_2"
	socialRowStep  = 0x41 // labels, ticks and job icons
	socialIconStep = 0x42 // mail, element and presence icons
	// Presence values (+0x428): online, busy, the third icon, offline.
	PresenceOnline  = 0
	PresenceBusy    = 1
	PresenceInline  = 2
	PresenceOffline = 3
	// mailBlink: the unread mail icon alternates every five refreshes
	// (+0x458, +0x454).
	mailBlink = 5
	// The page numbers: Num_MainInfo cut into height/11 rows, the
	// digits then "/" at row 11 (the sheet's last ten pixels).
	pageX        = 0xa0
	pageX2       = 0xaa
	pageY        = 400
	pageDigitRow = 11
	pageRows     = 11
	// Row portrait and level offsets (FUN_0027b5e8).
	rowLeft    = 0x19
	rowTop     = 0x46
	portraitX  = 0x1f
	portraitY  = 0x1b
	levelX     = 0xb4
	levelY     = 3
	labelColor = 0x0841
)

// socialTabPlaces are the tab buttons (Btn_FriendIcon_1,
// Btn_EmuationalIcon_1, Btn_setup_2) at y 0x2b, 0x42 × 0x15.
var socialTabPlaces = [socialTabs]struct {
	picture string
	left    int
}{{"Btn_FriendIcon_1", 0x14}, {"Btn_EmuationalIcon_1", 0x55}, {"Btn_setup_2", 0x97}}

var jobIcons = map[byte]struct{ picture, hint string }{
	1: {"Icon_Asn", "Killer"}, 2: {"Icon_Samurai", "Warrior"}, 3: {"Icon_Knight", "Knight"},
	4: {"Icon_Sage", "Sage"}, 5: {"Icon_Clergyman", "Priest"}, 6: {"Icon_Prophet", "Prophet"},
}

var presenceIcons = [...]string{"Icon_online_1", "Icon_Busy_1", "Icon_inline_1", "icon_offline_1"}

// socialRow is one friend row's controls.
type socialRow struct {
	name, nickname, guild  *seui.Editor      // +0x3c8, +0x144, +0x12c
	element, presence, job *seui.Panel       // +0x3b0, +0x410, +0x560
	mail, mailGray         *seui.Panel       // +0x174, +0x18c
	tick                   *seui.FixedButton // +0xe8
}

// NewSocial is FUN_0027c6a4's Friend-tab part.
func NewSocial(env *seui.Env) *Social {
	s := &Social{Page: 1}
	s.InitFixedForm(s, env)
	s.Name = "TSe_EmotiomForm"
	s.Init(socialPicture, (seui.MainAreaW-socialWidth)/2, 0, 0, 0, 0, false, socialHeight, socialWidth, (seui.MainAreaH-socialHeight)/2)
	button := func(picture string, left, top, w, h int, click func()) *seui.FixedButton {
		b := seui.NewFixedButton(env, s)
		b.Init(picture, left, h, w, 0, 0, true, h, w, top)
		b.OnClick = click
		return b
	}
	for i, t := range socialTabPlaces {
		i := i
		s.Tabs[i] = button(t.picture, t.left, 0x2b, 0x42, 0x15, func() { s.SelectTab(i) })
	}
	// Marry shows only for a married player (+0x3ca4, FUN_0027b41c);
	// the Friend tab hides it (FUN_0027eeb4).
	s.Marry = button("Btn_Marry", 0xd9, 0x2b, 0x42, 0x15, nil)
	s.Marry.Visible = false
	s.Close = button("Btn_Close_1", 0x98, 0x1ca, 0x38, 0x14, s.Hide)
	s.CloseCorner = button("Btn_Close_s_1", 0x130, 0x18, 0x12, 0x12, s.Hide)
	s.SendMail = button("btn_sendmail_3", 0x6f, 0x1a4, 0x3d, 0x14, s.sendMail)
	s.DelFriend = button("btn_delFriend_1", 0xb3, 0x1a4, 0x3d, 0x14, s.delFriend)
	s.PrevPage = button("Btn_ArrowL_8", 0x86, 0x188, 0x15, 0x15, func() { s.turn(-1) })
	s.NextPage = button("Btn_ArrowR_8", 0xc5, 0x188, 0x15, 0x15, func() { s.turn(1) })
	label := func(left, top, w, h int) *seui.Editor {
		e := seui.NewEditor(env, s)
		e.Init("", left, h, w, 0, 0, false, h, w, top)
		e.SetColor(labelColor)
		e.ReadOnly = true
		e.Visible = false
		return e
	}
	panel := func(picture string, left, top, w, h int) *seui.Panel {
		p := seui.NewPanel(env, s)
		p.Init(picture, left, h, w, 0, 0, true, h, w, top)
		p.Hover = false
		p.Visible = false
		return p
	}
	for i := range s.rows {
		r := &s.rows[i]
		r.nickname = label(0x84, i*socialRowStep+0x5f, 0x82, 10)
		r.guild = label(0x84, i*socialRowStep+0x71, 0x82, 10)
		r.name = label(0x57, i*socialRowStep+0x3d, 0x5a, 0x28)
		r.job = panel("", 0xb5, i*socialRowStep+0x49, 0x10, 0x10)
		r.element = panel("", 0x10e, i*socialIconStep+0x49, 0x1e, 0x1e)
		r.presence = panel("", 0x104, i*socialIconStep+0x68, 0x20, 0x10)
		r.mailGray = panel("icon_mailGary_1", 0x131, i*socialIconStep+0x49, 0x14, 0xf)
		r.mail = panel("icon_mail_1", 0x131, i*socialIconStep+0x49, 0x14, 0xf)
		row := i
		r.mail.OnClick = func() { s.MailRow(row) }
		r.tick = button("btn_UnCheck_1", 0x131, i*socialRowStep+0x6b, 0x10, 0x10, func() { s.TickRow(row) })
		r.tick.Visible = false
	}
	s.digits = env.Pics.Find("Num_MainInfo")
	s.newEmoteTab()
	s.newSetupTab()
	s.Friends = []Friend{{ID: GMFriendID, Name: GMFriendName}}
	s.SelectTab(SocialFriend)
	return s
}

// SetFriends is the end of 14/5 (FUN_003c1804 → FUN_003c4130): the list
// after the GM entry, ticks cleared, the page kept in range.
func (s *Social) SetFriends(list []Friend) {
	s.Friends = append([]Friend{{ID: GMFriendID, Name: GMFriendName}}, list...)
	s.Ticked = 0
	for i := range s.rows {
		s.rows[i].tick.Image = s.Env.Pics.Find("btn_UnCheck_1")
	}
	s.Page = min(s.Page, s.pages())
	s.Refresh()
}

// count is the friend count (DAT_0072b0ec): the records after the GM one.
func (s *Social) count() int { return len(s.Friends) - 1 }

// pages is +0x394: count / 5 + 1.
func (s *Social) pages() int { return s.count()/socialRows + 1 }

// SelectTab shows a tab's controls: the Friend tab (FUN_0027eeb4), the
// Emote tab (LAB_0027e890); Set Up is not ported and shows the bare
// picture.
func (s *Social) SelectTab(tab int) {
	s.Tab = tab
	for i, b := range s.Tabs {
		b.Sticky = i == tab
		b.State = 0
		if b.Sticky {
			b.State = 2
		}
	}
	friend := tab == SocialFriend
	for _, b := range []*seui.FixedButton{s.SendMail, s.DelFriend, s.PrevPage, s.NextPage} {
		b.Visible = friend
	}
	s.showEmote(tab == SocialEmote)
	s.showSetUp(tab == SocialSetUp)
	if !friend {
		s.Image = s.Env.Pics.Find(socialPicture)
		if tab == SocialSetUp {
			s.Image = s.Env.Pics.Find(setupPicture)
		} else {
			s.Close.SetVisible(true)
		}
		for i := range s.rows {
			r := &s.rows[i]
			for _, c := range []interface{ SetVisible(bool) }{r.name, r.nickname, r.guild, r.element, r.presence, r.job, r.mail, r.mailGray, r.tick} {
				c.SetVisible(false)
			}
		}
		return
	}
	s.Image = s.Env.Pics.Find(friendPicture)
	s.Refresh()
}

// Refresh is FUN_0027b5e8: the page's rows from the friend table.
func (s *Social) Refresh() {
	if s.Tab != SocialFriend {
		return
	}
	pics := s.Env.Pics
	first := (s.Page - 1) * socialRows
	for i := range s.rows {
		r := &s.rows[i]
		for _, c := range []interface{ SetVisible(bool) }{r.name, r.nickname, r.guild, r.mail, r.tick, r.job} {
			c.SetVisible(false)
		}
		r.element.Image, r.presence.Image = -1, -1
		r.job.SetHint(nil)
		r.mailGray.SetVisible(true)
		k := first + i
		if k >= len(s.Friends) {
			continue
		}
		f := s.Friends[k]
		r.mail.SetVisible(true)
		r.mail.Image = pics.Find("icon_mail_1")
		if k != 0 {
			r.tick.SetVisible(true)
		}
		r.nickname.SetText(f.Nickname)
		r.guild.SetText(f.Guild)
		r.nickname.SetVisible(true)
		r.guild.SetVisible(true)
		presence := byte(PresenceOffline)
		if s.Presence != nil {
			presence = s.Presence(f.ID)
		}
		if int(presence) < len(presenceIcons) {
			r.presence.Image = pics.Find(presenceIcons[presence])
		}
		r.presence.SetVisible(true)
		if f.Element <= 4 {
			r.element.Image = pics.Find("Icon_Element_" + strconv.Itoa(int(f.Element)) + "_1")
		}
		r.element.SetVisible(true)
		r.name.SetText(f.Name)
		r.name.SetVisible(true)
		if j, ok := jobIcons[f.Job]; ok {
			r.job.Image = pics.Find(j.picture)
			r.job.SetHint([]byte(j.hint))
			r.job.SetVisible(true)
		}
	}
}

// Paint is FUN_0027b41c with the parts of FUN_0027b5e8 that draw straight
// to the screen: the portraits, the levels and the page numbers.
func (s *Social) Paint() {
	s.FixedForm.Paint()
	if s.Tab == SocialSetUp {
		s.refreshSetUp()
	}
	if s.Tab != SocialFriend {
		return
	}
	at := s.Abs()
	first := (s.Page - 1) * socialRows
	for i := 0; i < socialRows && first+i < len(s.Friends); i++ {
		f := s.Friends[first+i]
		x, y := at.X+rowLeft, at.Y+rowTop+i*socialRowStep
		if s.DrawPortrait != nil {
			s.DrawPortrait(f, x+portraitX, y+portraitY)
		}
		if s.DrawLevel != nil {
			s.DrawLevel(f.Level, x+levelX, y+levelY)
		}
	}
	s.drawPages(at)
}

// drawPages draws "page / pages" with Num_MainInfo at full digit width.
func (s *Social) drawPages(at image.Point) {
	if s.digits < 0 {
		return
	}
	pics := s.Env.Pics
	w, h := pics.Size(s.digits)
	h /= pageRows
	cur, all := strconv.Itoa(s.Page), strconv.Itoa(s.pages())
	x := at.X + pageX
	if len(cur) == 2 {
		x = at.X + pageX2 - w
	}
	y := at.Y + pageY
	row := func(n, x int) {
		pics.DrawRect(s.Env.Screen, s.digits, x, y, image.Rect(0, n*h, w, n*h+h), true)
	}
	for i, c := range cur {
		row(int(c-'0'), x+i*w)
	}
	row(pageDigitRow, x+len(cur)*w)
	x = at.X + pageX + (len(cur)+1)*w
	for i, c := range all {
		row(int(c-'0'), x+i*w)
	}
}

// turn is the page arrows (LAB_0027901c, LAB_00279124).
func (s *Social) turn(d int) {
	p := s.Page + d
	if p < 1 || p > s.pages() {
		return
	}
	s.Page = p
	first := (p - 1) * socialRows
	for i := range s.rows {
		name := "btn_UnCheck_1"
		if k := first + i; k < len(s.Friends) && s.Friends[k].Ticked {
			name = "btn_Check_1"
		}
		s.rows[i].tick.Image = s.Env.Pics.Find(name)
	}
	s.Refresh()
}

// TickRow is a row's tick box (LAB_0027b0bc), rows counted from 0.
func (s *Social) TickRow(row int) {
	k := (s.Page-1)*socialRows + row
	if k >= len(s.Friends) {
		return
	}
	f := &s.Friends[k]
	f.Ticked = !f.Ticked
	name := "btn_UnCheck_1"
	if f.Ticked {
		name = "btn_Check_1"
		s.Ticked++
	} else {
		s.Ticked--
	}
	s.rows[row].tick.Image = s.Env.Pics.Find(name)
}

// sendMail is LAB_0027b050: without a tick it asks for one, otherwise
// the mail form opens in mode 2 for the ticked friends; their ticks clear
// as the original's send does.
func (s *Social) sendMail() {
	if s.Ticked == 0 {
		if s.Notice != nil {
			s.Notice("Select friends to mail")
		}
		return
	}
	var to []Friend
	for i := range s.Friends {
		if s.Friends[i].Ticked {
			to = append(to, s.Friends[i])
			s.Friends[i].Ticked = false
		}
	}
	s.Ticked = 0
	for i := range s.rows {
		s.rows[i].tick.Image = s.Env.Pics.Find("btn_UnCheck_1")
	}
	if s.OpenMail != nil {
		s.OpenMail(MailToTicked, Friend{}, to)
	}
}

// MailRow is a row's mail icon (LAB_00279248), rows counted from 0: the
// mail form for that friend.
func (s *Social) MailRow(row int) {
	k := (s.Page-1)*socialRows + row
	if k < len(s.Friends) && s.OpenMail != nil {
		s.OpenMail(MailToFriend, s.Friends[k], nil)
	}
}

// delFriend is FUN_0027ada0: one ticked friend asks "Delete this friend?"
// with the name, several "Delete these friends?"; Yes sends 14/4 for each
// (FUN_00279230, LAB_0027ad04). The ticks clear either way.
func (s *Social) delFriend() {
	if s.Ticked == 0 {
		if s.Notice != nil {
			s.Notice("Select friends to delete")
		}
		return
	}
	var ids []uint32
	var name []byte
	for i := range s.Friends {
		if i > 0 && s.Friends[i].Ticked {
			ids = append(ids, s.Friends[i].ID)
			name = s.Friends[i].Name
		}
	}
	text := []byte("Delete these friends?")
	if s.Ticked == 1 {
		text = append([]byte("Delete this friend?"), name...)
		for i := range s.Friends {
			s.Friends[i].Ticked = false
		}
		s.Ticked = 0
		for i := range s.rows {
			s.rows[i].tick.Image = s.Env.Pics.Find("btn_UnCheck_1")
		}
	}
	yes := func() {
		for _, id := range ids {
			if s.Remove != nil {
				s.Remove(id)
			}
		}
		s.Ticked = 0
		for i := range s.Friends {
			s.Friends[i].Ticked = false
		}
		for i := range s.rows {
			s.rows[i].tick.Image = s.Env.Pics.Find("btn_UnCheck_1")
		}
	}
	if s.Confirm != nil {
		s.Confirm(text, yes)
	}
}

// Toggle is the main menu's Social button (FUN_00295678 → FUN_0027f8dc).
func (s *Social) Toggle() {
	if s.Visible {
		s.Hide()
		return
	}
	s.Show()
	s.Refresh()
}
