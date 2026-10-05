// Package settings ports TSe_SystemForm and its channel/information dialogs.
package settings

import (
	"encoding/json"
	"errors"
	"golang.org/x/text/encoding/traditionalchinese"
	"os"
	"path/filepath"
	"strings"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

const (
	VolumeSteps   = 10
	DefaultVolume = 5
	ChannelCount  = 5
	InfoCount     = 10
)

const (
	InfoOwnPetName = iota // indexes follow TTH_HuminfoForm, FUN_002824bc
	InfoOtherPetName
	InfoOtherPets
	InfoOwnNickname
	InfoOwnGuild
	InfoOtherNames
	InfoOtherNicknames
	InfoOtherGuilds
	InfoFullTents
	InfoAdventureLevel
)

// Local preferences follow native user/save*.dat. Server permissions are never
// restored from this file: AC33:2 remains authoritative for each character.
type Local struct {
	MusicOn     bool              `json:"music_on"`
	SoundOn     bool              `json:"sound_on"`
	MusicVolume int               `json:"music_volume"`
	SoundVolume int               `json:"sound_volume"`
	Zoom        bool              `json:"zoom"`
	Info        [InfoCount]bool   `json:"info"`
	Colors      [ChannelCount]int `json:"chat_colors"`
	Blacklist   []string          `json:"blacklist"`
}

// Palette is the native chat text palette in RGB565; channel colors wrap.
var Palette = [...]uint16{0xfe31, 0xffb3, 0xc6f3, 0x8653, 0x6e7e, 0x8e27, 0xffff, 0xf800, 0xf4a3, 0xff80, 0xfc00}

func Defaults() Local {
	l := Local{MusicOn: true, SoundOn: true, MusicVolume: DefaultVolume, SoundVolume: DefaultVolume, Zoom: true, Colors: [ChannelCount]int{9, 10, 6, 4, 1}}
	for i := range l.Info {
		l.Info[i] = true
	}
	return l
}
func Load(path string) (Local, error) {
	l := Defaults()
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return l, err
	}
	if err = json.Unmarshal(raw, &l); err != nil {
		return Defaults(), err
	}
	l.MusicVolume = max(0, min(VolumeSteps, l.MusicVolume))
	l.SoundVolume = max(0, min(VolumeSteps, l.SoundVolume))
	for i, v := range l.Colors {
		l.Colors[i] = (v%len(Palette) + len(Palette)) % len(Palette)
	}
	return l, nil
}
func (l Local) Save(path string) error {
	raw, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}
func (l Local) Blocked(name []byte) bool {
	for _, n := range l.Blacklist {
		if strings.EqualFold(n, DecodeName(name)) {
			return true
		}
	}
	return false
}

// State holds the server snapshot separately from the local display preferences.
type State struct {
	Local       Local
	Permissions game.ClientSettings
	Synced      bool
}

func NewState() *State  { return &State{Local: Defaults(), Permissions: game.Character{}.Preferences()} }
func (s *State) Reset() { s.Permissions = game.Character{}.Preferences(); s.Synced = false }
func (s *State) Apply(p []byte) bool {
	if len(p) != 8 || p[0] != protocol.CommandSettings || p[1] != protocol.SettingsSnapshot || p[6]&^byte(game.AllChatChannels) != 0 {
		return false
	}
	for _, v := range p[2:6] {
		if v != game.SettingEnabled && v != game.SettingDisabled {
			return false
		}
	}
	s.Permissions = game.ClientSettings{PKAllowed: p[2] == game.SettingEnabled, JoinAllowed: p[3] == game.SettingEnabled, TradeAllowed: p[4] == game.SettingEnabled, PartyInvitesBlocked: p[5] == game.SettingDisabled, Channels: p[6]}
	s.Synced = true
	return true
}
func Flag(on bool) byte {
	if on {
		return game.SettingEnabled
	}
	return game.SettingDisabled
}

// DecodeName converts native Big5 names before storing UTF-8 preferences.
func DecodeName(name []byte) string {
	b, err := traditionalchinese.Big5.NewDecoder().Bytes(name)
	if err != nil {
		return string(name)
	}
	return string(b)
}
