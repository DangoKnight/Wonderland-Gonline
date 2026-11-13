package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type controllerProfile struct {
	Name        string `json:"name"`
	Preferences string `json:"preferences"`
	Accounts    []int  `json:"accounts"`
}

func configureProfileAccounts(p *controllerProfile, directory, selection string, count int) error {
	if selection != "" {
		p.Accounts = nil
		for _, entry := range strings.Split(selection, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(entry))
			if err != nil {
				return errors.New("accounts must be comma-separated account numbers")
			}
			p.Accounts = append(p.Accounts, n)
		}
	}
	if len(p.Accounts) == 0 {
		return errors.New("profile has no allowed accounts; set --accounts 1,2 or edit its profile.json accounts array")
	}
	seen := map[int]bool{}
	for _, n := range p.Accounts {
		if n < 1 || n > count || seen[n] {
			return errors.New("profile accounts must be unique numbers present in .secrets")
		}
		seen[n] = true
	}
	if selection != "" {
		// The profile lock excludes concurrent readers/writers by controllers.
		path := filepath.Join(directory, "profile.json")
		data, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return err
		}
		temp := path + ".tmp"
		if err := os.WriteFile(temp, append(data, '\n'), 0600); err != nil {
			return err
		}
		if err := os.Rename(temp, path); err != nil {
			_ = os.Remove(temp)
			return err
		}
	}
	return nil
}

const (
	maxProfileBytes   = 16 * 1024
	maxProfileIDBytes = 64
)

// Profiles are local controller configuration, not server gameplay state.
func loadProfile(root, id string) (controllerProfile, string, error) {
	id = strings.ToLower(id)
	if id == "" || len(id) > maxProfileIDBytes || strings.Trim(id, "abcdefghijklmnopqrstuvwxyz0123456789-_") != "" {
		return controllerProfile{}, "", errors.New("profile must contain 1–64 ASCII letters, digits, hyphens or underscores")
	}
	directory := filepath.Join(root, "agent", "profiles", id)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return controllerProfile{}, "", err
	}
	p := controllerProfile{Name: id, Preferences: "Explore and develop your own interests as an ordinary player."}
	switch id {
	case "aria":
		p = controllerProfile{Name: "Aria", Preferences: "You are Aria. You enjoy finding fun things to do: exploration, stories, unusual activities, discovering systems and variety. Follow your curiosity and avoid repetitive grinding unless it supports an interesting goal."}
	case "meteo":
		p = controllerProfile{Name: "Meteo", Preferences: "You are Meteo. You enjoy gaining levels, improving stats and equipment, learning efficient combat, and building strong teams. Set progression goals, compare opportunities, and balance productive training with recovery and unlocking useful systems."}
	}
	path := filepath.Join(directory, "profile.json")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		err = json.NewEncoder(f).Encode(p)
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
	} else if errors.Is(err, os.ErrExist) {
		err = nil
	}
	if err != nil {
		return controllerProfile{}, "", err
	}
	f, err = os.Open(path)
	if err != nil {
		return controllerProfile{}, "", err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, maxProfileBytes+1))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return controllerProfile{}, "", fmt.Errorf("read profile: %w", err)
	}
	if d.Decode(new(any)) != io.EOF || strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Preferences) == "" || len(p.Name)+len(p.Preferences) > maxProfileBytes {
		return controllerProfile{}, "", errors.New("profile requires a name and preferences within 16 KiB")
	}
	return p, directory, nil
}

// OS locks survive neither a closed handle nor a crashed controller. Empty lock
// files stay in place: unlinking them could create two independently locked files.
type reservations struct {
	directory string
	held      map[string]*os.File
}

var errReserved = errors.New("reserved by another controller")

func reservationKey(kind, identity string) string {
	sum := sha256.Sum256([]byte(kind + ":" + strings.ToLower(identity)))
	return kind + "-" + hex.EncodeToString(sum[:])
}

func (r *reservations) acquire(key string) error {
	if r.held[key] != nil {
		return nil
	}
	if err := os.MkdirAll(r.directory, 0700); err != nil {
		return err
	}
	f, err := lockReservation(filepath.Join(r.directory, key+".lock"))
	if err != nil {
		return err
	}
	if r.held == nil {
		r.held = map[string]*os.File{}
	}
	r.held[key] = f
	return nil
}

func (r *reservations) release(key string) {
	if f := r.held[key]; f != nil {
		_ = f.Close()
		delete(r.held, key)
	}
}

func (r *reservations) close() {
	for key := range r.held {
		r.release(key)
	}
}

func (r *reservations) available(key string) (bool, error) {
	if r.held[key] != nil {
		return true, nil
	}
	if err := r.acquire(key); err != nil {
		if errors.Is(err, errReserved) {
			return false, nil
		}
		return false, err
	}
	r.release(key)
	return true, nil
}
