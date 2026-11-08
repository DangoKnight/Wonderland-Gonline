package seui

import (
	"bytes"
	"testing"
)

func TestMemoCaretAndDelete(t *testing.T) {
	m := NewMemo(testEnv(), nil)
	m.SetBounds(0, 0, 200, 100)
	m.SetText([]byte{'A', 0xa4, 0x40, 'B'})
	m.KeyDown(VKLeft, 0)
	m.KeyDown(VKLeft, 0)
	if m.Caret != 1 {
		t.Fatal("navigation split Big5 character", m.Caret)
	}
	m.KeyDown(VKDelete, 0)
	if string(m.Text) != "AB" || m.Caret != 1 {
		t.Fatal("Delete did not remove the complete next character", m.Text)
	}
	m.Char('x')
	if string(m.Text) != "AxB" {
		t.Fatal("typing ignored caret", m.Text)
	}
	m.Char(VKBack)
	if string(m.Text) != "AB" {
		t.Fatal("Backspace removed wrong character", m.Text)
	}
	m.KeyDown(VKEnd, 0)
	m.KeyDown(VKDelete, 0)
	if string(m.Text) != "AB" {
		t.Fatal("Delete at end changed text")
	}
	m.ReadOnly = true
	m.KeyDown(VKHome, 0)
	m.KeyDown(VKDelete, 0)
	m.Char('x')
	if string(m.Text) != "AB" {
		t.Fatal("read-only memo was edited")
	}
}

func TestMemoWrappedLineNavigation(t *testing.T) {
	m := NewMemo(testEnv(), nil)
	m.SetBounds(0, 0, 40, 100) // Four bytes per wrapped line.
	m.SetText([]byte("abcde\rXY"))
	m.KeyDown(memoKeyUp, 0)
	if m.Caret != 5 {
		t.Fatal("Up did not preserve the bounded column", m.Caret)
	}
	m.KeyDown(VKHome, 0)
	if m.Caret != 4 {
		t.Fatal("Home did not reach wrapped line start", m.Caret)
	}
	m.KeyDown(VKDelete, 0)
	if !bytes.Equal(m.Text, []byte("abcd\rXY")) {
		t.Fatal(m.Text)
	}
}
