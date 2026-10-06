package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/protocol"
)

const (
	DefaultExpRate      = 1.0
	MinExpRate          = 0.01
	MaxExpRate          = 1000.0
	expRatePrecision    = 100
	maxScaledExperience = math.MaxInt32
)

type RuntimeSettings struct {
	Name       string  `json:"server_name"`
	MOTD       string  `json:"motd"`
	ExpRate    float64 `json:"exp_rate"`
	DropRate   float64 `json:"drop_rate"`
	StatusMode string  `json:"status_mode"`
	LogLevel   string  `json:"log_level"`
}

func validExpRate(rate float64) bool {
	return !math.IsNaN(rate) && !math.IsInf(rate, 0) && rate >= MinExpRate && rate <= MaxExpRate
}

// ScaleExperience matches ServerStatusManager.ScaleExperience: midpoint-even
// rounding, one EXP minimum for positive rewards and a signed native EXP cap.
func ScaleExperience(amount uint64, rate float64) uint64 {
	if amount == 0 {
		return 0
	}
	if !validExpRate(rate) {
		rate = DefaultExpRate
	}
	return uint64(min(float64(maxScaledExperience), max(1, math.RoundToEven(float64(amount)*rate))))
}

func (s *Server) runtimeSettingsLocked() RuntimeSettings {
	exp := s.expRateMultiplier
	if exp == 0 {
		exp = DefaultExpRate
	}
	drop := s.dropRateMultiplier
	if drop == 0 {
		drop = battle.DefaultDropRateMultiplier
	}
	mode := s.statusMode
	if mode == "" {
		mode = "auto"
	}
	return RuntimeSettings{Name: s.Name(), MOTD: s.motd, ExpRate: exp, DropRate: drop, StatusMode: mode, LogLevel: s.logs.Level()}
}
func (s *Server) RuntimeSettings() RuntimeSettings {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	return s.runtimeSettingsLocked()
}
func (s *Server) UpdateRuntimeSettings(ctx context.Context, v RuntimeSettings) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	return s.updateRuntimeSettingsLocked(ctx, v)
}
func (s *Server) updateRuntimeSettingsLocked(ctx context.Context, v RuntimeSettings) error {
	if len(v.MOTD) > math.MaxUint8 {
		return errors.New("MOTD exceeds the native 255-byte popup limit")
	}
	if !validExpRate(v.ExpRate) || math.IsNaN(v.DropRate) || math.IsInf(v.DropRate, 0) || v.DropRate < battle.MinDropRateMultiplier || v.DropRate > battle.MaxDropRateMultiplier {
		return errors.New("invalid EXP or drop multiplier")
	}
	if v.StatusMode != "auto" && v.StatusMode != "green" && v.StatusMode != "yellow" && v.StatusMode != "red" && v.StatusMode != "offline" {
		return errors.New("invalid status mode")
	}
	if !validLogLevel(v.LogLevel) {
		return errors.New("invalid log level")
	}
	v.LogLevel = normalizedLogLevel(v.LogLevel)
	v.ExpRate = math.RoundToEven(v.ExpRate*expRatePrecision) / expRatePrecision
	settings := map[string]string{"server_name": v.Name, "motd": v.MOTD, "exp_rate": strconv.FormatFloat(v.ExpRate, 'f', 2, 64), "drop_rate": strconv.FormatFloat(v.DropRate, 'f', -1, 64), "status_mode": v.StatusMode, "log_level": v.LogLevel}
	if err := s.Store.SaveSettings(ctx, settings); err != nil {
		return err
	}
	s.applyRuntimeSettings(v)
	return nil
}
func (s *Server) applyRuntimeSettings(v RuntimeSettings) {
	s.SetName(v.Name)
	s.motd = v.MOTD
	s.expRateMultiplier = v.ExpRate
	s.dropRateMultiplier = v.DropRate
	s.statusMode = v.StatusMode
	s.logs.SetLevel(v.LogLevel)
}
func (s *Server) LoadRuntimeSettings(ctx context.Context) error {
	v, err := s.Store.Settings(ctx)
	if err != nil {
		return err
	}
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	current := s.runtimeSettingsLocked()
	if name := v["server_name"]; name != "" {
		current.Name = name
	}
	current.MOTD = v["motd"]
	if len(current.MOTD) > math.MaxUint8 {
		return errors.New("persisted MOTD exceeds the native 255-byte popup limit")
	}
	for key, target := range map[string]*float64{"exp_rate": &current.ExpRate, "drop_rate": &current.DropRate} {
		if raw := v[key]; raw != "" {
			value, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return fmt.Errorf("invalid persisted %s", key)
			}
			*target = value
		}
	}
	if mode := v["status_mode"]; mode != "" {
		current.StatusMode = mode
	}
	if level := v["log_level"]; level != "" {
		current.LogLevel = level
	}
	if !validExpRate(current.ExpRate) || math.IsNaN(current.DropRate) || math.IsInf(current.DropRate, 0) || current.DropRate < battle.MinDropRateMultiplier || current.DropRate > battle.MaxDropRateMultiplier || !validLogLevel(current.LogLevel) {
		return errors.New("invalid persisted runtime settings")
	}
	switch current.StatusMode {
	case "auto", "green", "yellow", "red", "offline":
	default:
		return errors.New("invalid persisted status mode")
	}
	s.applyRuntimeSettings(current)
	return nil
}
func (s *Server) gmExpRate(ctx context.Context, c *Session, _ string, args []string) error {
	current := s.runtimeSettingsLocked()
	if len(args) == 0 {
		return s.chatFeedback(c, fmt.Sprintf("Current EXP rate: %.2fx. Usage: /exprate <0.01–1000>", current.ExpRate))
	}
	if len(args) != 1 {
		return s.chatFeedback(c, "Usage: /exprate <0.01–1000>")
	}
	value, err := strconv.ParseFloat(args[0], 64)
	if err != nil || !validExpRate(value) {
		return s.chatFeedback(c, "EXP rate must be a finite number from 0.01 to 1000.")
	}
	current.ExpRate = value
	if err = s.updateRuntimeSettingsLocked(ctx, current); err != nil {
		return err
	}
	return s.chatFeedback(c, fmt.Sprintf("Global EXP rate saved: %.2fx.", s.expRateMultiplier))
}
func (s *Server) launcherStatusPacket(online int) []byte {
	packet := StatusPacket(online, s.Config.StatusServerIDs...)
	s.worldMu.Lock()
	mode := s.statusMode
	s.worldMu.Unlock()
	colors := map[string]byte{"offline": protocol.StatusLoadOffline, "green": protocol.StatusLoadGreen, "yellow": protocol.StatusLoadYellow, "red": protocol.StatusLoadRed}
	if color, ok := colors[mode]; ok {
		const firstStatusColorOffset = 5
		const statusEntryBytes = 3
		for at := firstStatusColorOffset; at < len(packet); at += statusEntryBytes {
			packet[at] = color
		}
	}
	return packet
}

func normalizedLogLevel(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

// Patch identity settings under the same lock as EXP commands. Reading and then
// saving an entire snapshot could otherwise undo a concurrent rate update.
func (s *Server) UpdateIdentitySettings(ctx context.Context, fields map[string]string) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	current := s.runtimeSettingsLocked()
	for key, value := range fields {
		switch key {
		case "server_name":
			current.Name = value
		case "motd":
			current.MOTD = value
		default:
			return errors.New("unknown setting")
		}
	}
	return s.updateRuntimeSettingsLocked(ctx, current)
}
