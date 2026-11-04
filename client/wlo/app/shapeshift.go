package app

import (
	"encoding/binary"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/world"
)

type shapePortrait struct {
	login.RoleView
	sprite *role.NPC
	face   uint16
}

func (p *shapePortrait) DrawPortrait(dst *surface.Surface, x, y int, blinking bool) {
	p.sprite.DrawFace(dst, x, y, p.face, blinking)
}

const (
	shapeshiftHeaderBytes = 2
	shapeshiftRecordBytes = 6 // Owner UInt32, NPC template UInt16; zero restores.
)

func (c *Client) receiveShapeshift(p []byte) {
	if len(p) < shapeshiftHeaderBytes+shapeshiftRecordBytes || (len(p)-shapeshiftHeaderBytes)%shapeshiftRecordBytes != 0 || c.World == nil {
		return
	}
	for record := p[shapeshiftHeaderBytes:]; len(record) > 0; record = record[shapeshiftRecordBytes:] {
		c.setShapeshift(binary.LittleEndian.Uint32(record), binary.LittleEndian.Uint16(record[4:]))
	}
}

func (c *Client) setShapeshift(owner uint32, id uint16) {
	if owner != c.World.Player.ID && c.World.Peers[owner] == nil {
		return
	}
	if id == 0 {
		c.World.SetShapeshift(owner, world.PlayerShape{})
		c.invalidateShapeshiftBattleBody(owner)
		if owner == c.World.Player.ID {
			c.MainStatus.Portrait = c.World.Body
		}
		return
	}
	template, known := c.npcTemplates[uint32(id)]
	if !known {
		return
	}
	shape := world.PlayerShape{Template: id, Info: template}
	if c.lib != nil {
		sprite := role.NewNPC(c.lib, template.Look, template.Colors)
		sprite.SetHeightScale(template.HeightScale)
		sprite.Now = func() time.Time { return c.Now() }
		shape.Painter = sprite
		if owner == c.World.Player.ID {
			c.MainStatus.Portrait = &shapePortrait{RoleView: c.World.Body, sprite: sprite, face: template.Face}
		}
	}
	c.World.SetShapeshift(owner, shape)
	c.invalidateShapeshiftBattleBody(owner)
}

func (c *Client) invalidateShapeshiftBattleBody(owner uint32) {
	for cell, body := range c.battle.bodies {
		if body.kind == clientbattle.Player && body.id == owner {
			delete(c.battle.bodies, cell)
		}
	}
}
