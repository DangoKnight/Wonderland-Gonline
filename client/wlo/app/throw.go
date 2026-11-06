package app

import (
	"fmt"
	"math/rand/v2"
	"time"

	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/minigame"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/world"
)

// The dodgeball game's explanation form (FUN_001b5740 case 1) and its
// Leave (FUN_001b20c0 case 1).
var throwLayout = sportLayout{"Throw_Exp_Form", 0xe1, 0x1fb, 0x184, 0x7a, 0xd2, 0x1d1, 0x152, "", 0, 0}

const (
	throwExitLeft = 700
	throwExitTop  = 0x14
	// throwSceneBase is FUN_001ae968's scene: the battle scene record
	// 59000 + the start packet's sixth byte.
	throwSceneBase = 59000
	throwVariant   = 5 // offset in 57/1 after the command byte
	throwHeight    = world.DefaultBattleHeight
)

// newThrow is 57/1 kind 1 (FUN_001ae788 and FUN_001ae968): the player's
// side and the launch template's NPCs on the scene the start packet names.
// In a party the members line up, leader first, and play only on their own
// clients; alone the player gets three of the template's figures as
// computer-run teammates (FUN_001af02c, FUN_001aef04).
func (c *Client) newThrow(s []byte, template uint16, result func(bool)) *minigame.Throw {
	var own []minigame.Figure
	player := 0
	ownAI := true
	if c.World != nil {
		ids := []uint32{c.World.Player.ID}
		if c.TeamState != nil && c.TeamState.InParty() {
			ids, ownAI = c.TeamState.Members, false
		}
		for i, id := range ids {
			if id == c.World.Player.ID {
				player = i
				own = append(own, c.humanFigure(c.World.Player))
			} else if peer := c.World.Peers[id]; peer != nil {
				own = append(own, c.humanFigure(peer.Player))
			} else if peer := c.teamAppearances[id]; peer != nil {
				own = append(own, c.humanFigure(peer.Player))
			}
		}
		if ownAI {
			for len(own) < 4 {
				own = append(own, c.npcFigure(template))
			}
		}
	}
	npc := make([]minigame.Figure, len(own))
	for i := range npc {
		npc[i] = c.npcFigure(template)
	}
	g := minigame.NewThrow(c.Pics, own, player, npc, ownAI, c.Now())
	g.Rand, g.Sound, g.Music, g.Result = rand.IntN, c.playSound, c.playMusic, result
	g.Shadow = func(dst *surface.Surface, x, y int) { world.DrawShadow(dst, c.Pics, world.ShadowSmall, x, y) }
	variant := byte(0)
	if len(s) > throwVariant {
		variant = s[throwVariant]
	}
	if c.resources.battleAssets == nil {
		if a, err := clientbattle.Load(c.Assets); err == nil {
			c.resources.battleAssets = a
		}
	}
	if a := c.resources.battleAssets; a != nil {
		if bg, ok := a.Backgrounds[throwSceneBase+uint16(variant)]; ok {
			for _, id := range bg.Pictures {
				if id != 0 {
					c.loadPictures(fmt.Sprint(id))
				}
			}
			g.Background = func(dst *surface.Surface) { c.drawThrowScene(dst, bg) }
		}
	}
	return g
}

// drawThrowScene is FUN_001af700: the scene picture, then the two layers
// at their heights, seen from the battle camera.
func (c *Client) drawThrowScene(dst *surface.Surface, bg clientbattle.Background) {
	for layer := len(bg.Pictures) - 1; layer >= 0; layer-- {
		i := c.Pics.Find(fmt.Sprint(bg.Pictures[layer]))
		if bg.Pictures[layer] == 0 || i < 0 {
			continue
		}
		x, y := -battleSceneCameraX, -battleSceneCameraY
		if layer < 2 {
			y = int(bg.Heights[layer]) - battleSceneCameraY
		}
		c.Pics.Draw(dst, i, x, y, layer != 2)
	}
}

// humanFigure is a character's body for the game.
func (c *Client) humanFigure(pl world.Player) minigame.Figure {
	if c.lib == nil {
		return nil
	}
	h := role.NewHuman(c.lib, c.items)
	h.Now = func() time.Time { return c.Now() }
	ch := login.CharacterSlot{Body: pl.Body, Head: pl.Head, Color1: pl.Color1, Color2: pl.Color2}
	for _, id := range pl.Items {
		if it, ok := c.items[id]; ok && it.Definition.EquipSlot >= 1 && it.Definition.EquipSlot <= 6 {
			ch.Equipment[it.Definition.EquipSlot] = id
		}
	}
	h.SetCharacter(&ch)
	return humanFigure{h}
}

type humanFigure struct{ *role.Human }

func (f humanFigure) Draw(dst *surface.Surface, x, y, action int) {
	f.DrawBody(dst, x, y, int32(action))
}
func (f humanFigure) Height() int { return throwHeight }

// npcFigure is the launch template's figure.
func (c *Client) npcFigure(template uint16) minigame.Figure {
	m := c.newMonster(template)
	if m == nil {
		return nil
	}
	return npcFigure{m.(*monsterRole)}
}

type npcFigure struct{ *monsterRole }

// Draw leaves the shadow to the game, which draws one under everyone.
func (f npcFigure) Draw(dst *surface.Surface, x, y, action int) { f.NPC.Draw(dst, x, y, action) }
func (f npcFigure) Height() int {
	if _, h := f.FrameSize(0x10); h > 0 {
		return h
	}
	return throwHeight
}

// sportRelease passes the left button's release to a game that wants it
// (the manager's mouse-up, FUN_003bd32c).
func (c *Client) sportRelease(held bool, x, y int) {
	sp := c.sport
	if sp == nil {
		return
	}
	if held {
		sp.held = true
		return
	}
	if !sp.held {
		return
	}
	sp.held = false
	if g, ok := sp.game.(interface{ Release(int, int, time.Time) }); ok {
		g.Release(x, y, c.Now())
	}
}
