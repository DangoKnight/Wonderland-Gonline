package ui

import (
	"image"
	"time"
)

// loginOrigin is the login form's origin. Constructor coordinates (aLogin
// FUN_003ffbc0) are relative to it; captures place every control 25 px above
// its constructor top. The form's own position is not traced yet.
var (
	loginOrigin   = image.Pt(0, -25)
	loginForm     = image.Pt(267, 135) // Form_IdPassword_1.jpg, measured
	accountArrow  = image.Pt(469, 244) // TAccountList drop-down, measured
	rememberLabel = image.Pt(327, 304) // Icon_RecordAccount, measured
)

// Field focus.
const (
	focusAccount = iota
	focusPassword
)

// Login is the account form (Form_IdPassword_1).
type Login struct {
	account, password editor
	focus             int
	remember          bool
	rememberBox       *button
	buttons           []*button
	shown             time.Time

	// OnLogin runs for a valid submission (FUN_00400420).
	OnLogin func(account, password string)
	// OnPrevious returns to the server list (FUN_00400170).
	OnPrevious func()
	// OnSignUp and OnAbout open the original's external pages; nil does nothing.
	OnSignUp, OnAbout func()
	message           string
	messageAt         time.Time
}

// Notify shows a notice for three seconds, the duration the original passes
// to its message display (3000 ms).
func (l *Login) Notify(msg string) {
	l.message, l.messageAt = msg, time.Now()
}

func at(x, y int) image.Point { return loginOrigin.Add(image.Pt(x, y)) }

func NewLogin() *Login {
	l := &Login{shown: time.Now()}
	// FUN_00468e8c(field, 10) sets +0x1b4 and an enable flag; read as the
	// maximum length (inferred).
	l.account = editor{at: at(0x16d, 0x110), width: 0x68, max: 10}
	l.password = editor{at: at(0x16d, 0x130), width: 0x66, max: 10, password: true}
	// FUN_004001f0: Enter in the account field moves to the password.
	l.account.onEnter = func() { l.focus = focusPassword }
	// LAB_00400414: Enter in the password field clicks Login.
	l.password.onEnter = l.submit
	l.rememberBox = &button{sprite: "btn_UnCheck_1.bmp", at: at(0x148, 0x14a)}
	l.rememberBox.onClick = func() {
		// FUN_00400788 swaps the box image.
		l.remember = !l.remember
		l.rememberBox.sprite = map[bool]string{true: "btn_Check_1.bmp", false: "btn_UnCheck_1.bmp"}[l.remember]
	}
	call := func(f *func()) func() {
		return func() {
			if *f != nil {
				(*f)()
			}
		}
	}
	l.buttons = []*button{
		{sprite: "Btn_Login_L.bmp", at: at(0x148, 0x168), onClick: l.submit},
		{sprite: "Btn_Prev_L.bmp", at: at(0x148, 0x186), onClick: call(&l.OnPrevious)},
		{sprite: "Btn_applyID_1.bmp", at: at(0x148, 0x1a4), onClick: call(&l.OnSignUp)},
		{sprite: "Btn_About_L.bmp", at: at(0x148, 0x1c2), onClick: call(&l.OnAbout)},
		l.rememberBox,
	}
	return l
}

// Reset clears the password and focuses the account field, as returning to
// the form does.
func (l *Login) Reset() {
	l.password.text = nil
	l.focus = focusAccount
	l.shown = time.Now()
}

func (l *Login) Account() string { return string(l.account.text) }

func (l *Login) SetAccount(s string) { l.account.text = []byte(s) }

// submit is FUN_00400420's validation.
func (l *Login) submit() {
	switch {
	case !validAccount(l.account.text):
		l.Notify("Incorrect name")
		l.focus = focusAccount
	case len(l.password.text) == 0:
		l.Notify("Please enter the password")
		l.focus = focusPassword
	default:
		if l.OnLogin != nil {
			l.OnLogin(string(l.account.text), string(l.password.text))
		}
	}
}

// validAccount stands in for FUN_004a91d4, which has not been traced: a
// non-empty account of printable ASCII.
func validAccount(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c <= 0x20 || c >= 0x7f {
			return false
		}
	}
	return true
}

func (l *Login) Update(a *Assets, in *Input) {
	for _, b := range l.buttons {
		b.update(a, in)
	}
	for i, e := range []*editor{&l.account, &l.password} {
		if in.Pressed && in.at().In(image.Rect(e.at.X, e.at.Y, e.at.X+0x68, e.at.Y+0x0e)) {
			l.focus = i
		}
	}
	for _, k := range in.Keys {
		if k == KeyTab {
			l.focus ^= 1
		}
	}
	if l.focus == focusAccount {
		l.account.update(in)
	} else {
		l.password.update(in)
	}
}

func (l *Login) Draw(a *Assets, dst *image.RGBA) error {
	if err := drawBackdrop(a, dst); err != nil {
		return err
	}
	form, err := a.Skin("Form_IdPassword_1.jpg", 1)
	if err != nil {
		return err
	}
	form.Draw(dst, loginForm.X, loginForm.Y, 0)
	// The caret blink period is not traced; 500 ms is a placeholder.
	caret := time.Since(l.shown)/(500*time.Millisecond)%2 == 0
	if err := l.account.draw(a, dst, l.focus == focusAccount, caret); err != nil {
		return err
	}
	if err := l.password.draw(a, dst, l.focus == focusPassword, caret); err != nil {
		return err
	}
	arrow, err := a.Skin("Btn_ArrowDn_1.bmp", 3)
	if err != nil {
		return err
	}
	arrow.Draw(dst, accountArrow.X, accountArrow.Y, 0)
	label, err := a.Skin("Icon_RecordAccount.bmp", 1)
	if err != nil {
		return err
	}
	label.Draw(dst, rememberLabel.X, rememberLabel.Y, 0)
	for _, b := range l.buttons {
		if err := b.draw(a, dst); err != nil {
			return err
		}
	}
	// The original's notice display is not traced; notices are centred
	// below the form until it is.
	if l.message != "" && time.Since(l.messageAt) < 3*time.Second {
		text := Big5(l.message)
		a.Text(dst, (ScreenWidth-8*len(text))/2, 492, text, rgb565(0xffff))
	}
	return nil
}
