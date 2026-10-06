package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"wonderland-gonline/internal/clientassets"
)

func TestSettingsSnapshotAndValidation(t *testing.T) {
	s := NewState()
	if !s.Apply([]byte{33, 2, 2, 1, 2, 2, 18, 0}) || !s.Synced || s.Permissions.PKAllowed || !s.Permissions.JoinAllowed || s.Permissions.TradeAllowed || !s.Permissions.PartyInvitesBlocked || s.Permissions.Channels != 18 {
		t.Fatal(s)
	}
	before := s.Permissions
	for _, p := range [][]byte{{33, 2}, {33, 2, 1}, {33, 2, 0, 1, 1, 1, 31, 0}, {33, 2, 1, 1, 1, 1, 255, 0}} {
		if s.Apply(p) || s.Permissions != before {
			t.Fatal("invalid reply changed permissions", p)
		}
	}
	s.Reset()
	if s.Synced || !s.Permissions.PKAllowed {
		t.Fatal("permissions retained across characters")
	}
}
func TestSettingsLocalRoundTripAndBig5Blacklist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user", "settings.json")
	l, err := Load(path)
	if err != nil || !reflect.DeepEqual(l, Defaults()) {
		t.Fatal(l, err)
	}
	l.Blacklist = []string{"Tester", "小明"}
	l.SoundOn = false
	l.Info[InfoOtherNames] = false
	l.Colors[0] = 3
	if err = l.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || !reflect.DeepEqual(got, l) || !got.Blocked([]byte("TESTER")) || !got.Blocked(clientassets.Big5Text("小明")) || got.Blocked([]byte("Other")) {
		t.Fatal(got, err)
	}
	if err = os.WriteFile(path, []byte(`{"music_volume":99,"sound_volume":-4,"chat_colors":[-1,100,0,0,0]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = Load(path)
	if err != nil || got.MusicVolume != 10 || got.SoundVolume != 0 || got.Colors[0] != len(Palette)-1 || !got.Info[InfoOtherNames] {
		t.Fatal(got, err)
	}
	if err = os.WriteFile(path, []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(path); err == nil {
		t.Fatal("corrupt preferences accepted")
	}
}
