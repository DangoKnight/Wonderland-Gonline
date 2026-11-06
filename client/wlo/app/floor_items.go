package app

import "wonderland-gonline/internal/localization"

import (
	"encoding/binary"
	"fmt"
	"image"
	"sort"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/protocol"
)

const (
	nativeGroundRecordBytes       = 15
	nativeGroundDepthOffset       = 11
	nativeFloorDropHeaderBytes    = 12
	battleItemFlightDuration      = 600 * time.Millisecond
	floorPickupRangePixels        = 180 // Matches the server's native ground-pickup reach.
	worldItemFlightDuration       = 600 * time.Millisecond
	worldItemFlightHeight         = 60
	worldItemTargetLift           = 25
	itemAcquisitionNoticeDuration = 1300 * time.Millisecond // FUN_003dbdb8: 0x514 ms.
	floorItemHoverLight           = 15                      // FUN_003d7b80: additive icon highlight.
	floorItemNameInk              = 0xffe0
	floorItemNameStyle            = 2
	floorItemNameGlyphWidth       = 8
	floorItemNameHeight           = 16
	floorItemNameOffsetX          = 24
	floorItemNameOffsetY          = -floorItemNameHeight / 2
	floorItemMovementFacings      = 8
)

type worldItemFlight struct {
	source, target image.Point // World coordinates, independent of camera movement.
	item           uint16
	mapID          uint16
	at             time.Time
}

// Native AC23:6 announces a successful grant and optionally flies its icon from
// the last clicked NPC (FUN_003dbdb8 -> FUN_00308e70). Event resume does not cancel it.
func (c *Client) presentItemAcquisition(p []byte) {
	id := binary.LittleEndian.Uint16(p[2:])
	c.Notices.Show([]byte(fmt.Sprintf(localization.UI("ui.app.obtain_s_x_d_a8afa433"), clientassets.Big5Text(c.items[id].Definition.Name), p[4])), itemAcquisitionNoticeDuration, c.Now())
	if c.sfx != nil {
		c.sfx.Play(`sound\wav0152.wav`)
	}
	if p[protocol.InventoryAcquisitionBytes-1] == 0 || c.World == nil || c.battle.state.Active {
		return
	}
	n := c.World.NPCs[c.acquisitionActor]
	if n == nil || !n.Shown {
		return
	}
	c.worldItemFlights = append(c.worldItemFlights, worldItemFlight{
		source: image.Pt(n.X, n.Y+n.Info.SpriteDrop()),
		target: image.Pt(c.World.Player.X, c.World.Player.Y-worldItemTargetLift),
		item:   id, mapID: c.World.Player.Map, at: c.Now(),
	})
}

func (f worldItemFlight) position(now time.Time) image.Point {
	t := float64(max(time.Duration(0), min(now.Sub(f.at), worldItemFlightDuration))) / float64(worldItemFlightDuration)
	return image.Pt(f.source.X+int(float64(f.target.X-f.source.X)*t),
		f.source.Y+int(float64(f.target.Y-f.source.Y)*t)-int(4*worldItemFlightHeight*t*(1-t)))
}

func (c *Client) drawWorldItemGains(now time.Time) {
	cx, cy := c.World.Camera()
	keep := c.worldItemFlights[:0]
	for _, f := range c.worldItemFlights {
		if f.mapID != c.World.Player.Map || now.Sub(f.at) >= worldItemFlightDuration {
			continue
		}
		keep = append(keep, f)
		c.drawGroundItemOn(c.Screen, f.item, f.position(now).Sub(image.Pt(cx, cy)))
	}
	c.worldItemFlights = keep
}

type floorItem struct {
	id    uint16
	x, y  int
	depth int32
}
type battleItemFlight struct {
	source, target clientbattle.Cell
	item           uint16
	at             time.Time
}

func (c *Client) floorItemPacket(p []byte) bool {
	if c.World == nil || len(p) < 2 || p[0] != protocol.CommandInventory {
		return false
	}
	if p[1] != protocol.InventoryDrop && p[1] != protocol.InventoryGroundItems && p[1] != protocol.InventoryPickup {
		return false
	}
	if c.floorMap != c.World.Player.Map {
		c.floorItems = nil
		c.floorPendingPickup, c.floorHovered = 0, 0
		c.floorMap = c.World.Player.Map
	}
	if c.floorItems == nil {
		c.floorItems = map[uint16]floorItem{}
	}
	word := func(at int) uint16 { return binary.LittleEndian.Uint16(p[at:]) }
	switch p[1] {
	case protocol.InventoryDrop:
		if len(p) <= nativeFloorDropHeaderBytes {
			return true
		}
		for _, slot := range p[nativeFloorDropHeaderBytes:] {
			c.floorItems[uint16(slot)] = floorItem{id: word(2), x: int(word(4)), y: int(word(6))}
		}
	case protocol.InventoryGroundItems:
		if (len(p)-2)%nativeGroundRecordBytes != 0 {
			return true
		}
		for at := 2; at < len(p); at += nativeGroundRecordBytes {
			c.floorItems[word(at+1)] = floorItem{id: uint16(binary.LittleEndian.Uint32(p[at+3:])), x: int(word(at + 7)), y: int(word(at + 9)), depth: int32(binary.LittleEndian.Uint32(p[at+nativeGroundDepthOffset:]))}
		}
	case protocol.InventoryPickup:
		if len(p) == 5 {
			if item, ok := c.floorItems[word(2)]; ok && p[4] != 0 {
				c.worldItemFlights = append(c.worldItemFlights, worldItemFlight{
					source: image.Pt(item.x, item.y), target: image.Pt(c.World.Player.X, c.World.Player.Y-worldItemTargetLift),
					item: item.id, mapID: c.World.Player.Map, at: c.Now(),
				})
			}
			if c.floorPendingPickup == word(2) {
				c.floorPendingPickup = 0
			}
			if c.floorHovered == word(2) {
				c.floorHovered = 0
			}
			delete(c.floorItems, word(2))
		}
	}
	return true
}

func (c *Client) floorItemPoint(item floorItem) image.Point {
	x, y := c.World.Camera()
	return image.Pt(item.x-x, item.y-y)
}
func (c *Client) reachFloorItem() {
	if c.World == nil || c.floorMap != c.World.Player.Map || c.held || c.event.active || c.battle.state.Active || c.movie != nil {
		return
	}
	if slot := c.floorPendingPickup; slot != 0 {
		item, ok := c.floorItems[slot]
		if !ok {
			c.floorPendingPickup = 0
			return
		}
		dx, dy := item.x-c.World.Player.X, item.y-c.World.Player.Y
		if dx*dx+dy*dy <= floorPickupRangePixels*floorPickupRangePixels {
			c.stopWalkForFloorPickup()
			c.floorPendingPickup = 0
			c.Net.Send(protocol.Builder{protocol.CommandInventory, protocol.InventoryPickup}.U16(slot))
		}
	}
}

func (c *Client) floorItemSprites() []world.SceneSprite {
	if c.World == nil || c.floorMap != c.World.Player.Map {
		return nil
	}
	var slots []int
	for slot := range c.floorItems {
		slots = append(slots, int(slot))
	}
	sort.Ints(slots)
	sprites := make([]world.SceneSprite, 0, len(slots))
	for _, slot := range slots {
		item := c.floorItems[uint16(slot)]
		lit := uint16(slot) == c.floorHovered
		sprites = append(sprites, world.SceneSprite{X: item.x, Y: item.y, Depth: int(item.depth), Draw: func(dst *surface.Surface, x, y int) {
			c.drawGroundItemOn(dst, item.id, image.Pt(x, y))
			if lit {
				pic, w, h := c.itemPicture(item.id)
				c.Pics.DrawLight(dst, pic, x-w/2, y-h/2, image.Rect(0, 0, w, h), floorItemHoverLight)
			}
		}})
	}
	return sprites
}
func (c *Client) drawAcquiredItem(id uint16, at image.Point) {
	c.drawAcquiredItemOn(c.Screen, id, at)
}
func (c *Client) drawAcquiredItemOn(dst *surface.Surface, id uint16, at image.Point) {
	pic, w, h := c.itemPicture(id)
	if pic >= 0 {
		c.Pics.Draw(dst, pic, at.X-w/2, at.Y-h, true)
	}
}

// FUN_003d78e4 stores half the icon dimensions; FUN_003d7b80 subtracts
// both halves. The wire position is the centre, not the bottom of the icon.
func (c *Client) drawGroundItemOn(dst *surface.Surface, id uint16, at image.Point) {
	pic, w, h := c.itemPicture(id)
	if pic >= 0 {
		c.Pics.Draw(dst, pic, at.X-w/2, at.Y-h/2, true)
	}
}

func (c *Client) groundItemBounds(item floorItem) image.Rectangle {
	pic, w, h := c.itemPicture(item.id)
	if pic < 0 {
		return image.Rectangle{}
	}
	at := c.floorItemPoint(item)
	return image.Rect(at.X-w/2, at.Y-h/2, at.X-w/2+w, at.Y-h/2+h)
}

func (c *Client) itemPicture(id uint16) (pic, w, h int) {
	icon := c.items[id].Icon
	if icon == 0 {
		return -1, 0, 0
	}
	name := fmt.Sprint(icon)
	c.loadPictures(name)
	pic = c.Pics.Find(name)
	w, h = c.Pics.Size(pic)
	return
}
func (c *Client) pickupFloorItem(x, y int) bool {
	selected := c.floorItemAt(x, y)
	if selected == 0 {
		return false
	}
	c.pendingNPC = nil
	c.waterTravel.goal = nil
	c.groundHeld, c.groundSince = false, time.Time{}
	c.stopWalkForFloorPickup()
	c.floorPendingPickup = 0
	item := c.floorItems[selected]
	dx, dy := item.x-c.World.Player.X, item.y-c.World.Player.Y
	if dx*dx+dy*dy > floorPickupRangePixels*floorPickupRangePixels {
		c.floorPendingPickup = selected
		at := c.floorItemPoint(item)
		c.walkTo(at.X, at.Y)
		return true
	}
	c.Net.Send(protocol.Builder{protocol.CommandInventory, protocol.InventoryPickup}.U16(selected))
	return true
}

func (c *Client) stopWalkForFloorPickup() {
	walking := c.World.Walking()
	c.World.StopWalk()
	c.World.Marker.Hide()
	if walking {
		// Legs announce their destination at the start. Check pickup reach at
		// the actual stopping position, using the existing native movement packet.
		c.sendLeg(int(c.World.Player.Direction)%floorItemMovementFacings, c.World.Player.X, c.World.Player.Y)
	}
}

// Like the native item hit test, inspect source pixels rather than the screen:
// transparent icon margins do not intercept walking. Select the last drawn item.
func (c *Client) floorItemAt(x, y int) uint16 {
	if c.World == nil || c.floorMap != c.World.Player.Map {
		return 0
	}
	var selected uint16
	var depth int
	for slot, item := range c.floorItems {
		r := c.groundItemBounds(item)
		if !image.Pt(x, y).In(r) {
			continue
		}
		pic, _, _ := c.itemPicture(item.id)
		if c.Pics.Pixel(pic, x-r.Min.X, y-r.Min.Y) == 0 {
			continue
		}
		key := item.y + int(item.depth)
		if selected == 0 || key > depth || key == depth && slot > selected {
			selected = slot
			depth = key
		}
	}
	return selected
}

func (c *Client) updateFloorHover() {
	c.floorHovered = 0
	if c.World == nil || c.uiHovered || c.Talk.Contains(c.Input.X, c.Input.Y) || c.movie != nil || c.sport != nil || c.battle.state.Active || c.held || c.event.active || c.sceneFrozen() || c.settingsPromptForm != nil {
		return
	}
	x, y := c.World.ScenePoint(c.Input.X, c.Input.Y)
	c.floorHovered = c.floorItemAt(x, y)
}

func (c *Client) drawFloorItemName() {
	item, ok := c.floorItems[c.floorHovered]
	if !ok || c.floorHovered == 0 || c.World.HideNames {
		return
	}
	name := clientassets.Big5Text(c.items[item.id].Definition.Name)
	if len(name) == 0 {
		return
	}
	width := len(name) * floorItemNameGlyphWidth
	x := c.Input.X + floorItemNameOffsetX
	if x+width+floorItemNameGlyphWidth > ScreenWidth {
		x = c.Input.X - width - floorItemNameOffsetX
	}
	x = max(0, x)
	y := max(0, min(c.Input.Y+floorItemNameOffsetY, ScreenHeight-floorItemNameHeight))
	c.Env.Text.Draw(x, y, 0, false, true, c.Screen, name, floorItemNameHeight, width+floorItemNameGlyphWidth, 0, floorItemNameInk, floorItemNameStyle)
}

func (c *Client) battleItemGainPacket(p []byte) bool {
	if len(p) < 2 || p[0] != protocol.CommandBattleEffect || p[1] != protocol.BattleEffectItemGain {
		return false
	}
	if len(p) < 8 || (len(p)-6)%2 != 0 {
		return true
	}
	source := clientbattle.Cell{X: p[4], Y: p[5]}
	if !source.Valid() {
		return true
	}
	for at := 6; at < len(p); at += 2 {
		target := clientbattle.Cell{X: p[at], Y: p[at+1]}
		if target.Valid() {
			c.itemFlights = append(c.itemFlights, battleItemFlight{source, target, binary.LittleEndian.Uint16(p[2:]), c.Now()})
		}
	}
	return true
}
func (c *Client) drawBattleItemGains(now time.Time, offset image.Point) {
	keep := c.itemFlights[:0]
	for _, flight := range c.itemFlights {
		age := now.Sub(flight.at)
		if age >= battleItemFlightDuration {
			continue
		}
		keep = append(keep, flight)
		progress := float64(max(time.Duration(0), age)) / float64(battleItemFlightDuration)
		from, to := flight.source.Position().Add(offset), flight.target.Position().Add(offset)
		at := image.Pt(from.X+int(float64(to.X-from.X)*progress), from.Y+int(float64(to.Y-from.Y)*progress)-int(160*progress*(1-progress)))
		c.drawAcquiredItem(flight.item, at)
	}
	c.itemFlights = keep
}
