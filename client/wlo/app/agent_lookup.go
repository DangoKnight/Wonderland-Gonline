package app

import (
	"fmt"
	"wonderland-gonline/client/wlo/world"
)

// AgentLookup reads client definitions only; it cannot inspect credentials,
// another character's durable state, or hidden minigame outcomes.
func (c *Client) AgentLookup(kind string, id uint32) (any, error) {
	if err := c.AgentAccessError(); err != nil {
		return nil, err
	}
	switch kind {
	case "item":
		if id > 0xffff {
			return nil, fmt.Errorf("unknown item")
		}
		if item, ok := c.items[uint16(id)]; ok {
			return item.Definition, nil
		}
	case "skill":
		if id <= 0xffff && c.SkillState != nil && c.SkillState.Catalog != nil {
			if skill, ok := c.SkillState.Catalog.Definitions[uint16(id)]; ok {
				return skill, nil
			}
		}
	case "npc":
		if c.npcTemplates == nil {
			c.npcTemplates, _ = world.NPCTemplates(c.Assets)
		}
		if npc, ok := c.npcTemplates[id]; ok {
			return npc, nil
		}
	case "quest":
		for _, entry := range c.AgentQuests() {
			if uint32(entry.ID) == id {
				return entry, nil
			}
		}
	default:
		return nil, fmt.Errorf("unknown catalog kind")
	}
	return nil, fmt.Errorf("catalog entry unavailable")
}
