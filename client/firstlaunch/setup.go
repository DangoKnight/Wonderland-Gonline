// Package firstlaunch provides an asset-independent native client setup window.
package firstlaunch

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/localization"
)

const stateVersion = 1
const stateMaximumBytes = 16 << 10
const terrainProbeMap = 10017 // Ship Deck: present in every supported WLRI installation.

var ErrCancelled = errors.New("client setup cancelled")

type State struct {
	Selected bool   `json:"-"` // This invocation completed the language prompt.
	Version  int    `json:"version"`
	Language string `json:"language"`
	Assets   string `json:"assets,omitempty"`
}

type Options struct {
	Assets, Language      string
	ExplicitAssets, Force bool
}

type Request struct {
	StatePath  string `json:"state_path"`
	Assets     string `json:"assets"`
	Language   string `json:"language"`
	Reason     string `json:"reason,omitempty"`
	NeedAssets bool   `json:"need_assets"`
}

func statePath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "Wonderland-Gonline", "setup.json"), nil
}

func readState(path string) (State, error) {
	var s State
	raw, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if len(raw) > stateMaximumBytes {
		return s, errors.New("setup preferences are too large")
	}
	if err = json.Unmarshal(raw, &s); err != nil {
		return s, err
	}
	if s.Version != stateVersion || (s.Language != localization.English && s.Language != localization.Chinese) {
		return s, errors.New("invalid setup preferences")
	}
	return s, nil
}

func saveState(path string, s State) error {
	s.Version = stateVersion
	if s.Language != localization.English && s.Language != localization.Chinese {
		return errors.New("select a language")
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".setup-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

// ValidateAssets catches the incomplete installation that can otherwise reach
// login with working UI but no terrain. Mount also verifies companion packs.
func ValidateAssets(path string) error {
	root := path
	if strings.EqualFold(filepath.Ext(path), ".zip") {
		var closePack func() error
		var err error
		root, closePack, err = clientfs.Mount(path)
		if err != nil {
			return err
		}
		defer closePack()
	}
	scene, err := world.LoadScene(login.NewAssets(root), terrainProbeMap)
	if err != nil {
		return err
	}
	if scene.Width <= 0 || scene.Height <= 0 || len(scene.Layers) == 0 {
		return errors.New("Ship Deck terrain/background is incomplete")
	}
	return nil
}

func requestFor(path string, o Options) (Request, bool) {
	s, err := readState(path)
	if err != nil {
		s = State{}
	}
	r := Request{StatePath: path, Assets: o.Assets, Language: s.Language}
	if !o.ExplicitAssets && s.Assets != "" {
		r.Assets = s.Assets
	}
	if absolute, e := filepath.Abs(r.Assets); e == nil {
		r.Assets = absolute
	}
	if o.Language != "" {
		r.Language, _ = localization.Normalize(o.Language)
	}
	if r.Language == "" {
		r.Language = localization.English
	}
	if assetErr := ValidateAssets(r.Assets); assetErr != nil {
		r.NeedAssets, r.Reason = true, assetErr.Error()
	}
	return r, o.Force || err != nil || r.NeedAssets
}

// Prepare runs setup in a short-lived child process because Ebitengine allows
// only one RunGame per process. The parent then starts the game normally, so no
// manual restart is needed. Extraction/tool and snapshot modes bypass this.
func Prepare(o Options) (State, error) {
	if _, err := localization.Normalize(o.Language); err != nil {
		return State{}, err
	}
	path, err := statePath()
	if err != nil {
		return State{}, err
	}
	r, needed := requestFor(path, o)
	if !needed {
		return State{Version: stateVersion, Language: r.Language, Assets: r.Assets}, nil
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return State{}, err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".setup-request-*.json")
	if err != nil {
		return State{}, err
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(r)
	closeErr := f.Close()
	if err != nil {
		return State{}, err
	}
	if closeErr != nil {
		return State{}, closeErr
	}
	exe, err := os.Executable()
	if err != nil {
		return State{}, err
	}
	cmd := exec.Command(exe, "-setup-window", f.Name())
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err = cmd.Run(); err != nil {
		var status *exec.ExitError
		if errors.As(err, &status) && status.ExitCode() == CancelledExitCode {
			return State{}, ErrCancelled
		}
		return State{}, fmt.Errorf("setup window: %w", err)
	}
	s, err := readState(path)
	s.Selected = err == nil
	return s, err
}

const CancelledExitCode = 2 // Helper cancellation, distinguished from setup errors.

// RunWindow is used only by the startup helper, before opening game assets.
func RunWindow(requestPath string) error {
	raw, err := os.ReadFile(requestPath)
	if err != nil {
		return err
	}
	var r Request
	if len(raw) > stateMaximumBytes {
		return errors.New("setup request is too large")
	}
	if err = json.Unmarshal(raw, &r); err != nil {
		return err
	}
	return runWindow(r)
}
