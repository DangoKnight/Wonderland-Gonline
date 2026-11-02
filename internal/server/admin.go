package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
)

// Account changes exclude in-flight authentication so an old credential cannot
// publish a session after the change and escape the disconnect.
func (s *Server) ResetAccountPassword(ctx context.Context, id uint32, password string) error {
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	if err := s.Store.ResetPassword(ctx, id, password); err != nil {
		return err
	}
	s.KickAccount(id)
	return nil
}

func (s *Server) DeleteAccount(ctx context.Context, id uint32) error {
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	characters, err := s.Store.Characters(ctx, id)
	if err != nil {
		return err
	}
	type relation struct{ owner, friend uint32 }
	var relations []relation
	for _, c := range characters {
		friends, err := s.Store.Friends(ctx, c.ID)
		if err != nil {
			return err
		}
		for _, f := range friends {
			relations = append(relations, relation{c.ID, f.ID})
		}
	}

	if err := s.Store.DeleteAccount(ctx, id); err != nil {
		return err
	}
	s.KickAccount(id)
	for _, r := range relations {
		if peer := s.friendPlayer(r.friend); peer != nil {
			delete(peer.friendRequests, r.owner)
			s.sendOrClose(peer, protocol.Builder{protocol.CommandFriends, protocol.FriendsRemove}.U32(r.owner))
			if err := s.sendFriendList(context.Background(), peer); err != nil {
				s.Log.Error("friend deletion refresh failed", "error", err)
			}
		}
	}
	return nil
}

// AdminAnnouncement keeps UI color choices tied to native chat channels.
func (s *Server) AdminAnnouncement(text, channel string) error {
	codes := map[string]byte{"notice": protocol.ChatGMMessage, "world": protocol.ChatWorldMessage, "guild": protocol.ChatGuildMessage, "whisper": protocol.ChatWhisperMessage, "banner": protocol.ChatHeadBanner}
	if channel == "" {
		channel = "notice"
	}
	code, ok := codes[channel]
	if !ok {
		return errors.New("invalid announcement channel")
	}
	lines := strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' || r == '|' })
	if len(lines) == 0 {
		return errors.New("empty announcement")
	}
	packets := make([][]byte, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !validChatText(line, chatCommandMaxBytes) {
			return errors.New("invalid announcement")
		}
		if code == protocol.ChatHeadBanner {
			packets = append(packets, serverwire.HeadBanner(line))
		} else {
			packets = append(packets, protocol.Builder{protocol.CommandChat, code}.U32(0).Bytes([]byte(line)))
		}
	}
	if len(packets) == 0 {
		return errors.New("empty announcement")
	}
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	s.mu.Lock()
	recipients := make([]*Session, 0, len(s.sessions))
	for _, c := range s.sessions {
		if c.info.CharacterID != 0 {
			recipients = append(recipients, c)
		}
	}
	s.mu.Unlock()
	for _, c := range recipients {
		for _, p := range packets {
			if err := c.send(p); err != nil {
				c.conn.Close()
				break
			}
		}
	}
	return nil
}

type StartupConfiguration struct {
	Engine          string        `json:"engine"`
	Path            string        `json:"path"`
	Version         string        `json:"version"`
	Configuration   config.Config `json:"configuration"`
	Running         config.Config `json:"running"`
	RestartRequired bool          `json:"restart_required"`
}

func (s *Server) SetConfigurationPath(path string) { s.configPath = path }
func (s *Server) startupConfigurationLocked() (StartupConfiguration, error) {
	path := s.configPath
	if path == "" {
		path = "config.admin.json"
	}
	raw, err := os.ReadFile(path)
	version := "missing"
	next := s.Config
	if err == nil {
		version = assetdb.DocumentVersion(raw)
		next, err = config.Load(path)
		if err != nil {
			return StartupConfiguration{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return StartupConfiguration{}, err
	}
	return StartupConfiguration{Engine: "SQLite", Path: path, Version: version, Configuration: next, Running: s.Config, RestartRequired: true}, nil
}
func (s *Server) StartupConfiguration() (StartupConfiguration, error) {
	s.adminEditMu.Lock()
	defer s.adminEditMu.Unlock()
	return s.startupConfigurationLocked()
}
func (s *Server) SaveStartupConfiguration(version string, next config.Config) error {
	s.adminEditMu.Lock()
	defer s.adminEditMu.Unlock()
	if err := config.Validate(next); err != nil {
		return err
	}
	current, err := s.startupConfigurationLocked()
	if err != nil {
		return err
	}
	if version != current.Version {
		return errors.New("configuration changed; refresh before saving")
	}
	raw, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	file, err := os.CreateTemp(filepath.Dir(current.Path), ".wonderland-config-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(raw); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, current.Path); err != nil {
		return err
	}
	s.configPath = current.Path
	s.Log.Info("startup configuration saved", "path", current.Path, "restart_required", true)
	return nil
}

// The caller holds worldMu and a catalog lock, excluding initial map snapshots.
func (s *Server) adminCharacterLoading(id uint32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.sessions {
		if c.info.CharacterID == id && s.friendSessions[id] != c {
			return true
		}
	}
	return false
}

func (s *Server) validateAdminState(c game.Character) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Element < game.Earth || c.Element > game.Wind || c.Gold > game.MaxGold || len(c.Pets) > game.MaxPets {
		return errors.New("invalid element, gold or pet count")
	}
	validateItems := func(items []game.Item, equipped bool) error {
		for _, item := range items {
			if item.ID == 0 && item.Count == 0 {
				continue
			}
			limit, known := s.stackLimit(item.ID)
			if !known || item.Count == 0 || item.Count > limit || (equipped && item.Count != 1) {
				return fmt.Errorf("invalid item or stack %d", item.ID)
			}
		}
		return nil
	}
	for _, group := range []struct {
		items    []game.Item
		equipped bool
	}{{c.Bag[:], false}, {c.Storage[:], false}, {c.Equipment[:], true}} {
		if err := validateItems(group.items, group.equipped); err != nil {
			return err
		}
	}
	for _, inventory := range []game.Inventory{c.Bag, c.Storage} {
		if _, err := inventory.Occupancy(s.Assets.Items); err != nil {
			return fmt.Errorf("invalid inventory footprint: %w", err)
		}
	}
	seenSkills := map[uint16]bool{}
	for _, skill := range c.Skills {
		if _, known := s.Assets.Skills[skill.ID]; !known || seenSkills[skill.ID] || skill.Grade < game.MinSkillGrade || skill.Grade > game.SkillGradeLimit(skill.ID) {
			return errors.New("invalid or duplicate skill")
		}
		seenSkills[skill.ID] = true
	}
	seenPets := map[uint32]bool{}
	active := c.ActivePet == 0
	mount := c.ActiveMount == 0
	for groupIndex, pets := range [][]game.Pet{c.Pets, c.ReservePets, c.HotelPets} {
		for _, pet := range pets {
			if pet.ID == 0 || pet.Level < 1 || pet.Level > game.MaxLevel || pet.HP < 0 || pet.HP > pet.MaxHP || pet.SP < 0 || pet.SP > pet.MaxSP || seenPets[game.BroadcastID(pet.ID)] {
				return errors.New("invalid or duplicate pet")
			}
			seenPets[game.BroadcastID(pet.ID)] = true
			if err := validateItems(pet.Equipment[:], true); err != nil {
				return err
			}
			if groupIndex == 0 {
				active = active || game.SamePet(c.ActivePet, pet.ID)
				mount = mount || game.SamePet(c.ActiveMount, pet.ID)
			}
		}
	}
	if !active || !mount {
		return errors.New("active pet or mount is not in the party")
	}
	if c.RecordPoint != nil {
		if _, ok := s.Assets.Maps[c.RecordPoint.Map]; !ok {
			return errors.New("unknown record map")
		}
	}
	for key, expiry := range c.ChestRespawns {
		if key == 0 || expiry.IsZero() {
			return errors.New("invalid chest cooldown")
		}
	}
	if c.ActiveVehicle != 0 && (c.VehicleSlot < 1 || int(c.VehicleSlot) > len(c.Bag) || c.Bag[c.VehicleSlot-1].ID != c.ActiveVehicle) {
		return errors.New("vehicle is not in the selected bag slot")
	}
	return nil
}

func (s *Server) AdminKickAll() {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	s.mu.Lock()
	targets := make([]*Session, 0, len(s.sessions))
	for _, c := range s.sessions {
		targets = append(targets, c)
	}
	s.mu.Unlock()
	for _, c := range targets {
		s.gmKick(c, "Server maintenance.")
	}
}
