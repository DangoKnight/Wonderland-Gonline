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
		field.Clipboard = func() []byte { return []byte("Abcd1234567890xy") }
		c.UI.Focus(field)
		c.UI.KeyDown(seui.VKV, seui.ShiftCtrl)
		want := "Abcd123456"
		if i == 1 {
			want = "Abcd1234567890"
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
		if string(field.Text) != want[:len(want)-1]+"9" {
			t.Fatal("typing broken after paste", i)
		}
	}
}
