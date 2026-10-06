package app

import (
	"testing"
	"wonderland-gonline/client/wlo/seui"
)

func TestLoginClipboardPaste(t *testing.T) {
	c := testClient(t)
	c.Login.Show()
	fields := []*seui.Editor{&c.Login.Account.Editor, c.Login.Password}
	for i, field := range fields {
		if field.Clipboard == nil || !field.AllowPaste {
			t.Fatal("login field not wired to clipboard", i)
		}
		field.SetText(nil)
		field.Clipboard = func() []byte { return []byte("Abcd12345678") }
		c.UI.Focus(field)
		c.UI.KeyDown(seui.VKV, seui.ShiftCtrl)
		want := "Abcd123456"
		if i == 0 {
			want = "ABCD123456"
		}
		if string(field.Text) != want {
			t.Fatal("paste did not respect field rules", i)
		}
		if field.Password != (i == 1) {
			t.Fatal("password masking changed")
		}
		// Release resets suppression; subsequent typing must still work.
		field.KeyDown(seui.VKBack, 0)
		field.Char('9')
		if len(field.Text) != 10 || field.Text[9] != '9' {
			t.Fatal("typing broken after paste", i)
		}
	}
}
