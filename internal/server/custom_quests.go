package server

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/battle"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// customQuestKills runs on the settlement clone. Persistence and all receipts
// share the normal victory transaction; captures and PvP never count.
func (s *Server) customQuestKills(next *game.Character, run *battleRun) [][]byte {
	if run.b.PvP {
		return nil
	}
	ids := make([]uint32, 0, len(next.Quests))
	for id := range next.Quests {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var packets [][]byte
	for _, monster := range run.b.Defenders {
		if monster.Kind != battle.Monster || monster.Captured || monster.HP > 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(monster.Name))
		for _, id := range ids {
			q := next.Quests[id]
			d, ok := s.Assets.QuestDefinitions[id]
			if !ok || q.State != game.InProgress {
				continue
			}
			kind, tid, pattern, required := d.Type, d.BattleMonsterID, d.BattleMonsterName, d.RequiredKillCount
			if q.Step > 0 && q.Step <= len(d.Steps) {
				step := d.Steps[q.Step-1]
				kind, tid, pattern, required = step.Type, step.BattleMonsterID, step.BattleMonsterName, step.RequiredKillCount
			}
			if kind != assets.QuestMonsterBattle {
				continue
			}
			pattern = strings.ToLower(strings.TrimSpace(pattern))
			matched := tid > 0 && tid == monster.Template || (name != "" && pattern != "" && (strings.Contains(name, pattern) || strings.Contains(pattern, name))) || (tid == 0 && pattern == "")
			if !matched || q.Kills >= math.MaxInt32 {
				continue
			}
			q.Kills++
			next.Quests[id] = q
			required = max(1, required)
			text := fmt.Sprintf("[Quest] %s: %d/%d defeated.", d.Title, q.Kills, required)
			packets = append(packets, questProgressMessage(text))
			if q.Kills >= required {
				packets = append(packets, questProgressMessage("[Quest Objective Completed] "+d.Title))
			}
		}
	}
	return packets
}
func questProgressMessage(text string) []byte {
	const maxQuestMessageBytes = math.MaxUint8
	// This diagnostic mirrors AC23:57; it does not accept, advance or reward quests.
	raw := []byte(text)
	if len(raw) > maxQuestMessageBytes {
		raw = raw[:maxQuestMessageBytes]
		for !utf8.Valid(raw) {
			raw = raw[:len(raw)-1]
		}
	}
	p, _ := protocol.Builder{protocol.CommandInventory, protocol.InventoryMessage, 0}.String(string(raw))
	return p
}
