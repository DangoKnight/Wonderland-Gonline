package assets

import (
	"encoding/json"
	"fmt"
	"math"
)

// Optional authored SQL rows, in reference QuestManager registration order.
// Original source defines these lists but populates none in QuestDataBase.
const QuestVisibilityAsset = "quest_visibility.json"
const questVisibilityActorAction = 2

type QuestActors struct {
	Spawn   []uint16 `json:"spawn_npc_click_ids,omitempty"`
	Despawn []uint16 `json:"despawn_npc_click_ids,omitempty"`
}
type QuestVisibilityStep struct {
	Index int `json:"step"`
	QuestActors
}
type QuestVisibility struct {
	ID  uint32 `json:"quest_id"`
	Map uint16 `json:"map_id"`
	QuestActors
	Steps []QuestVisibilityStep `json:"steps,omitempty"`
}

func ParseQuestVisibility(raw []byte, c *Catalog) ([]QuestVisibility, error) {
	var definitions []QuestVisibility
	if err := json.Unmarshal(raw, &definitions); err != nil {
		return nil, err
	}
	seen := map[uint32]bool{}
	for _, q := range definitions {
		m, ok := c.Maps[q.Map]
		if !ok || q.ID == 0 || seen[q.ID] {
			return nil, fmt.Errorf("invalid quest visibility identity %d/map %d", q.ID, q.Map)
		}
		seen[q.ID] = true
		// PreEvents can refer to actors omitted from the NPC list. Those references
		// are valid scene click IDs too; retain their authored namespace.
		actors := map[uint16]bool{}
		for _, n := range m.NPCs {
			actors[n.ClickID] = true
		}
		for _, ev := range m.Events {
			actors[ev.ClickID] = true
		}
		for _, ev := range m.PreEvents {
			actors[ev.ClickID] = true
			for _, br := range ev.Branches {
				for _, op := range br.Operations {
					if op.Data[0] == questVisibilityActorAction {
						actors[le.Uint16(op.Data[1:])] = true
					}
				}
			}
		}
		validate := func(a QuestActors) error {
			for _, list := range [][]uint16{a.Spawn, a.Despawn} {
				seen := map[uint16]bool{}
				for _, id := range list {
					if id == 0 || !actors[id] || seen[id] {
						return fmt.Errorf("quest %d references invalid or duplicate actor %d", q.ID, id)
					}
					seen[id] = true
				}
			}
			// Both lists are allowed: the reference checks despawn first when active.
			return nil
		}
		if err := validate(q.QuestActors); err != nil {
			return nil, err
		}
		steps := map[int]bool{}
		for _, step := range q.Steps {
			if step.Index < 1 || step.Index > math.MaxUint8 || steps[step.Index] {
				return nil, fmt.Errorf("invalid quest %d visibility step %d", q.ID, step.Index)
			}
			steps[step.Index] = true
			if err := validate(step.QuestActors); err != nil {
				return nil, err
			}
		}
	}
	return definitions, nil
}
