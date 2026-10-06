package world

import (
	"slices"
	"wonderland-gonline/internal/game"
)

// Match reference registration/list order. A spawn list owns visibility even
// before activation; a despawn list only overrides visibility when active.
func (w *World) questVisibility(c *game.Character, mapID, click uint16) (bool, bool) {
	for _, q := range w.catalog.QuestVisibility {
		if q.Map != mapID {
			continue
		}
		state := c.Quests[q.ID]
		completed := state.State == game.Completed
		if completed && slices.Contains(q.Despawn, click) {
			return false, true
		}
		if slices.Contains(q.Spawn, click) {
			return completed, true
		}
		for _, step := range q.Steps {
			active := state.State == game.InProgress && state.Step == step.Index
			if active && slices.Contains(step.Despawn, click) {
				return false, true
			}
			if slices.Contains(step.Spawn, click) {
				return active, true
			}
		}
	}
	return false, false
}
