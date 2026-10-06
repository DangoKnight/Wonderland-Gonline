package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"wonderland-gonline/client/wlo/login"
)

const workspaceProfileVersion = 1
const workspaceProfileMaximumBytes = 1 << 20
const workspaceProfileMaximumSessions = 256

type sessionProfile struct {
	ID      int                    `json:"id"`
	Title   string                 `json:"title,omitempty"`
	Account *string                `json:"account"`
	Server  *login.ServerSelection `json:"server"`
}
type workspaceProfile struct {
	Version       int              `json:"version"`
	Sessions      []sessionProfile `json:"sessions"`
	ActiveSession int              `json:"active_session"`
	NextSessionID int              `json:"next_session_id"`
	Collapsed     bool             `json:"collapsed"`
}

func (c *Client) rememberProfileAccount() {
	if login.ValidAccount([]byte(c.Login.SubmittedAccount)) {
		c.profileAccount = c.Login.SubmittedAccount
	}
}
func (c *Client) clearProfileAccount() {
	c.profileAccount = ""
	if c.Login != nil {
		c.Login.SubmittedAccount = ""
	}
}
func (w *Workspace) ProfilePath() string {
	base := w.options.UserRoot
	if w.options.SettingsPath != "" {
		base = filepath.Dir(w.options.SettingsPath)
	}
	return filepath.Join(base, "workspace.json")
}
func (w *Workspace) snapshotProfile() workspaceProfile {
	p := workspaceProfile{Version: workspaceProfileVersion, NextSessionID: w.nextID, Collapsed: w.Collapsed}
	for i, s := range w.Sessions {
		entry := sessionProfile{ID: s.ID, Title: s.Title, Server: s.Client.Servers.LastSelection}
		if s.Client.profileAccount != "" {
			name := s.Client.profileAccount
			entry.Account = &name
		}
		p.Sessions = append(p.Sessions, entry)
		if i == w.Active {
			p.ActiveSession = s.ID
		}
	}
	// Closing the last instance leaves a fresh login available on the next launch.
	if len(p.Sessions) == 0 {
		p.Sessions = []sessionProfile{{ID: 1}}
		p.ActiveSession = 1
	}
	return p
}
func (w *Workspace) SaveProfile() error {
	if !w.profileEnabled {
		return nil
	}
	data, err := json.MarshalIndent(w.snapshotProfile(), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if bytes.Equal(data, w.profileLast) {
		return nil
	}
	path := w.ProfilePath()
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".workspace-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	w.profileLast = data
	return nil
}
func readWorkspaceProfile(path string) (workspaceProfile, error) {
	var p workspaceProfile
	f, err := os.Open(path)
	if err != nil {
		return p, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return p, err
	}
	if info.Size() > workspaceProfileMaximumBytes {
		return p, fmt.Errorf("workspace profile exceeds size limit")
	}
	d := json.NewDecoder(io.LimitReader(f, workspaceProfileMaximumBytes+1))
	if err = d.Decode(&p); err != nil {
		return p, err
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return p, fmt.Errorf("workspace profile has trailing data")
	}
	if p.Version != workspaceProfileVersion || len(p.Sessions) == 0 || len(p.Sessions) > workspaceProfileMaximumSessions {
		return p, fmt.Errorf("unsupported workspace profile version or session count")
	}
	if p.NextSessionID < 0 || p.NextSessionID >= int(^uint(0)>>1)-1 {
		return p, fmt.Errorf("invalid next workspace session ID")
	}
	ids := map[int]bool{}
	for _, s := range p.Sessions {
		if s.ID <= 0 || s.ID >= int(^uint(0)>>1)-1 || ids[s.ID] {
			return p, fmt.Errorf("invalid workspace session ID")
		}
		ids[s.ID] = true
		if !validSessionTitle(s.Title) {
			return p, fmt.Errorf("invalid session title")
		}
		if s.Account != nil && !login.ValidAccount([]byte(*s.Account)) {
			return p, fmt.Errorf("invalid workspace account name")
		}
	}
	if !ids[p.ActiveSession] {
		return p, fmt.Errorf("invalid active workspace session")
	}
	return p, nil
}

// RestoreProfile restores only login screens. It never submits saved credentials.
// An invalid file is left intact and disables persistence for this run.
func (w *Workspace) RestoreProfile() error {
	p, err := readWorkspaceProfile(w.ProfilePath())
	if os.IsNotExist(err) {
		w.profileEnabled = true
		return nil
	}
	if err != nil {
		return err
	}
	first := w.Sessions[0].Client
	restored := make([]*Session, 0, len(p.Sessions))
	for _, entry := range p.Sessions {
		c := first
		if entry.ID != 1 {
			c, err = w.factory(w.sessionOptions(entry.ID))
			if err != nil {
				for _, s := range restored {
					if s.Client != first {
						s.Client.closeSession()
					}
				}
				return err
			}
		}
		restored = append(restored, &Session{ID: entry.ID, Title: entry.Title, Client: c})
	}
	reusedFirst := false
	w.Active = 0
	for i, entry := range p.Sessions {
		c := restored[i].Client
		reusedFirst = reusedFirst || c == first
		if entry.Server != nil {
			saved := *entry.Server
			if !c.Servers.RestoreSelection(saved) {
				c.Servers.LastSelection = &saved
			}
		}
		if entry.Account != nil {
			c.profileAccount = *entry.Account
			c.Login.Account.SetText([]byte(*entry.Account))
		}
		c.backgroundSession = entry.ID != p.ActiveSession
		if entry.ID == p.ActiveSession {
			w.Active = i
		}
		p.NextSessionID = max(p.NextSessionID, entry.ID+1)
	}
	if !reusedFirst {
		first.closeSession()
	}
	w.Sessions = restored
	w.nextID = p.NextSessionID
	w.Collapsed = p.Collapsed
	if w.Collapsed {
		w.slide = sessionSlideTicks
	}
	w.setScroll(min(w.Active, w.maxScroll()))
	w.profileEnabled = true
	return w.SaveProfile()
}
