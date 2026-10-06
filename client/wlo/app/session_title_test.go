package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wonderland-gonline/client/wlo/login"
)

func TestSessionTitleEditingAndProfile(t *testing.T) {
	w := &Workspace{Sessions: []*Session{{ID: 7, Client: &Client{}}}, nextID: 8}
	if w.Sessions[0].DisplayTitle() != "Session 7" {
		t.Fatal("wrong default title")
	}
	r := w.titleRect(0)
	w.panelClick(r.Min.X+3, r.Min.Y+3)
	if w.titleEditing != 7 {
		t.Fatal("title click did not open editor")
	}
	for _, r := range "Fishing α" {
		w.titleChar(r)
	}
	w.titleKey(titleKeyBackspace)
	w.titleChar('β')
	w.titleKey(titleKeyEnter)
	if w.Sessions[0].Title != "Fishing β" {
		t.Fatal(w.Sessions[0].Title)
	}
	w.beginTitle(0)
	w.titleChar('X')
	w.titleKey(titleKeyEscape)
	if w.Sessions[0].Title != "Fishing β" {
		t.Fatal("escape saved changes")
	}
	w.beginTitle(0)
	w.titleKey(titleKeyBackspace)
	w.titleKey(titleKeyEnter)
	if w.Sessions[0].DisplayTitle() != "Session 7" {
		t.Fatal("empty title did not restore default")
	}
	w.beginTitle(0)
	for i := 0; i < sessionTitleMaximumRunes+3; i++ {
		w.titleChar('a')
	}
	w.titleKey(titleKeyEnter)
	if len(w.Sessions[0].Title) != sessionTitleMaximumRunes {
		t.Fatal("title limit")
	}
	// Profiles store optional titles alongside account/server metadata.
	w.Sessions[0].Client.Servers = &login.SelectServer{}
	p := w.snapshotProfile()
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "workspace.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := readWorkspaceProfile(path)
	if err != nil || restored.Sessions[0].Title != strings.Repeat("a", sessionTitleMaximumRunes) {
		t.Fatal("title profile roundtrip", err)
	}
	if validSessionTitle("name\nother") || validSessionTitle(strings.Repeat("x", sessionTitleMaximumRunes+1)) {
		t.Fatal("invalid title accepted")
	}
}
