package app

import (
	"encoding/binary"
	"fmt"
	"image"
	"sort"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/internal/protocol"
)

const (
	nativeGroundRecordBytes    = 15
	nativeFloorDropHeaderBytes = 12
	battleItemFlightDuration   = 600 * time.Millisecond
	floorPickupRangePixels     = 180 // Matches the server's native ground-pickup reach.
)

type floorItem struct {
	id   uint16
	x, y int
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
			c.floorItems[uint16(slot)] = floorItem{word(2), int(word(4)), int(word(6))}
		}
	case protocol.InventoryGroundItems:
		if (len(p)-2)%nativeGroundRecordBytes != 0 {
			return true
		}
		for at := 2; at < len(p); at += nativeGroundRecordBytes {
			c.floorItems[word(at+1)] = floorItem{uint16(binary.LittleEndian.Uint32(p[at+3:])), int(word(at + 7)), int(word(at + 9))}
		}
	case protocol.InventoryPickup:
		if len(p) == 5 {
			if c.floorPendingPickup == word(2) {
				c.floorPendingPickup = 0
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
func (c *Client) drawFloorItems() {
	if c.World == nil || c.floorMap != c.World.Player.Map {
		return
	}
	if slot := c.floorPendingPickup; slot != 0 {
		if item, ok := c.floorItems[slot]; ok {
			dx, dy := item.x-int(c.World.Player.X), item.y-int(c.World.Player.Y)
			if dx*dx+dy*dy <= floorPickupRangePixels*floorPickupRangePixels {
				c.Net.Send(protocol.Builder{protocol.CommandInventory, protocol.InventoryPickup}.U16(slot))
				c.floorPendingPickup = 0
			}
		}
	}
	var slots []int
	for slot := range c.floorItems {
		slots = append(slots, int(slot))
	}
	sort.Ints(slots)
	for _, slot := range slots {
		item := c.floorItems[uint16(slot)]
		c.drawAcquiredItem(item.id, c.floorItemPoint(item))
	}
}
func (c *Client) drawAcquiredItem(id uint16, at image.Point) {
	icon := c.items[id].Icon
	if icon == 0 {
		return
	}
	name := fmt.Sprint(icon)
	c.loadPictures(name)
	pic := c.Pics.Find(name)
	w, h := c.Pics.Size(pic)
	c.Pics.Draw(c.Screen, pic, at.X-w/2, at.Y-h, true)
}
func (c *Client) pickupFloorItem(x, y int) bool {
	if c.World == nil || c.floorMap != c.World.Player.Map {
		return false
	}
	var selected uint16
	for slot, item := range c.floorItems {
		at := c.floorItemPoint(item)
		if image.Pt(x, y).In(image.Rect(at.X-18, at.Y-36, at.X+18, at.Y+4)) && (selected == 0 || slot < selected) {
			selected = slot
		}
	}
	if selected == 0 {
		return false
	}
	item := c.floorItems[selected]
	dx, dy := item.x-int(c.World.Player.X), item.y-int(c.World.Player.Y)
	if dx*dx+dy*dy > floorPickupRangePixels*floorPickupRangePixels {
		c.floorPendingPickup = selected
		c.walkTo(x, y)
		return true
	}
	c.Net.Send(protocol.Builder{protocol.CommandInventory, protocol.InventoryPickup}.U16(selected))
	return true
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
