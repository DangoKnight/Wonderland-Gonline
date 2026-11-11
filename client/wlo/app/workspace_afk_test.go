package app

import (
	"image/png"
	"os"
	"testing"
	"wonderland-gonline/internal/protocol"
)

func TestWorkspaceAFKControlsTargetSessionWithoutSwitching(t *testing.T) {
	first, _, _ := enteredClient(t)
	_ = wire(t, first)
	second, _, _ := enteredClient(t)
	_ = wire(t, second)
	w := NewWorkspace(first)
	defer w.Close()
	second.afkWorkspace = w
	w.Sessions = append(w.Sessions, &Session{ID: 2, Client: second})
	for _, c := range []*Client{first, second} {
		c.connection = connectionRuntime{enabled: true, features: protocol.ConnectionAFK}
		c.mapReady = true
		c.stopMapLoading()
	}
	r := w.afkRect(1)
	if r.Overlaps(w.titleRect(1)) || r.Overlaps(w.closeRect(1)) {
		t.Fatal("preview controls overlap")
	}
	if !w.Pointer(r.Min.X+5, r.Min.Y+5, true, true, 0) {
		t.Fatal("sidebar click leaked to scene")
	}
	if !second.afk.pending || first.afk.pending || w.Active != 0 || w.PendingRemove != 0 || w.titleEditing != 0 {
		t.Fatal("wrong session toggled or selection changed")
	}
	second.afkPacket([]byte{253, 1, 1})
	if path := os.Getenv("AFK_WORKSPACE_SNAPSHOT"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, w.Compose().RGBA())
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	w.Pointer(r.Min.X+5, r.Min.Y+5, false, false, 0)
	w.Pointer(r.Min.X+5, r.Min.Y+5, true, true, 0)
	if !second.afk.pending || !second.afk.active || w.Active != 0 {
		t.Fatal("AFK preview cannot request return")
	}
	if first.afkClick(330, 10) {
		t.Fatal("old scene button still captures input")
	}
	first.connection = connectionRuntime{}
	w.panelClick(w.afkRect(0).Min.X+5, w.afkRect(0).Min.Y+5)
	if first.afk.pending {
		t.Fatal("unsupported session accepted AFK")
	}
}
