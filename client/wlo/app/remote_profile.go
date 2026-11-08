package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/internal/localization"
)

const remoteProfileVersion = 1

type remoteProfile struct {
	Version int                     `json:"version"`
	Options inventory.RemoteOptions `json:"options"`
}

func (c *Client) remoteProfilePath() string {
	if c.World == nil {
		return ""
	}
	name := fmt.Sprintf("remote-%d.json", c.World.Player.ID)
	if c.settingsPath == "" {
		return c.Assets.UserPath(name)
	}
	return filepath.Join(filepath.Dir(c.settingsPath), name)
}

// Native FUN_001fe79c/FUN_001fed8c store/load a local User profile keyed by
// character ID. Persist confirmed configuration only, never running automation,
// elapsed time, pending requests, death counts or per-battle sequence cursors.
func saveRemoteProfile(path string, options inventory.RemoteOptions) error {
	if !options.Valid() {
		return fmt.Errorf("invalid Remote profile settings")
	}
	raw, err := json.MarshalIndent(remoteProfile{remoteProfileVersion, options}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".remote-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func readRemoteProfile(path string) (inventory.RemoteOptions, bool, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return inventory.RemoteOptions{}, false, nil
	}
	if err != nil {
		return inventory.RemoteOptions{}, false, err
	}
	var profile remoteProfile
	if err = json.Unmarshal(raw, &profile); err != nil {
		return inventory.RemoteOptions{}, false, err
	}
	if profile.Version != remoteProfileVersion || !profile.Options.Valid() {
		return inventory.RemoteOptions{}, false, fmt.Errorf("unsupported or invalid Remote profile")
	}
	return profile.Options, true, nil
}

func (c *Client) loadRemoteProfile() {
	if c.World == nil {
		return
	}
	o, found, err := readRemoteProfile(c.remoteProfilePath())
	if err != nil {
		c.Chat.Notice(localization.UI("ui.remote.load_failed") + err.Error())
		return
	}
	if found {
		c.remote.options = o
		c.Inventory.RestoreRemoteOptions(o)
	}
}
