package app

import (
	"encoding/binary"
	"image"
	"time"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/team"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const mainTeamButton = 4
const joinTeamButton = 5

func (c *Client) initTeam() {
	c.TeamState = &team.State{}
	c.Team = team.NewForm(c.Env, c.TeamState, c.Inventory)
	c.resources.loadInstances(c.Assets)
	c.Team.Instances.Definitions = c.resources.instanceDefinitions
	c.Team.CanAct = c.Inventory.CanAct
	c.Team.Send = c.Net.Send
	c.Team.Notice = c.Chat.Notice
	c.Team.NameOf = c.playerName
	// Confirmation previews have their own fixed frame, independent of Inventory.
	previews := make(map[uint16]*role.NPC)
	preview := func(slot byte) *role.NPC {
		if c.lib == nil {
			return nil
		}
		id := c.InventoryState.Pets[slot-1].ID
		if c.npcTemplates == nil {
			c.npcTemplates, _ = world.NPCTemplates(c.Assets)
		}
		t, ok := c.npcTemplates[uint32(id)]
		if !ok {
			return nil
		}
		if previews[id] == nil {
			n := role.NewNPC(c.lib, t.Look, t.Colors)
			n.Now = func() time.Time { return c.Now() }
			n.SetHeightScale(t.HeightScale)
			n.Hold(0, false)
			previews[id] = n
		}
		return previews[id]
	}
	c.Team.DrawPetBody = func(slot byte, x, y, action int) {
		if n := preview(slot); n != nil {
			n.Draw(c.Screen, x, y, action)
		}
	}
	c.Team.PetPreviewSize = func(slot byte) (int, int) {
		if n := preview(slot); n != nil {
			return n.FrameSize(team.PetConfirmationAction)
		}
		return 0, 0
	}
	c.Team.PetPreviewTemplate = func(slot byte) world.NPCTemplate {
		preview(slot)
		return c.npcTemplates[uint32(c.InventoryState.Pets[slot-1].ID)]
	}
	c.Team.StatsOf = func(id uint32) *world.Stats {
		m := c.TeamState.Vitals[id]
		if m == nil {
			return nil
		}
		return &m.Stats
	}
	c.Team.DrawFace = func(id uint32, r image.Rectangle) {
		peer := c.teamAppearances[id]
		if peer == nil {
			return
		}
		if face, ok := peer.Role.(interface {
			DrawFace(*surface.Surface, int, int, int, bool)
		}); ok {
			face.DrawFace(c.Screen, r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2, role.PortraitAction, false)
		}
	}
	c.Team.DrawPetFace = func(slot byte, r image.Rectangle) {
		if c.lib == nil {
			return
		}
		pet := c.InventoryState.Pets[slot-1]
		if c.npcTemplates == nil {
			c.npcTemplates, _ = world.NPCTemplates(c.Assets)
		}
		t := c.npcTemplates[uint32(pet.ID)]
		if portrait := c.resources.petPortrait(t); portrait != nil {
			c.Screen.DrawStretch(r, portrait, true)
		}
	}
	c.UI.Add(c.Team)
	c.MainButtons.Buttons[mainTeamButton].OnClick = c.toggleTeam
	c.FuncButtons.Buttons[joinTeamButton].OnClick = c.toggleJoinTeam
}

// Join Team is a world selection tool: clicking the toolbar arms it;
// the next player click sends the native AC13:1 invitation request.
func (c *Client) setJoinTeamTarget(active bool) {
	c.joinTeamTarget = active
	if c.World != nil {
		c.World.JoinTeamSelection = active
		c.World.Hover(-1, -1)
	}
	if c.FuncButtons != nil && c.FuncButtons.Buttons[joinTeamButton] != nil {
		c.FuncButtons.Buttons[joinTeamButton].IconDown = active
	}
}
func (c *Client) toggleJoinTeam() {
	if c.joinTeamTarget {
		c.setJoinTeamTarget(false)
		return
	}
	if c.World == nil || !c.Inventory.CanAct() || c.UI.Modal != nil {
		return
	}
	if c.inTeam() {
		c.Notices.Show([]byte("Leave your current party before joining another."), inventoryWarningDuration, c.Now())
		return
	}
	c.Team.Hide()
	c.World.StopWalk()
	c.groundHeld = false
	c.setJoinTeamTarget(true)
}
func (c *Client) toggleTeam() {
	if c.Team.Visible {
		c.Team.Hide()
	} else if c.Inventory.CanAct() && c.UI.Modal == nil {
		c.Team.Show()
	}
}
func (c *Client) TeamKey(key uint16, shift byte) bool {
	if key == 'T' && shift&shiftCtrl != 0 {
		c.toggleTeam()
		return true
	}
	if key == escapeKey && c.Team.Instances.New.Visible {
		c.Team.Instances.New.Hide()
		return true
	}
	if key == escapeKey && c.Team.Visible && c.Team.Instances.Visible {
		c.Team.Instances.Hide()
		return true
	}
	if key == escapeKey && c.joinTeamTarget {
		c.setJoinTeamTarget(false)
		return true
	}
	if key == escapeKey && c.Team.Visible && (c.UI.Active == c.Team || c.Input.Focused != nil && c.Input.Focused.Base().Root() == c.Team) {
		c.Team.Hide()
		return true
	}
	return false
}
func (c *Client) teamTargetAt(x, y int) bool {
	if !c.joinTeamTarget {
		return false
	}
	if c.World == nil || !c.Inventory.CanAct() || c.UI.Modal != nil || c.inTeam() {
		c.setJoinTeamTarget(false)
		return true
	}
	if id := c.World.TeamTargetAt(x, y); id != 0 {
		c.setJoinTeamTarget(false)
		if err := c.Net.Send(protocol.Builder{protocol.CommandTeam, protocol.TeamRequest}.U32(id)); err != nil {
			c.Notices.Show([]byte(err.Error()), inventoryWarningDuration, c.Now())
		}
		return true
	}
	return true
}
func (c *Client) rememberTeammate(peer *world.Peer) {
	if c.teamAppearances == nil {
		c.teamAppearances = map[uint32]*world.Peer{}
	}
	if peer.Role == nil && c.lib != nil {
		peer.Role = role.NewHuman(c.lib, c.items)
		world.Dress(peer.Role, peer.Player)
	}
	c.teamAppearances[peer.ID] = peer
	m := c.TeamState.Vitals[peer.ID]
	if m == nil {
		m = &team.Member{ID: peer.ID}
		c.TeamState.Vitals[peer.ID] = m
	}
	m.Stats.Element = peer.Element
	m.Stats.Level = peer.Level
	m.Stats.Formula = c.Stats.Formula
}
func (c *Client) teamPacket(p []byte) bool {
	if len(p) < 2 || c.World == nil || c.Team == nil {
		return false
	}
	switch p[0] {
	case protocol.CommandInstance:
		if !c.Team.Instances.Apply(p) && c.Unhandled != nil {
			c.Unhandled(p)
		}
		return true
	case protocol.CommandTeam:
		switch p[1] {
		case protocol.TeamRoster:
			if !c.TeamState.ApplyRoster(p) && c.Unhandled != nil {
				c.Unhandled(p)
			}
			if c.inTeam() {
				c.setJoinTeamTarget(false)
			}
			c.Team.Refresh()
			return true
		case protocol.TeamRequest:
			if len(p) < 7 || len(p) != 7+int(p[6]) {
				if c.Unhandled != nil {
					c.Unhandled(p)
				}
				return true
			}
			id := binary.LittleEndian.Uint32(p[2:])
			name := append([]byte(nil), p[7:]...)
			if id == 0 || id == c.World.Player.ID {
				return true
			}
			c.Team.Confirm(string(name)+": Please let me join you!", 0, func() {
				if c.Inventory.CanAct() {
					if err := c.Net.Send(protocol.Builder{protocol.CommandTeam, protocol.TeamReply, 1}.U32(id)); err != nil {
						c.Notices.Show([]byte(err.Error()), inventoryWarningDuration, c.Now())
					}
				}
			}, func() {
				if c.Inventory.CanAct() {
					if err := c.Net.Send(protocol.Builder{protocol.CommandTeam, protocol.TeamReply, 2}.U32(id)); err != nil {
						c.Notices.Show([]byte(err.Error()), inventoryWarningDuration, c.Now())
					}
				}
			})
			return true
		case protocol.TeamLeave:
			if len(p) == 6 && binary.LittleEndian.Uint32(p[2:]) == c.World.Player.ID {
				c.TeamState.Reset(c.World.Player.ID)
			}
			return true
		case protocol.TeamFormation:
			return true // Public follow presentation is independent of own membership.
		}
	case protocol.CommandStats:
		if p[1] == protocol.StatsWireCode3 {
			if len(p) >= 6 {
				id := binary.LittleEndian.Uint32(p[2:])
				if m := c.TeamState.Vitals[id]; m != nil {
					m.Stats.Formula = c.Stats.Formula
				}
			}
			if !c.TeamState.ApplyStat(p) && c.Unhandled != nil {
				c.Unhandled(p)
			}
			return true
		}
	case protocol.CommandBattlePet:
		switch p[1] {
		case protocol.BattlePetSelect:
			if len(p) != 6 {
				return true
			}
			id := uint16(binary.LittleEndian.Uint32(p[2:]))
			c.TeamState.BattlePet = id
			c.showCompanion(c.World.Player.ID, id, nil)
			return true
		case protocol.BattlePetRest:
			if len(p) != 2 {
				return true
			}
			c.TeamState.BattlePet = 0
			delete(c.World.Companions, c.World.Player.ID)
			return true
		case protocol.BattlePetWireCode7:
			if len(p) == 6 {
				delete(c.World.Companions, binary.LittleEndian.Uint32(p[2:]))
			}
			return true
		}
	case protocol.CommandPetControl:
		switch p[1] {
		case protocol.PetControlBoardVehicleAlternate:
			if len(p) >= 7 && binary.LittleEndian.Uint32(p[2:]) == c.World.Player.ID && p[6] >= 1 && p[6] <= game.MaxPets {
				pet := &c.InventoryState.Pets[p[6]-1]
				pet.Name = append([]byte(nil), p[7:]...)
				if companion := c.World.Companions[c.World.Player.ID]; companion != nil && companion.ID == uint32(pet.ID) {
					companion.Name = append([]byte(nil), pet.Name...)
				}
			}
			return true
		case protocol.PetControlMount:
			if len(p) != 37 {
				return true
			}
			owner := binary.LittleEndian.Uint32(p[3:])
			id := uint16(binary.LittleEndian.Uint32(p[7:]))
			if owner == c.World.Player.ID {
				c.TeamState.MountPet = id
			}
			delete(c.World.Companions, owner)
			c.setPetMount(owner, id)
			return true
		case protocol.PetControlUnmount:
			if len(p) != 6 {
				return true
			}
			owner := binary.LittleEndian.Uint32(p[2:])
			if owner == c.World.Player.ID {
				c.TeamState.MountPet = 0
			}
			c.setPetMount(owner, 0)
			return true
		case protocol.PetControlWireCode4:
			if len(p) < 14 {
				return true
			}
			nameLen := int(p[12])
			if len(p) < 13+nameLen {
				return true
			}
			c.showCompanion(binary.LittleEndian.Uint32(p[2:]), uint16(binary.LittleEndian.Uint32(p[6:])), p[13:13+nameLen])
			return true
		case protocol.PetControlWireCode1:
			if c.InventoryState.ApplyPetRecruit(p, c.World.Player.ID) {
				c.assignPetSkills()
				c.Skills.Refresh()
			}
			return true
		case protocol.PetControlPetSlot:
			if len(p) == 7 && binary.LittleEndian.Uint32(p[2:]) == c.World.Player.ID && p[6] >= 1 && p[6] <= game.MaxPets {
				id := c.InventoryState.Pets[p[6]-1].ID
				if c.TeamState.BattlePet == id {
					c.TeamState.BattlePet = 0
					delete(c.World.Companions, c.World.Player.ID)
				}
				if c.TeamState.MountPet == id {
					c.TeamState.MountPet = 0
					c.setPetMount(c.World.Player.ID, 0)
				}
				c.InventoryState.Pets[p[6]-1] = inventory.UsePet{}
				c.Inventory.Select(0)
				c.Skills.Refresh()
			}
			return true
		}
	}
	return false
}
func (c *Client) showCompanion(owner uint32, id uint16, name []byte) {
	if c.npcTemplates == nil {
		c.npcTemplates, _ = world.NPCTemplates(c.Assets)
	}
	t, ok := c.npcTemplates[uint32(id)]
	if !ok {
		return
	}
	if len(name) == 0 && owner == c.World.Player.ID {
		for _, pet := range c.InventoryState.Pets {
			if pet.ID == id && len(pet.Name) != 0 {
				name = pet.Name
				break
			}
		}
	}
	if len(name) == 0 {
		name = []byte(t.Name)
	}
	var painter world.NPCPainter
	if c.lib != nil {
		sprite := role.NewNPC(c.lib, t.Look, t.Colors)
		sprite.Now = func() time.Time { return c.Now() }
		sprite.SetHeightScale(t.HeightScale)
		painter = sprite
	}
	c.World.SetCompanion(owner, uint32(id), name, painter)
	c.World.Companions[owner].Info = t
}
func (c *Client) setPetMount(owner uint32, id uint16) {
	var body any = c.World.Body
	if owner != c.World.Player.ID {
		peer := c.World.Peers[owner]
		if peer == nil {
			return
		}
		body = peer.Role
	}
	if h, ok := body.(*role.Human); ok {
		if id == 0 {
			h.SetPetMount(nil)
			h.SetPetMountPlacement(nil)
			return
		}
		if c.npcTemplates == nil {
			c.npcTemplates, _ = world.NPCTemplates(c.Assets)
		}
		t, found := c.npcTemplates[uint32(id)]
		if !found {
			return
		}
		sprite := role.NewNPC(c.lib, t.Look, t.Colors)
		sprite.Now = func() time.Time { return c.Now() }
		h.SetPetMount(sprite)
		h.SetPetMountGeometry(t.HeightScale, t.HeightPreset)
		c.resources.loadMountPositions(c.Assets)
		position := role.ResolveMountPlacement(uint16(sprite.Sprite()), c.resources.mountPositions)
		h.SetPetMountPlacement(&position)
	}
}
