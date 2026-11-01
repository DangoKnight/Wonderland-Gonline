package hud

import (
	"strconv"
	"time"

	"wonderland-gonline/client/wlo/seui"
)

// The Social window's Set Up tab (LAB_0027eae4, mode 3, picture
// Form_setup_1): the nickname (Panel26 field), Online and Busy ticks,
// the birthday (year, month, day), the sun sign it gives, the blood type
// and the "physically attractive" switch. Editing makes the form dirty
// (+0x4b4, LAB_00279c74 for the date, the paint's nickname compare), which
// swaps Close for OK and Cancel. OK asks for confirmation, then sends the
// nickname (AC10:1) when it changed and, with a full date and a blood
// type, the profile (AC10:2) and a friend-list refresh (AC10:3)
// (0x27a73c); Cancel restores the saved values (LAB_0027aa14). Online and
// Busy take effect at once (FUN_0027966c, LAB_002796f4 → FUN_0027b188).
// Not ported: the illegal-word check of the nickname (FUN_004a6d00) and
// the attractive switch's heart over a matching nearby player
// (FUN_0027a304's scan, icon_Jumpheart_1).
type setupTab struct {
	nickname     *seui.Editor      // +0x128
	online, busy *seui.FixedButton // +0x110, +0x114
	date         [3]*seui.Editor   // +0x48c, +0x490, +0x494
	blood        [bloodTypes]*seui.FixedButton
	attractive   *seui.FixedButton // +0x4a8
	ok, cancel   *seui.FixedButton // +0x4cc, +0x4d0
	sign         *seui.Panel       // +0x4b0
	Blood        byte              // +0x485, 1..4 (A, B, O, AB)
	Attractive   bool              // +0x4ac
	Sign         byte              // +0x4ad
	dirty        bool              // +0x4b4
	savedNick    []byte            // +0x4b8
	savedDate    [3][]byte         // +0x4bc, +0x4c0, +0x4c4
	savedBlood   byte              // +0x4c8
	BusySelected bool              // +0x11d
}

const (
	bloodTypes   = 4
	bloodLeft    = 0x55
	bloodStep    = 0x32
	bloodTop     = 0x146
	checkSize    = 0x10
	setupPicture = "Form_setup_1"
	nicknameMax  = 0xe
	yearBase     = 1900
	yearMin      = 1000
	confirmSetUp = "Ensure your data is correct. System will be sending bday gift on specified date. Log in game to receive it."
	noBloodType  = "No blood type"
	signPrefix   = "SunSign_"
	signSuffix   = "_1"
)

// dateFields are the year, month and day fields' (left, width, max).
var dateFields = [3]struct{ left, width, max int }{{0x46, 0x28, 4}, {0x83, 0x17, 2}, {0xad, 0x17, 2}}

// signCuts are FUN_00279c88's sun signs by month: up to the cut day the
// first sign, after it the second.
var signCuts = [13]struct{ cut, low, high byte }{
	1: {20, 10, 11}, 2: {19, 11, 12}, 3: {20, 12, 1}, 4: {20, 1, 2}, 5: {20, 2, 3}, 6: {21, 3, 4},
	7: {22, 4, 5}, 8: {22, 5, 6}, 9: {22, 6, 7}, 10: {22, 7, 8}, 11: {22, 8, 9}, 12: {22, 9, 10},
}

// SunSign is FUN_00279c88: 0 without a full date.
func SunSign(year, month, day int) byte {
	if year == 0 || month < 1 || month > 12 || day == 0 {
		return 0
	}
	c := signCuts[month]
	if day > int(c.cut) {
		return c.high
	}
	return c.low
}

// ClampDate is FUN_0027977c: a year past now becomes this year, one in
// 1000..1899 becomes 1900; months stop at 12, days at the month's length
// (February by year mod 4), and a date in this year stops at today. Zero
// means an empty field.
func ClampDate(year, month, day int, now time.Time) (int, int, int) {
	if year != 0 && year > now.Year() {
		year = now.Year()
	}
	if year != 0 && year < yearBase && year >= yearMin {
		year = yearBase
	}
	if month > 12 {
		month = 12
	}
	if month != 0 && day != 0 {
		limit := 31
		switch month {
		case 4, 6, 9, 11:
			limit = 30
		case 2:
			if year == 0 {
				break
			}
			limit = 28
			if year%4 == 0 {
				limit = 29
			}
		}
		day = min(day, limit)
	}
	if year != 0 && month != 0 && year == now.Year() {
		month = min(month, int(now.Month()))
		if day != 0 && month == int(now.Month()) {
			day = min(day, now.Day())
		}
	}
	return year, month, day
}

func (s *Social) newSetupTab() {
	t := &s.setup
	env := s.Env
	button := func(picture string, left, top, w, h int, click func()) *seui.FixedButton {
		b := seui.NewFixedButton(env, s)
		b.Init(picture, left, h, w, 0, 0, true, h, w, top)
		b.OnClick = click
		b.Visible = false
		return b
	}
	t.nickname = seui.NewEditor(env, s)
	t.nickname.Init("Panel26", 0x7d, 0xe, 0x68, 0, 0, true, 0x13, 0x82, 0x4b)
	t.nickname.SetColor(labelColor)
	t.nickname.AllowPaste = true
	t.nickname.SetMaxLen(nicknameMax)
	t.nickname.Visible = false
	t.online = button("btn_check_1", 0x90, 0x6b, checkSize, checkSize, func() { s.setStatus(false) })
	t.busy = button("btn_Uncheck_1", 0xc7, 0x6b, checkSize, checkSize, func() { s.setStatus(true) })
	for i, f := range dateFields {
		ed := seui.NewEditor(env, s)
		ed.Init("", f.left, 0x12, f.width, 0, 0, false, 0x12, f.width, 0x88)
		ed.SetColor(labelColor)
		ed.AllowPaste = true
		ed.Filter = 1 // digits (FUN_00468eac)
		ed.SetMaxLen(f.max)
		ed.Visible = false
		ed.OnChange = s.dateChanged
		t.date[i] = ed
	}
	for i := range t.blood {
		n := byte(i + 1)
		t.blood[i] = button("btn_UnCheck_1", i*bloodStep+bloodLeft, bloodTop, checkSize, checkSize, func() { s.pickBlood(n) })
	}
	t.attractive = button("Btn_off_1", 0x68, 0x162, 0x27, 0x14, s.toggleAttractive)
	t.ok = button("Btn_OK_2", 0x7a, 0x1ca, 0x27, 0x14, s.confirmSetUp)
	t.cancel = button("Btn_Cancel_3", 200, 0x1ca, 0x27, 0x14, s.cancelSetUp)
	t.sign = seui.NewPanel(env, s)
	t.sign.Init("", 0x24, 0x8f, 0xae, 0, 0, true, 0x8f, 0xae, 0xa8)
	t.sign.Hover = false
	t.sign.Visible = false
}

// showSetUp shows or hides the Set Up tab and keeps the values to restore
// (LAB_0027eae4 copies them to +0x4b8…).
func (s *Social) showSetUp(on bool) {
	t := &s.setup
	for _, c := range []interface{ SetVisible(bool) }{t.nickname, t.online, t.busy, t.attractive, t.sign, t.date[0], t.date[1], t.date[2]} {
		c.SetVisible(on)
	}
	for _, b := range t.blood {
		b.SetVisible(on)
	}
	if !on {
		t.ok.SetVisible(false)
		t.cancel.SetVisible(false)
		return
	}
	s.keepSetUp()
	t.dirty = false
	s.refreshSetUp()
}

func (s *Social) keepSetUp() {
	t := &s.setup
	t.savedNick = append([]byte(nil), t.nickname.Text...)
	for i, ed := range t.date {
		t.savedDate[i] = append([]byte(nil), ed.Text...)
	}
	t.savedBlood = t.Blood
}

// LoadProfile is FUN_0027ab64: the self record's nickname, blood type
// (taken mod 5), birth year (offset from 1900), month and day (0 empty).
func (s *Social) LoadProfile(nickname []byte, blood, year, month, day byte) {
	t := &s.setup
	t.nickname.SetText(nickname)
	// An unset year shows "1900" (DAT_0027acd0).
	y := strconv.Itoa(int(year) + yearBase)
	t.date[0].SetText([]byte(y))
	for i, v := range []byte{month, day} {
		txt := ""
		if v != 0 {
			txt = strconv.Itoa(int(v))
		}
		t.date[i+1].SetText([]byte(txt))
	}
	t.Blood = blood % 5
	s.keepSetUp()
	s.updateSign()
	s.refreshSetUp()
}

// Profile is the form's nickname, blood type and date values.
func (s *Social) Profile() (nickname []byte, blood byte, year, month, day int) {
	t := &s.setup
	return t.nickname.Text, t.Blood, s.dateValue(0), s.dateValue(1), s.dateValue(2)
}

func (s *Social) dateValue(i int) int {
	v, _ := strconv.Atoi(string(s.setup.date[i].Text))
	return v
}

// dateChanged is LAB_00279c74: clamp the date and mark the form dirty.
func (s *Social) dateChanged() {
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	y, m, d := s.dateValue(0), s.dateValue(1), s.dateValue(2)
	cy, cm, cd := ClampDate(y, m, d, now)
	for i, pair := range [][2]int{{y, cy}, {m, cm}, {d, cd}} {
		if pair[0] != pair[1] {
			s.setup.date[i].SetText([]byte(strconv.Itoa(pair[1])))
		}
	}
	s.updateSign()
	s.setup.dirty = true
	s.refreshSetUp()
}

func (s *Social) updateSign() {
	s.setup.Sign = SunSign(s.dateValue(0), s.dateValue(1), s.dateValue(2))
}

// pickBlood is FUN_0027a1dc: the blood type ticks act as one choice that
// a second click clears.
func (s *Social) pickBlood(n byte) {
	t := &s.setup
	if t.Blood == n {
		t.Blood = 0
	} else {
		t.Blood = n
	}
	t.dirty = true
	s.refreshSetUp()
}

// setStatus is Online (FUN_0027966c) or Busy (LAB_002796f4).
func (s *Social) setStatus(busy bool) {
	s.setup.BusySelected = busy
	s.refreshSetUp()
	if s.SetStatus != nil {
		s.SetStatus(busy)
	}
}

// toggleAttractive is LAB_0027a288; the switch is saved with the emotes
// (FUN_0027b254).
func (s *Social) toggleAttractive() {
	s.setup.Attractive = !s.setup.Attractive
	s.refreshSetUp()
	if s.EmotesChanged != nil {
		s.EmotesChanged()
	}
}

// Attractive reports the "physically attractive" switch; SetAttractive
// restores it from the saved emotes.
func (s *Social) Attractive() bool { return s.setup.Attractive }

func (s *Social) SetAttractive(on bool) {
	s.setup.Attractive = on
	s.refreshSetUp()
}

// refreshSetUp sets the pictures and the OK/Cancel/Close swap (the paint,
// FUN_0027b41c).
func (s *Social) refreshSetUp() {
	t := &s.setup
	pics := s.Env.Pics
	check := func(on bool) int {
		if on {
			return pics.Find("btn_Check_1")
		}
		return pics.Find("btn_UnCheck_1")
	}
	t.online.Image, t.busy.Image = check(!t.BusySelected), check(t.BusySelected)
	for i, b := range t.blood {
		b.Image = check(t.Blood == byte(i+1))
	}
	t.attractive.Image = pics.Find("Btn_off_1")
	if t.Attractive {
		t.attractive.Image = pics.Find("Btn_on_1")
	}
	t.sign.Image = -1
	if t.Sign != 0 {
		t.sign.Image = pics.Find(signPrefix + strconv.Itoa(int(t.Sign)) + signSuffix)
	}
	if s.Tab != SocialSetUp {
		return
	}
	dirty := t.dirty || string(t.nickname.Text) != string(t.savedNick)
	t.ok.SetVisible(dirty)
	t.cancel.SetVisible(dirty)
	s.Close.SetVisible(!dirty)
}

// confirmSetUp is OK: the talk window's confirmation, then 0x27a73c.
func (s *Social) confirmSetUp() {
	if s.Confirm != nil {
		s.Confirm([]byte(confirmSetUp), s.saveSetUp)
	}
}

// saveSetUp is 0x27a73c.
func (s *Social) saveSetUp() {
	t := &s.setup
	for _, ed := range t.date {
		if len(ed.Text) == 0 {
			ed.SetText([]byte("0"))
		}
	}
	if string(t.nickname.Text) != string(t.savedNick) {
		t.savedNick = append([]byte(nil), t.nickname.Text...)
		if s.SaveNickname != nil {
			s.SaveNickname(t.savedNick)
		}
	}
	for i, ed := range t.date {
		t.savedDate[i] = append([]byte(nil), ed.Text...)
	}
	t.savedBlood = t.Blood
	// FUN_0027a304: clamp, then the profile counts only when complete.
	y, m, d := ClampDate(s.dateValue(0), s.dateValue(1), s.dateValue(2), s.now())
	valid := y != 0 && m != 0 && d != 0 && t.Blood != 0
	if !valid {
		t.Attractive = false
		s.refreshSetUp()
		if s.Notice != nil {
			s.Notice(noBloodType)
		}
		return
	}
	if y >= yearBase && s.SaveProfile != nil {
		s.SaveProfile(s.setup.Sign, t.Blood, byte(y-yearBase), byte(m), byte(d))
	}
	s.updateSign()
	t.dirty = false
	s.refreshSetUp()
	if s.RequestFriends != nil {
		s.RequestFriends()
	}
}

func (s *Social) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// cancelSetUp is LAB_0027aa14.
func (s *Social) cancelSetUp() {
	t := &s.setup
	t.nickname.SetText(t.savedNick)
	for i, ed := range t.date {
		ed.SetText(t.savedDate[i])
	}
	t.Blood = t.savedBlood
	t.dirty = false
	s.updateSign()
	s.refreshSetUp()
}

// SetProfile fills the form's fields as if typed (tests and tools); the
// form becomes dirty.
func (s *Social) SetProfile(nickname []byte, blood byte, year, month, day int) {
	t := &s.setup
	t.nickname.SetText(nickname)
	for i, v := range []int{year, month, day} {
		t.date[i].SetText([]byte(strconv.Itoa(v)))
	}
	t.Blood = blood
	s.dateChanged()
}

// SaveSetUp is the confirmation's OK (0x27a73c).
func (s *Social) SaveSetUp() { s.saveSetUp() }
