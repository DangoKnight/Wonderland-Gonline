package seui

import (
	"testing"

	"wonderland-go/client/wlo/picdb"
	"wonderland-go/client/wlo/surface"
)

func testEnv() *Env {
	env := &Env{Pics: picdb.New(), Screen: surface.New(800, 600)}
	NewManager(env, &Input{})
	return env
}

func typeText(e *Editor, s string) {
	for _, b := range []byte(s) {
		e.Char(b)
	}
}

func TestEditorAccountField(t *testing.T) {
	e := NewEditor(testEnv(), nil)
	e.Name = "IDField"
	e.Init("", 0, 0, 0, 0, 0, false, 12, 0x68, 0)
	e.SetMaxLen(10)
	typeText(e, "abc\tdefghijklm")
	if string(e.Text) != "ABCDEFGHIJ" {
		t.Fatalf("got %q", e.Text)
	}
	e.KeyDown(VKHome, 0)
	e.KeyDown(VKDelete, 0)
	e.KeyDown(VKEnd, 0)
	e.KeyDown(VKBack, 0)
	if string(e.Text) != "BCDEFGHI" || e.Caret != 8 {
		t.Fatalf("got %q caret %d", e.Text, e.Caret)
	}
}

func TestEditorDoubleByte(t *testing.T) {
	e := NewEditor(testEnv(), nil)
	e.Init("", 0, 0, 0, 0, 0, false, 12, 200, 0)
	typeText(e, "a\xa4\x40b")
	if string(e.Text) != "a\xa4\x40b" || e.Caret != 4 {
		t.Fatalf("got % x caret %d", e.Text, e.Caret)
	}
	e.KeyDown(VKLeft, 0)
	e.KeyDown(VKBack, 0)
	if string(e.Text) != "ab" || e.Caret != 1 {
		t.Fatalf("a double-byte character should go at once: % x caret %d", e.Text, e.Caret)
	}
	// Overwrite mode replaces the character after the caret.
	e.KeyDown(VKInsert, 0)
	typeText(e, "z")
	if string(e.Text) != "az" {
		t.Fatalf("got %q", e.Text)
	}
}

func TestEditorFilters(t *testing.T) {
	e := NewEditor(testEnv(), nil)
	e.Init("", 0, 0, 0, 0, 0, false, 12, 200, 0)
	e.SetFilter(1)
	typeText(e, "0")
	typeText(e, "x7")
	if string(e.Text) != "7" {
		t.Fatalf("a digit replaces a lone zero: got %q", e.Text)
	}
	e.Clear()
	e.SetFilter(0)
	e.Name = "CreateRoleName"
	typeText(e, "a b")
	if string(e.Text) != "ab" {
		t.Fatalf("got %q", e.Text)
	}
}

func TestComboAddItem(t *testing.T) {
	c := NewComboBox(testEnv(), nil)
	c.Init("", 0, 0, 0, 0, 0, false, 12, 100, 0)
	c.Setup("", 0, 0, 0, "", "", 0)
	for _, s := range []string{"A", "B", "C", "D", "E", "F", "C"} {
		c.AddItem([]byte(s))
	}
	want := []string{"C", "F", "E", "D", "B"}
	for i, w := range want {
		if string(c.List.Items[i]) != w {
			t.Fatalf("items %q", c.List.Items)
		}
	}
}
