package seui

import "testing"

func TestOwnedLanguageBindingAndLiteralReplacement(t *testing.T) {
	root := NewForm(testEnv())
	field := NewEditor(root.Env, root)
	value := "English"
	field.BindLanguage("hint", func() { field.SetHint([]byte(value)) })
	field.SetText([]byte("Player input"))
	value = "Chinese"
	root.Base().RefreshLanguage()
	if string(field.HintText) != "Chinese" || string(field.Text) != "Player input" {
		t.Fatal("binding did not update presentation independently of input")
	}
	field.SetHint([]byte("Server literal"))
	value = "Third"
	root.Base().RefreshLanguage()
	if string(field.HintText) != "Server literal" {
		t.Fatal("old binding overwrote a literal replacement")
	}
}
