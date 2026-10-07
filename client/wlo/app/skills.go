package app

import (
	"encoding/binary"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const mainSkillsButton = 2

func (c *Client) initSkills() {
	catalog := c.resources.skillCatalog
	var err error
	if catalog == nil {
		catalog, err = skills.Load(c.Assets)
	}
	if err != nil {
		c.Chat.Notice("Could not load skills: " + err.Error())
		catalog = &skills.Catalog{Definitions: map[uint16]skills.Definition{}, Orders: map[uint16]uint16{}}
	}
	c.SkillState = skills.NewState(catalog)
	c.Skills = skills.NewForm(c.Env, catalog)
	c.Skills.Targets = c.skillTargets
	c.Skills.LoadPictures = c.loadPictures
	c.Skills.Notice = func(text string) { c.Notices.Show([]byte(text), inventoryWarningDuration, c.Now()) }
	c.UI.Add(c.Skills)
	c.MainButtons.Buttons[mainSkillsButton].OnClick = c.toggleSkills
	c.initHotbar()
}
func (c *Client) toggleSkills() {
	if c.Skills.Visible {
		c.Skills.Hide()
	} else if c.Inventory.CanAct() && c.UI.Modal == nil {
		c.Skills.Show()
	}
}
func (c *Client) SkillsKey(key uint16, shift byte) bool {
	if c.Skills == nil {
		return false
	}
	if key == 'S' && shift&shiftCtrl != 0 {
		c.toggleSkills()
		return true
	}
	if key == escapeKey && c.Skills.Visible && (c.UI.Active == c.Skills || c.Input.Focused != nil && c.Input.Focused.Base().Root() == c.Skills) {
		c.Skills.Hide()
		return true
	}
	return false
}
func (c *Client) skillTargets() []skills.Target {
	if c.World == nil {
		return nil
	}
	targets := []skills.Target{{Name: c.World.Player.Name, Element: c.Stats.Element, Skills: c.SkillState.Learned, Stunt: game.StarterStunt(uint16(c.World.Player.Body), uint16(c.World.Player.Head))}}
	for i, pet := range c.InventoryState.Pets {
		if pet.ID == 0 {
			continue
		}
		learned := map[uint16]skills.Progress{}
		for _, sk := range pet.Skills {
			if sk.ID != 0 {
				learned[sk.ID] = skills.FromEXP(sk.Grade, sk.Exp)
			}
		}
		targets = append(targets, skills.Target{Slot: byte(i + 1), Name: pet.Name, Element: pet.Stats.Element, Skills: learned})
	}
	return targets
}
func (c *Client) assignPetSkills() {
	if c.npcTemplates == nil {
		c.npcTemplates, _ = world.NPCTemplates(c.Assets)
	}
	for i := range c.InventoryState.Pets {
		p := &c.InventoryState.Pets[i]
		if p.ID == 0 {
			continue
		}
		t, ok := c.npcTemplates[uint32(p.ID)]
		if !ok {
			t = c.npcTemplates[game.BroadcastID(uint32(p.ID))]
		}
		p.Stats.Element = t.Element
		if len(p.Name) == 0 {
			p.Name = []byte(t.Name)
		}
		for j, id := range t.Skills {
			p.Skills[j].ID = id
		}
	}
}
func (c *Client) skillPacket(p []byte) bool {
	if c.World == nil || c.SkillState == nil || len(p) < 2 {
		return false
	}
	known := p[0] == protocol.CommandCharacterState && (p[1] == protocol.CharacterStateSkillProficiency || p[1] == protocol.CharacterStateWireCode12)
	known = known || p[0] == protocol.CommandStats && p[1] == protocol.StatsStatUpdate && len(p) > 2 && (p[2] == game.StatSkillGrade || p[2] == game.StatSkillEXP && len(p) == 12 && game.IsAlchemySkill(uint16(binary.LittleEndian.Uint32(p[8:]))))
	if !known {
		return false
	}
	if c.SkillState.Apply(p) {
		c.Skills.Refresh()
	} else if c.Unhandled != nil {
		c.Unhandled(p)
	}
	return true
}
