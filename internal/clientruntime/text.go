package clientruntime

import "path/filepath"

// TextDirectory holds generated loose text assets, never translation tables.
const TextDirectory = "runtime/text"

func TextPath(name string) (string, bool) {
	switch name {
	case "npc_data.json", "item_data.json", "skill_data.json", "scene_data.json", "talk_data.json", "mark_data.json":
		return filepath.Join(filepath.FromSlash(TextDirectory), name), true
	}
	return "", false
}
