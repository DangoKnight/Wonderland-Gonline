package app

import (
	"image"
	"sort"
	"strconv"
	"time"

	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// Potential previews keep a seated pose; player and supported companion
// sprites use action 49 (FUN_001cd574). Never substitute a standing pose.
const potentialPreviewSeated = 49
const potentialPortraitOriginX = 16 // FUN_001cb9e8, relative to the roster cell.
const potentialPortraitOriginY = -3

func (c *Client) initPotentialPreview() {
	c.Inventory.PotentialSkillsAtRisk = c.potentialSkillsAtRisk
	c.Inventory.LoadPotentialPictures = func() {
		c.loadPictures("PotentialFire1", "PotentialFire2")
		for _, id := range []uint16{game.ItemPotentialPill, game.ItemGoldenPotentialPill, game.ItemSuperPotentialPill} {
			c.loadPictures(strconv.Itoa(int(c.items[id].Icon)))
		}
	}
	template := func(slot byte) world.NPCTemplate {
		if c.npcTemplates == nil {
			c.npcTemplates, _ = world.NPCTemplates(c.Assets)
		}
		return c.npcTemplates[game.BroadcastID(uint32(c.InventoryState.Pets[slot-1].ID))]
	}
	c.Inventory.DrawPotentialPortrait = func(slot byte, bounds image.Rectangle) {
		if slot == 0 {
			if human, ok := c.Inventory.Preview.(*role.Human); ok {
				human.DrawRosterPortrait(c.Screen, bounds.Min.X+potentialPortraitOriginX, bounds.Min.Y+potentialPortraitOriginY)
			}
			return
		}
		if portrait := c.resources.petPortrait(template(slot)); portrait != nil {
			bounds.Min.X += 2
			bounds.Min.Y++
			c.Screen.DrawStretch(bounds, portrait, true)
		}
	}
	previews := map[uint16]*role.NPC{}
	c.Inventory.DrawPotentialPreview = func(slot byte, x, y int) {
		if slot == 0 {
			if original, ok := c.Inventory.Preview.(*role.Human); ok {
				preview := *original
				preview.SetVehicle(0)
				preview.SetPetMount(nil)
				preview.Hold(0, false)
				preview.DrawBody(c.Screen, x, y, potentialPreviewSeated)
			}
			return
		}
		if c.lib == nil {
			return
		}
		id := c.InventoryState.Pets[slot-1].ID
		n := previews[id]
		if n == nil {
			t := template(slot)
			n = role.NewNPC(c.lib, t.Look, t.Colors)
			n.SetHeightScale(t.HeightScale)
			n.Now = func() time.Time { return c.Now() }
			n.Hold(0, false)
			previews[id] = n
		}
		if action, ok := potentialSittingAction(n); ok {
			n.Draw(c.Screen, x, y, action)
		}
	}
}

// Use the server's skill qualification rules on a copy with one potential level
// removed. Stats already include avatar bonuses; a zero body adds none twice.
func (c *Client) potentialSkillsAtRisk() []string {
	if c.SkillState == nil || c.Stats.Potential == 0 {
		return nil
	}
	s := c.Stats
	bonus := int(game.PotentialBonus(uint16(s.Potential)))
	base := func(v uint16) uint16 { return uint16(max(0, int(v)-bonus)) }
	character := game.Character{Element: s.Element, Potential: uint16(s.Potential) - 1,
		Base: game.Attributes{Strength: base(s.STR), Constitution: base(s.CON), Intelligence: base(s.INT), Wisdom: base(s.WIS), Agility: base(s.AGI)}}
	var ids []int
	for id, progress := range c.SkillState.Learned {
		if progress.Grade > 0 {
			ids = append(ids, int(id))
		}
	}
	sort.Ints(ids)
	for _, id := range ids {
		character.Skills = append(character.Skills, game.LearnedSkill{ID: uint16(id)})
	}
	if !character.ForgetUnqualifiedSkills() {
		return nil
	}
	kept := map[uint16]bool{}
	for _, sk := range character.Skills {
		kept[sk.ID] = true
	}
	var names []string
	for _, id := range ids {
		if kept[uint16(id)] {
			continue
		}
		name := "Skill " + strconv.Itoa(id)
		if c.Skills != nil && c.Skills.Catalog != nil {
			if def, ok := c.Skills.Catalog.Definitions[uint16(id)]; ok && def.Name != "" {
				name = def.Name
			}
		}
		names = append(names, name)
	}
	return names
}

func potentialSittingAction(n interface{ FrameSize(int) (int, int) }) (int, bool) {
	if w, h := n.FrameSize(potentialPreviewSeated); w > 0 && h > 0 {
		return potentialPreviewSeated, true
	}
	for action := protocol.PoseSitFirst; action <= protocol.PoseSitLast; action++ {
		if w, h := n.FrameSize(action); w > 0 && h > 0 {
			return action, true
		}
	}
	return 0, false
}
