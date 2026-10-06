package server

import (
	"context"
	"math"
	"strconv"

	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// GM progression changes don't mutate snapshots owned by combat, events,
// loading or trades. Caller holds worldMu and has checked administrator rights.
func (s *Server) gmProgress(ctx context.Context, c *Session, name string, words []string) error {
	if !commandTravelAvailable(c) || c.trade != nil {
		return nil
	}
	next := c.character.Clone()
	var packets [][]byte
	refill := false
	switch name {
	case "level", "lvl":
		if len(words) < 2 {
			return nil
		}
		level, err := strconv.ParseUint(words[1], 10, 8)
		if err != nil {
			return nil
		}
		next.SetLevel(byte(level))
		refill = true
		packets = append(packets, next.ExpPacket())
	case "points", "sp", "statpoint", "statpoints":
		if len(words) < 2 {
			return nil
		}
		points, err := strconv.ParseUint(words[1], 10, 16)
		if err != nil {
			return nil
		}
		next.StatPoints = uint16(min(uint64(next.StatPoints)+points, math.MaxUint16))
		packets = append(packets, protocol.Builder{protocol.CommandStats, protocol.StatsStatUpdate, game.StatUnallocatedPoints, protocol.StatsValueAbsolute}.U32(uint32(next.StatPoints)).U32(0))
	case "stats", "stat":
		if len(words) < 6 {
			return nil
		}
		var values [5]uint16
		for i := range values {
			v, err := strconv.ParseUint(words[i+1], 10, 16)
			if err != nil {
				return nil
			}
			values[i] = uint16(v)
		}
		next.Base = game.Attributes{Strength: values[0], Constitution: values[1], Intelligence: values[2], Wisdom: values[3], Agility: values[4]}
		refill = true
	case "exp":
		if len(words) < 2 {
			return nil
		}
		exp, err := strconv.ParseInt(words[1], 10, 64)
		if err != nil || exp > math.MaxUint32 {
			return nil
		}
		next.EXP = uint32(max(exp, 0))
		next.Level = next.LevelFromEXP(uint64(next.EXP))
		// Direct assignment grants no points (AC02 sets TotalExp, not AddExp).
		packets = append(packets, next.ExpPacket())
	case "skill":
		if len(words) < 2 {
			return nil
		}
		id, err := strconv.ParseUint(words[1], 10, 16)
		if err != nil || !s.hasSkill(uint16(id)) {
			return nil
		}
		grade := uint64(game.MinSkillGrade)
		if len(words) >= 3 {
			grade, err = strconv.ParseUint(words[2], 10, 8)
			if err != nil {
				return nil
			}
		}
		if grade < game.MinSkillGrade || grade > uint64(game.SkillGradeLimit(uint16(id))) {
			return nil
		}
		packets = next.SetSkillGrade(uint16(id), byte(grade))
	default:
		return nil
	}
	if name == "level" || name == "lvl" || name == "stat" || name == "stats" || name == "exp" {
		if refill {
			next.Refill(s.Assets.Items)
		} else {
			full := next.Combat(s.Assets.Items)
			next.MaxHP, next.MaxSP = uint32(max(full.MaxHP, 1)), uint32(max(full.MaxSP, 0))
			next.HP, next.SP = min(next.HP, next.MaxHP), min(next.SP, next.MaxSP)
		}
		packets = append(packets, next.StatPackets(s.Assets.Items)...)
	}
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	return s.sendAll(c, packets)
}
