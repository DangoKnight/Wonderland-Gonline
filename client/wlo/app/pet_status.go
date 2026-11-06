package app

import (
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
)

func (c *Client) initPetStatus() {
	c.MainStatus.Pet.Read = func() (hud.PetInfo, bool) {
		if c.World == nil {
			return hud.PetInfo{}, false
		}
		id := c.TeamState.BattlePet
		if id == 0 {
			if companion := c.World.Companions[c.World.Player.ID]; companion != nil {
				id = uint16(companion.ID)
			}
		}
		if id == 0 {
			return hud.PetInfo{}, false
		}
		for _, pet := range c.InventoryState.Pets {
			if pet.ID == id {
				stats := pet.Stats
				stats.Formula = c.Stats.Formula
				if c.battle.state.Active {
					for _, raw := range c.battle.state.Fighters {
						if raw.Kind == clientbattle.Pet && raw.Owner == c.World.Player.ID && game.SamePet(raw.ID, uint32(id)) {
							f := c.battle.displayed(raw)
							stats.HP, stats.MaxHP = f.HP, f.MaxHP
							stats.SP, stats.MaxSP = f.SP, f.MaxSP
							break
						}
					}
				}
				return hud.PetInfo{ID: id, Name: pet.Name, Stats: stats}, true
			}
		}
		return hud.PetInfo{}, false
	}
	portraits := map[uint16]*role.NPC{}
	c.MainStatus.Pet.DrawPortrait = func(id uint16, x, y int) {
		if c.lib == nil {
			return
		}
		if c.npcTemplates == nil {
			c.npcTemplates, _ = world.NPCTemplates(c.Assets)
		}
		t, ok := c.npcTemplates[uint32(id)]
		if !ok {
			return
		}
		portrait := portraits[id]
		if portrait == nil {
			portrait = role.NewNPC(c.lib, t.Look, t.Colors)
			portraits[id] = portrait
		}
		portrait.DrawIconAtAnchor(c.Screen, x, y, t.Icon)
	}
}
