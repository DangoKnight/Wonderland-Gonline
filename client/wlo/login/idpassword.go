package login

import (
	"os"
	"time"

	"wonderland-go/client/wlo/seui"
	"wonderland-go/client/wlo/surface"
)

// Account form layout from FUN_003ffbc0, FUN_0040063c and FUN_00400420.
const (
	loginBackX, loginBackY     = 0x10b, 0x87
	recordIconX, recordIconY   = 0x147, 0x130
	fieldInk                   = 0x1f
	fieldMaxBytes              = 10
	noticeDuration             = 3 * time.Second
	accountFile                = "AccountList.dat"
	accountFieldName           = "IDField"
	numberedAccountPrefix      = "WR"
	numberedAccountLetter      = 'O'
	numberedAccountReplacement = '0'
)

// Notices of the account form. accountInvalid is the Big5 text at
// DAT_00400404 ("帳號輸入不正確").
var (
	noticeIncorrectName = []byte("Incorrect name")
	noticeNoPassword    = []byte("Pwd is empty")
	noticeAccount       = []byte("\xb1\x62\xb8\xb9\xbf\xe9\xa4\x4a\xa4\xa3\xa5\xbf\xbd\x54")
)

// Notifier is the notice object at PTR_DAT_004c9e40 (slot +0x9c: text and
// duration), which is not traced yet.
type Notifier func(text []byte, d time.Duration)

// IDPassword is TSe_IDPassored (constructor FUN_003ffbc0).
type IDPassword struct {
	seui.Form
	G      *Globals
	Net    *Net
	Assets Assets
	Notify Notifier

	Login, Prev, About, Apply *seui.FixedButton // +0x130..+0x13c
	Background                *surface.Surface  // +0x140
	Account                   *seui.AccountList // +0x148
	Check                     *seui.FixedButton // +0x14c
	Remember                  bool              // +0x150
	RecordIcon                int               // +0x154
	Password                  *seui.Editor      // +0x158
	LoginTime                 time.Time         // +0x160, zero while idle

	// Servers is the server list form (DAT_007a15a4).
	Servers *SelectServer
	// OnAbout and OnApply open web pages in the original.
	OnAbout, OnApply func()
}

func NewIDPassword(env *seui.Env, g *Globals, n *Net, a Assets) *IDPassword {
	f := &IDPassword{G: g, Net: n, Assets: a}
	f.InitForm(f, env)
	f.Name = "IDPassword"
	f.build()
	return f
}

func (f *IDPassword) build() {
	env := f.Env
	f.Init("", 0, 0x32, 0x32, 0, 0, true, 600, 800, 0)
	if m, err := loadJPEG(f.Assets.SkinPicture("Menu", "Skins", "White", "Form_IdPassword_1.jpg")); err == nil {
		f.Background = m
	}
	f.Shown = false

	a := seui.NewAccountList(env, f)
	f.Account = a
	a.Name = accountFieldName
	a.Init("Panel26", 0x16d, 0x15, 0x78, 0, 0, true, 0xc, 0x68, 0x110)
	a.SetMargins(1, 1, 1, 1)
	a.SetColor(fieldInk)
	a.Color2 = 0
	a.SetText(nil)
	a.OnEnter, a.OnTab = f.checkAccount, f.checkAccount
	a.AllowPaste = true
	a.SetMaxLen(fieldMaxBytes)
	a.Setup("Btn_ArrowDn_1", 0, 1, 0, "", "", -3)
	a.Save = f.saveAccounts

	p := seui.NewEditor(env, f)
	f.Password = p
	p.Init("Panel26", 0x16d, 0x15, 0x78, 0, 0, true, 0xc, 0x66, 0x130)
	p.SetMargins(1, 1, 1, 1)
	p.SetColor(fieldInk)
	p.Color2 = 0
	p.SetText(nil)
	p.Password = true
	p.OnEnter, p.OnTab = f.clickLogin, f.clickLogin
	p.AllowPaste = true
	p.SetMaxLen(fieldMaxBytes)

	f.Login = f.button("Btn_Login_L", 0x168, f.Submit)
	f.Prev = f.button("Btn_Prev_L", 0x186, f.Previous)
	f.About = f.button("Btn_About_L", 0x1c2, func() { call(f.OnAbout) })
	f.Apply = f.button("Btn_applyID_1", 0x1a4, func() { call(f.OnApply) })

	f.Remember = false
	f.RecordIcon = env.Pics.Find("Icon_RecordAccount")
	f.Check = seui.NewFixedButton(env, f)
	f.Check.Init("btn_UnCheck_1", 0x148, 0x10, 0x10, 0, 0, true, 0x10, 0x10, 0x14a)
	f.Check.OnClick = f.toggleRemember
}

func call(fn func()) {
	if fn != nil {
		fn()
	}
}

func (f *IDPassword) button(name string, top int, click func()) *seui.FixedButton {
	b := seui.NewFixedButton(f.Env, f)
	b.Init(name, 0x148, 0x14, 0x8f, 0, 0, true, 0x14, 0x8f, top)
	b.OnClick = click
	return b
}

// clickLogin is 0x400414: Enter in the password field clicks Login.
func (f *IDPassword) clickLogin() { f.Login.Click() }

// toggleRemember is 0x400788.
func (f *IDPassword) toggleRemember() {
	f.Remember = !f.Remember
	name := "btn_UnCheck_1"
	if f.Remember {
		name = "btn_Check_1"
	}
	f.Check.Image = f.Env.Pics.Find(name)
}

func (f *IDPassword) notify(text []byte) {
	if f.Notify != nil {
		f.Notify(text, noticeDuration)
	}
}

// checkAccount is FUN_004001f0, Enter or Tab in the account field. The
// branch for two Windows locales (FUN_00486a84, FUN_00486a70) that
// rejects "TH" and "TF" accounts with a dialog is not taken here.
func (f *IDPassword) checkAccount() {
	f.Account.SetText(f.Account.Text)
	if !ValidAccount(f.Account.Text) {
		f.notify(noticeAccount)
		f.Account.Clear()
		return
	}
	f.Env.UI.Focus(f.Password)
}

// ValidAccount is FUN_004a91d4. The original also checks numbered
// accounts against a range table that nothing fills.
func ValidAccount(s []byte) bool {
	s = trim(s)
	if len(s) <= 2 {
		return false
	}
	prefix := upper(trim(copyStr(s, 1, 2)))
	if len(prefix) != 2 {
		return false
	}
	if string(prefix) != numberedAccountPrefix {
		return len(trim(s)) <= fieldMaxBytes
	}
	n, ok := strToInt(trim(copyStr(s, 3, len(s))))
	if !ok {
		return false
	}
	n = numberedAccount(n)
	return n >= 1 && n <= numberedAccountMax
}

// numberedAccountMax and numberedAccount are FUN_004852f8.
const numberedAccountMax = 4500000

func numberedAccount(n int) int {
	if n > 0 && n < 0x895441 && n > numberedAccountMax {
		n -= numberedAccountMax
	}
	return n
}

// Submit is FUN_00400420, the Login button.
func (f *IDPassword) Submit() {
	acct := f.Account.Text
	if string(upper(copyStr(acct, 1, 2))) == numberedAccountPrefix {
		fixed := append([]byte(nil), acct...)
		for i := 3; i <= len(fixed); i++ {
			if upper([]byte{fixed[i-1]})[0] == numberedAccountLetter {
				fixed[i-1] = numberedAccountReplacement
			}
		}
		f.Account.SetText(fixed)
	}
	switch {
	case !ValidAccount(f.Account.Text):
		f.notify(noticeIncorrectName)
		f.Account.Clear()
	case len(f.Password.Text) == 0:
		f.notify(noticeNoPassword)
	default:
		f.Net.Send(f.LoginPacket())
		f.wipePassword()
		f.Hide()
		f.LoginTime = time.Now()
	}
}

// wipePassword overwrites the field after sending, as the 63/4 send does.
func (f *IDPassword) wipePassword() {
	f.Password.SetText([]byte("12345678"))
	f.Password.SetText(nil)
}

// Previous is FUN_00400170: back to the server list.
func (f *IDPassword) Previous() {
	f.Hide()
	f.Net.Close()
	if f.Servers != nil {
		f.Servers.Show()
	}
	f.Password.Clear()
	f.LoginTime = time.Time{}
}

// Show is slot +0x20 (0x4006d4).
func (f *IDPassword) Show() {
	if f.G.Mode == 2 {
		return
	}
	f.Form.Show()
	f.Env.UI.Focus(f.Account)
	f.Password.SetText(nil)
	f.loadAccounts()
}

// Hide is slot +0x24 (0x40075c).
func (f *IDPassword) Hide() {
	f.Form.Hide()
	f.Password.SetText(nil)
}

func (f *IDPassword) accountPath() string { return f.Assets.UserPath(accountFile) }

// loadAccounts is FUN_00402d48.
func (f *IDPassword) loadAccounts() {
	raw, err := os.ReadFile(f.accountPath())
	if err != nil {
		f.Account.List.Clear()
		return
	}
	f.Account.LoadEntries(splitLines(raw))
}

// saveAccounts is FUN_00402c30, or the file's deletion when the list is
// empty (FUN_00402b5c).
func (f *IDPassword) saveAccounts(items [][]byte) {
	if len(items) == 0 {
		os.Remove(f.accountPath())
		return
	}
	os.WriteFile(f.accountPath(), joinLines(items), 0o644)
}

// Paint is slot +0x10 (FUN_0040063c): the background and the record icon
// at screen positions, around the form's own panel.
func (f *IDPassword) Paint() {
	if f.Background != nil {
		f.Env.Screen.Draw(loginBackX, loginBackY, f.Background, false)
	}
	f.Panel.Paint()
	f.Env.Pics.Draw(f.Env.Screen, f.RecordIcon, recordIconX, recordIconY, true)
}
