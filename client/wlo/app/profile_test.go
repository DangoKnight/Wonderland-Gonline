package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/client/wlo/login"
)

func TestWorkspaceProfileRestoresSessionsWithoutAuthentication(t *testing.T) {
	first := testClient(t)
	w := NewWorkspace(first)
	base := t.TempDir()
	w.options.SettingsPath = filepath.Join(base, "settings.json")
	w.factory = func(o Options) (*Client, error) {
		c, err := New(o)
		if err == nil {
			c.Net.Dial = first.Net.Dial
		}
		return c, err
	}
	if err := w.RestoreProfile(); err != nil {
		t.Fatal(err)
	}
	if err := w.Add(); err != nil {
		t.Fatal(err)
	}
	if err := w.Add(); err != nil {
		t.Fatal(err)
	}
	w.Sessions[0].Title = "Fishing setup"
	first.Servers.LastSelection = &login.ServerSelection{Host: "127.0.0.1", Region: 1, Index: 0}
	first.Login.SubmittedAccount = "Tester01"
	first.rememberProfileAccount()
	w.Sessions[1].Client.Login.Account.SetText([]byte("unlogged"))
	w.Current().Login.Password.SetText([]byte("secret123"))
	w.Remove(2)
	w.Collapsed = true
	if err := w.SaveProfile(); err != nil {
		t.Fatal(err)
	}
	path := w.ProfilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("secret123")) || bytes.Contains(data, []byte("unlogged")) {
		t.Fatal("profile contains credentials or unauthenticated account")
	}
	p, err := readWorkspaceProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Sessions) != 2 || p.Sessions[1].Account != nil || p.Sessions[0].Account == nil || *p.Sessions[0].Account != "Tester01" {
		t.Fatalf("incorrect profile: %+v", p)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("profile permissions: %v", info.Mode())
	}
	if err := w.SaveProfile(); err != nil {
		t.Fatal(err)
	}
	again, _ := os.Stat(path)
	if !again.ModTime().Equal(info.ModTime()) {
		t.Fatal("unchanged profile rewritten")
	}
	restored := NewWorkspace(testClient(t))
	restored.options.SettingsPath = w.options.SettingsPath
	restored.factory = w.factory
	defer restored.Close()
	if err := restored.RestoreProfile(); err != nil {
		t.Fatal(err)
	}
	if len(restored.Sessions) != 2 || restored.Sessions[1].ID != 3 || restored.nextID != 4 || restored.Active != 1 || !restored.Collapsed {
		t.Fatal("session layout not restored")
	}
	if restored.Sessions[0].Title != "Fishing setup" {
		t.Fatal("custom title not restored")
	}
	c := restored.Sessions[0].Client
	if string(c.Login.Account.Text) != "Tester01" || len(c.Login.Password.Text) != 0 || c.Login.SubmittedAccount != "" || c.G.InGame {
		t.Fatal("restoration authenticated or lost account prefill")
	}
	c.Login.ServerDescription(append([]byte{9, 101, 0, 1}, []byte("Wonderland Gonline Server")...))
	if !c.Login.Visible || string(c.Login.Account.Text) != "Tester01" || len(c.Login.Password.Text) != 0 {
		t.Fatal("restored login screen missing or contains a password")
	}
	if c.Servers.LastSelection == nil || c.Servers.LastSelection.Host != "127.0.0.1" {
		t.Fatal("server not restored")
	}
	if restored.Sessions[1].Client.Assets.UserPath("settings.json") == c.Assets.UserPath("settings.json") || restored.Sessions[1].Client.lib != c.lib {
		t.Fatal("restored sessions lost isolation/shared assets")
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceProfileRejectsInvalidFileWithoutOverwriting(t *testing.T) {
	c := testClient(t)
	w := NewWorkspace(c)
	defer w.Close()
	w.options.SettingsPath = filepath.Join(t.TempDir(), "settings.json")
	raw := []byte(`{"version":99,"sessions":[]}`)
	if err := os.WriteFile(w.ProfilePath(), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := w.RestoreProfile(); err == nil {
		t.Fatal("invalid profile accepted")
	}
	if err := w.SaveProfile(); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(w.ProfilePath())
	if !bytes.Equal(raw, saved) {
		t.Fatal("invalid profile overwritten")
	}
	for _, raw := range []string{
		`{"version":1,"active_session":1,"sessions":[{"id":1},{"id":1}]}`,
		`{"version":1,"active_session":2,"sessions":[{"id":1}]}`,
		`{"version":1,"active_session":1,"sessions":[{"id":1,"account":"ab"}]}`,
		`{"version":1,"active_session":1,"sessions":[{"id":1}]} {}`,
	} {
		if err := os.WriteFile(w.ProfilePath(), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readWorkspaceProfile(w.ProfilePath()); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestWorkspaceProfileAuthenticationAndLogout(t *testing.T) {
	c := testClient(t)
	c.Login.Account.SetText([]byte("Tester01"))
	c.Login.Password.SetText([]byte("secret123"))
	c.Login.Submit()
	if c.profileAccount != "" {
		t.Fatal("login submission treated as authentication")
	}
	c.roster([]byte{1})
	if c.profileAccount != "Tester01" {
		t.Fatal("successful login not recorded")
	}
	c.Login.Previous()
	if c.profileAccount != "" || c.Login.SubmittedAccount != "" {
		t.Fatal("logout retained account")
	}
	c.Login.SubmittedAccount = "Tester02"
	c.wrongPassword()
	c.rememberProfileAccount()
	if c.profileAccount != "" {
		t.Fatal("failed login remembered")
	}
}

func TestWorkspaceProfileJSONExplicitEmptyAccount(t *testing.T) {
	p := workspaceProfile{Version: 1, Sessions: []sessionProfile{{ID: 1}}}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"account":null`)) {
		t.Fatal("empty account not explicit")
	}
}

func TestXaolanWindowIcon(t *testing.T) {
	c := testClient(t)
	icon, err := c.lib.Image(xaolanPortraitFamily, xaolanPortraitID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if icon.Bounds().Dx() != 24 || icon.Bounds().Dy() != 24 {
		t.Fatalf("small portrait bounds: %v", icon.Bounds())
	}
	opaque, transparent := false, false
	for y := icon.Bounds().Min.Y; y < icon.Bounds().Max.Y; y++ {
		for x := icon.Bounds().Min.X; x < icon.Bounds().Max.X; x++ {
			a := icon.NRGBAAt(x, y).A
			opaque = opaque || a != 0
			transparent = transparent || a == 0
		}
	}
	if !opaque || !transparent {
		t.Fatal("icon lost artwork or transparency")
	}
}

func TestWorkspaceProfileRestoresAfterRemovingFirstSession(t *testing.T) {
	first := testClient(t)
	w := NewWorkspace(first)
	w.options.SettingsPath = filepath.Join(t.TempDir(), "settings.json")
	w.factory = func(o Options) (*Client, error) {
		c, err := New(o)
		if err == nil {
			c.Net.Dial = first.Net.Dial
		}
		return c, err
	}
	if err := w.RestoreProfile(); err != nil {
		t.Fatal(err)
	}
	if err := w.Add(); err != nil {
		t.Fatal(err)
	}
	w.Remove(1)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r := NewWorkspace(testClient(t))
	r.options.SettingsPath = w.options.SettingsPath
	r.factory = w.factory
	defer r.Close()
	if err := r.RestoreProfile(); err != nil {
		t.Fatal(err)
	}
	if len(r.Sessions) != 1 || r.Sessions[0].ID != 2 || r.nextID != 3 {
		t.Fatal("removed first session returned")
	}
	if filepath.Base(filepath.Dir(filepath.Dir(r.Current().Assets.UserPath("settings.json")))) != "session-2" {
		t.Fatal("session profile path changed")
	}
	if err := r.Add(); err != nil {
		t.Fatal(err)
	}
	if r.Current().lib != r.Sessions[0].Client.lib {
		t.Fatal("restored resources no longer shared")
	}
	r.Tick()
}

func TestWorkspaceProfileSelectionUsesCurrentServerList(t *testing.T) {
	c := testClient(t)
	saved := login.ServerSelection{Host: "127.0.0.1", Region: 70, Index: 9}
	if !c.Servers.RestoreSelection(saved) || c.Servers.LastSelection.Region != 1 || c.Servers.LastSelection.Index != 0 {
		t.Fatal("configured server moved but was not restored")
	}
	c.Servers.Connect(-1)
	if c.Servers.RestoreSelection(login.ServerSelection{Host: "unconfigured.example", Region: 1, Index: 0}) {
		t.Fatal("dialed server absent from current configuration")
	}
	if !c.Servers.Visible {
		t.Fatal("missing server did not fall back to selection")
	}
	c.Net.Shutdown()
}
