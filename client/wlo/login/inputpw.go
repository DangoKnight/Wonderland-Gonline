package login

import (
	"bytes"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
)

// TRe_InputPwAndDarkPw (VMT 0x21309c, constructor FUN_0021a8c0, DAT_006c6f40
// and PTR_DAT_004ca5fc): four masked fields for a login password and a
// secret (deletion) code, each typed twice. Mode 1 is character creation,
// mode 2 character deletion.
const (
	pwFields       = 4
	pwMinLen       = 6
	pwMaxLen       = 10
	pwFieldLeft    = 0x8d
	pwFieldTop     = 0x7b
	pwFieldStep    = 0x30
	pwButtonTop    = 0x138
	pwLeftButton   = 0x156
	pwRightButton  = 0x1bc
	pwFormHeight   = 0x160
	pwFormWidth    = 0x218
	pwCentreXShift = -6
	pwCentreYShift = 0xb
)

// Password check results (FUN_0021a60c).
const (
	PwOK = iota
	PwMismatch
	PwCodeMismatch
	PwBadLength
	PwBadCodeLength
	PwSameAsCode
)

type InputPassword struct {
	seui.FixedForm
	Assets Assets
	Notify Notifier

	Fields     [pwFields + 1]*seui.Editor // +0xec..+0xf8: password, confirm, code, confirm
	OK, Second *seui.FixedButton          // +0xfc, +0x100
	Field      byte                       // +0x108 the field Enter moves from
	Background *surface.Surface           // +0x104
}

func NewInputPassword(env *seui.Env, a Assets) *InputPassword {
	f := &InputPassword{Assets: a}
	f.InitFixedForm(f, env)
	f.Name = "TRe_InputPwAndDarkPw"
	f.Init("", 0, 0, 0, 0, 0, false, pwFormHeight, pwFormWidth, 0)
	f.OK = seui.NewFixedButton(env, f)
	f.OK.Init("btn_ok_1", pwRightButton, 0x14, 0x38, 0, 0, true, 0x14, 0x38, pwButtonTop)
	f.Second = seui.NewFixedButton(env, f)
	f.Second.Init("Btn_Prev_1", pwLeftButton, 0x14, 0x38, 0, 0, true, 0x14, 0x38, pwButtonTop)
	seui.NewEditor(env, f) // +0xe8, created and never placed
	for i := 1; i <= pwFields; i++ {
		e := seui.NewEditor(env, f)
		top := (i-1)*pwFieldStep + pwFieldTop
		e.Init("Panel26", 0x95, 0x15, 0x78, 0, 0, true, 0xe, 0x68, top)
		e.SetMargins(1, 1, 1, 1)
		e.SetColor(skinTextColor)
		e.Color2 = 0
		e.Init("Panel26", pwFieldLeft, 0x15, 0x68, 0, 0, true, 0xe, 0x89, top)
		e.SetMaxLen(pwMaxLen)
		e.Password = true
		e.Tag = i
		e.OnClickTag = f.fieldClicked
		e.OnEnter, e.OnTab = f.nextField, f.nextField
		f.Fields[i] = e
	}
	return f
}

// SetMode is FUN_0021a3f4: the background, centred on the screen, and the
// buttons. Mode 1 has OK on the right and Previous on the left; mode 2 has
// OK on the left and Cancel on the right. The second button hides the
// form; the caller sets OK's action.
func (f *InputPassword) SetMode(mode byte) {
	var name string
	switch mode {
	case 1:
		name = "Form_InputPw_1"
		f.OK.Image = f.Env.Pics.Find("Btn_Ok_1")
		f.OK.Left = pwRightButton
		f.Second.Image = f.Env.Pics.Find("Btn_Prev_1")
		f.Second.Left = pwLeftButton
	case 2:
		name = "Form_InputPw_2"
		f.OK.Image = f.Env.Pics.Find("Btn_Ok_1")
		f.OK.Left = pwLeftButton
		f.Second.Image = f.Env.Pics.Find("Btn_Cancel_1")
		f.Second.Left = pwRightButton
	default:
		return
	}
	f.OK.OnClick = nil
	f.Second.OnClick = f.Hide
	f.Background, _ = loadJPEG(f.Assets.SkinPicture("Menu", "Skins", "White", name+".jpg"))
	w, h := 0, 0
	if f.Background != nil {
		w, h = f.Background.W, f.Background.H
	}
	f.Left = (screenWidth-w)/2 + pwCentreXShift
	f.Top = (screenHeight-h)/2 + pwCentreYShift
}

// Show is slot +0x20 (FUN_0021a3d0): the fields are cleared (FUN_0021accc).
func (f *InputPassword) Show() {
	for _, e := range f.Fields[1:] {
		e.SetText(nil)
	}
	f.FixedForm.Show()
}

// Paint is slot +0x10 (FUN_0021acec): the background at the form's
// position.
func (f *InputPassword) Paint() {
	if f.Background != nil {
		a := f.Abs()
		f.Env.Screen.Draw(a.X, a.Y, f.Background, false)
	}
}

// fieldClicked is 0x21acbc.
func (f *InputPassword) fieldClicked(tag int) {
	if tag >= 1 && tag <= pwFields {
		f.Field = byte(tag)
	}
}

// nextField is 0x21ac6c: Enter and Tab move to the next field; after the
// last, OK is clicked.
func (f *InputPassword) nextField() {
	f.Field++
	if f.Field > pwFields {
		if f.OK.OnClick != nil {
			f.OK.OnClick()
		}
		f.Field = 0
		return
	}
	f.Env.UI.Focus(f.Fields[f.Field])
}

// Password and Code are the first and third fields.
func (f *InputPassword) Password() []byte { return f.Fields[1].Text }
func (f *InputPassword) Code() []byte     { return f.Fields[3].Text }

// Check is FUN_0021a60c; with notify set the problem is shown.
func (f *InputPassword) Check(notify bool) int {
	say := func(r int, text string) int {
		if notify && f.Notify != nil {
			f.Notify([]byte(text), noticeTime)
		}
		return r
	}
	pw, code := f.Fields[1].Text, f.Fields[3].Text
	switch {
	case !bytes.Equal(pw, f.Fields[2].Text):
		return say(PwMismatch, "Wrong Passwords")
	case !bytes.Equal(code, f.Fields[4].Text):
		return say(PwCodeMismatch, "Wrong Del Pwd")
	case len(pw) < pwMinLen:
		return say(PwBadLength, "Password less 6 char")
	case len(code) < pwMinLen:
		return say(PwBadCodeLength, "Del Pwd less 6 char")
	case len(pw) > pwMaxLen:
		return say(PwBadLength, "Password is too long")
	case len(code) > pwMaxLen:
		return say(PwBadCodeLength, "Del Pwd is too long")
	case bytes.Equal(pw, code):
		return say(PwSameAsCode, "Pwd & Del are same!")
	}
	return PwOK
}
