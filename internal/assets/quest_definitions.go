package assets

import (
	"fmt"
	"math"
	"wonderland-go/internal/game"
)

// QuestType is the reference custom registry type, distinct from EVE opcodes.
type QuestType byte

const (
	QuestDialogue       QuestType = 0
	QuestItemCollection QuestType = 1
	QuestMonsterBattle  QuestType = 2
	QuestDelivery       QuestType = 3
	QuestExploration    QuestType = 4
)
const MaxQuestSteps = math.MaxUint8

type QuestItem struct {
	ItemID uint16 `json:"item_id"`
	Count  int    `json:"count"`
	Name   string `json:"name,omitempty"`
}
type QuestReward struct {
	Gold          uint32      `json:"gold"`
	EXP           uint32      `json:"exp"`
	Items         []QuestItem `json:"items"`
	CompanionID   uint32      `json:"companion_id"`
	CompanionName string      `json:"companion_name"`
}
type QuestStep struct {
	Index               int         `json:"index"`
	TargetNPCTemplateID uint32      `json:"target_npc_template_id"`
	TargetNPCPattern    string      `json:"target_npc_pattern"`
	Type                QuestType   `json:"type"`
	PromptDialogue      string      `json:"prompt_dialogue"`
	InProgressDialogue  string      `json:"in_progress_dialogue"`
	CompleteDialogue    string      `json:"complete_dialogue"`
	RequiredItems       []QuestItem `json:"required_items"`
	GrantItems          []QuestItem `json:"grant_items"`
	Reward              QuestReward `json:"reward"`
	BattleMonsterID     uint32      `json:"battle_monster_id"`
	BattleMonsterName   string      `json:"battle_monster_name"`
	RequiredKillCount   int         `json:"required_kill_count"`
	SpawnNPCClickIDs    []uint16    `json:"spawn_npc_click_ids"`
	DespawnNPCClickIDs  []uint16    `json:"despawn_npc_click_ids"`
}
type QuestDefinition struct {
	ID                       uint32      `json:"id"`
	MapID                    uint16      `json:"map_id"`
	Title                    string      `json:"title"`
	Description              string      `json:"description"`
	Type                     QuestType   `json:"type"`
	RequiredLevel            uint16      `json:"required_level"`
	NPCTemplateID            uint32      `json:"npc_template_id"`
	NPCNamePattern           string      `json:"npc_name_pattern"`
	Category                 string      `json:"category"`
	AreaName                 string      `json:"area_name"`
	InProgressMarkID         uint32      `json:"in_progress_mark_id"`
	CompletedMarkID          uint32      `json:"completed_mark_id"`
	AllLinkedMarkIDs         []uint32    `json:"all_linked_mark_ids"`
	RequiredItems            []QuestItem `json:"required_items"`
	Reward                   QuestReward `json:"reward"`
	IntroDialogue            string      `json:"intro_dialogue"`
	InProgressDialogue       string      `json:"in_progress_dialogue"`
	CompleteDialogue         string      `json:"complete_dialogue"`
	AlreadyCompletedDialogue string      `json:"already_completed_dialogue"`
	BattleMonsterID          uint32      `json:"battle_monster_id"`
	BattleMonsterName        string      `json:"battle_monster_name"`
	RequiredKillCount        int         `json:"required_kill_count"`
	Repeatable               bool        `json:"repeatable"`
	Daily                    bool        `json:"daily"`
	CooldownMinutes          int         `json:"cooldown_minutes"`
	PrerequisiteQuestIDs     []uint32    `json:"prerequisite_quest_ids"`
	DespawnNPCClickIDs       []uint16    `json:"despawn_npc_click_ids"`
	SpawnNPCClickIDs         []uint16    `json:"spawn_npc_click_ids"`
	DespawnNPCTemplateIDs    []uint32    `json:"despawn_npc_template_ids"`
	SpawnNPCTemplateIDs      []uint32    `json:"spawn_npc_template_ids"`
	RelocateToMapID          uint16      `json:"relocate_to_map_id"`
	Steps                    []QuestStep `json:"steps"`
}

// ValidateQuestDefinitions validates authored registry data without inventing
// progression or rewards from translated Mark.dat titles.
func ValidateQuestDefinitions(c *Catalog) error {
	npc := func(id uint32) bool { _, ok := c.NPCs[uint16(id)]; return id == 0 || (id <= math.MaxUint16 && ok) }
	items := func(rows []QuestItem) error {
		for _, row := range rows {
			if _, ok := c.Items[row.ItemID]; !ok || row.ItemID == 0 || row.Count <= 0 || row.Count > math.MaxInt32 {
				return fmt.Errorf("invalid quest item %d", row.ItemID)
			}
		}
		return nil
	}
	reward := func(v QuestReward) error {
		if v.Gold > game.MaxGold || !npc(game.BroadcastID(v.CompanionID)) {
			return fmt.Errorf("invalid quest reward")
		}
		return items(v.Items)
	}
	for id, q := range c.QuestDefinitions {
		if id == 0 || id != q.ID || q.Title == "" || q.Type > QuestExploration || q.RequiredLevel > game.MaxLevel || q.RequiredKillCount < 0 || q.RequiredKillCount > math.MaxInt32 || q.CooldownMinutes < 0 || q.CooldownMinutes > math.MaxInt32 || len(q.Steps) > MaxQuestSteps || !npc(q.NPCTemplateID) || !npc(q.BattleMonsterID) {
			return fmt.Errorf("invalid quest definition %d", id)
		}
		if q.MapID != 0 {
			if _, ok := c.Maps[q.MapID]; !ok {
				return fmt.Errorf("quest %d has unknown map", id)
			}
		}
		if q.RelocateToMapID != 0 {
			if _, ok := c.Maps[q.RelocateToMapID]; !ok {
				return fmt.Errorf("quest %d has unknown relocation", id)
			}
		}
		actorIDs := func(ids []uint16) error {
			seen := map[uint16]bool{}
			for _, click := range ids {
				found := false
				for _, n := range c.Maps[q.MapID].NPCs {
					if n.ClickID == click {
						found = true
						break
					}
				}
				if !found || seen[click] {
					return fmt.Errorf("quest %d has invalid actor %d", id, click)
				}
				seen[click] = true
			}
			return nil
		}
		if err := items(q.RequiredItems); err != nil {
			return err
		}
		if err := reward(q.Reward); err != nil {
			return err
		}
		for _, ids := range [][]uint16{q.SpawnNPCClickIDs, q.DespawnNPCClickIDs} {
			if err := actorIDs(ids); err != nil {
				return err
			}
		}
		for _, ids := range [][]uint32{q.SpawnNPCTemplateIDs, q.DespawnNPCTemplateIDs} {
			for _, tid := range ids {
				if tid == 0 || !npc(tid) {
					return fmt.Errorf("invalid quest actor template")
				}
			}
		}
		for _, mark := range append(append([]uint32{}, q.AllLinkedMarkIDs...), q.InProgressMarkID, q.CompletedMarkID) {
			if mark != 0 {
				if _, ok := c.Marks[uint16(mark)]; mark > math.MaxUint16 || !ok {
					return fmt.Errorf("quest %d has unknown mark %d", id, mark)
				}
			}
		}
		seen := map[uint32]bool{}
		for _, pre := range q.PrerequisiteQuestIDs {
			_, custom := c.QuestDefinitions[pre]
			_, native := c.Marks[uint16(pre)]
			if pre == 0 || pre == id || seen[pre] || (!custom && !(pre <= math.MaxUint16 && native)) {
				return fmt.Errorf("invalid prerequisite for quest %d", id)
			}
			seen[pre] = true
		}
		for i, step := range q.Steps {
			if step.Index != i+1 || step.Type > QuestExploration || step.RequiredKillCount < 0 || step.RequiredKillCount > math.MaxInt32 || !npc(step.TargetNPCTemplateID) || !npc(step.BattleMonsterID) {
				return fmt.Errorf("invalid step in quest %d", id)
			}
			if err := items(step.RequiredItems); err != nil {
				return err
			}
			if err := items(step.GrantItems); err != nil {
				return err
			}
			if err := reward(step.Reward); err != nil {
				return err
			}
			for _, ids := range [][]uint16{step.SpawnNPCClickIDs, step.DespawnNPCClickIDs} {
				if err := actorIDs(ids); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
