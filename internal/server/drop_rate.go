package server

import (
	"fmt"
	"math"
	"strconv"
	"wonderland-go/internal/battle"
)

// gmDropRate ports AC02's process-local loot multiplier. The caller holds worldMu.
// It applies to subsequent victory loot rolls, including battles already in progress.
func (s *Server) gmDropRate(c *Session, arguments []string) error {
	if len(arguments) == 0 {
		multiplier := s.dropRateMultiplier
		if multiplier == 0 {
			multiplier = battle.DefaultDropRateMultiplier
		}
		return s.chatFeedback(c, fmt.Sprintf("Current drop rate multiplier: %.1fx. Usage: /droprate <multiplier>", multiplier))
	}
	if len(arguments) != 1 {
		return s.chatFeedback(c, "Usage: /droprate <multiplier> (0.1–100)")
	}
	multiplier, err := strconv.ParseFloat(arguments[0], 64)
	if err != nil || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return s.chatFeedback(c, "Drop rate multiplier must be a finite number.")
	}
	s.dropRateMultiplier = min(battle.MaxDropRateMultiplier, max(battle.MinDropRateMultiplier, multiplier))
	return s.chatFeedback(c, fmt.Sprintf("Global drop rate multiplier set to %.1fx!", s.dropRateMultiplier))
}
