package app

import (
	"encoding/binary"
	"fmt"
	"image"
	"sort"
	"strconv"

	"wonderland-gonline/client/wlo/housing"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/localization"
	"wonderland-gonline/internal/protocol"
)

const (
	tentReach                              = 150
	tentExitReach                          = 35
	tentFurniturePictureOffset             = 409 // Item.dat: native item object +0x19d minus prefix.
	tentFurnitureOtherPictureOffset        = 411
	tentFurnitureWidthOffset               = 402
	tentFurnitureHeightOffset              = 401
	tentDefaultFloorItem            uint16 = 39062
	tentDefaultWallpaperItem        uint16 = 39064
	tentFloorPatternWidth                  = 4
	tentFloorPatternHeight                 = 3
	tentWarpFloorOffset                    = 12
	tentWarpLayoutsOffset                  = 13
)

type tentStyle struct {
	kind        byte
	floor, wall uint16
}
type tentDrag struct {
	index uint16
	start image.Point
}

func (c *Client) resetTentScene() {
	c.housing = housing.State{}
	c.tentPending = 0
	c.tentDrag = nil
	c.tentSprites = nil
	c.tentKind = housing.DefaultRoom
	c.tentRoomKinds = [5]byte{1, 1, 1, 1, 1}
	c.tentLeaving = false
	if c.tentBackdrop != nil {
		c.tentBackdrop.Close()
		c.tentBackdrop = nil
	}
}

func (c *Client) tentPacket(p []byte) bool {
	if c.World == nil || len(p) < 2 {
		return false
	}
	if p[0] == protocol.CommandTent && p[1] == protocol.TentEvicted {
		return len(p) == 2
	}
	if p[0] == protocol.CommandTent && p[1] == protocol.TentPillarLayout {
		return len(p) == 33
	}
	if p[0] == protocol.CommandTent && p[1] == protocol.TentInteriorLayout {
		if len(p) != 4 || !game.IsTentMap(c.World.Player.Map) || p[2] < 1 || p[2] > 3 || int(p[3]) >= len(c.tentRoomKinds) {
			return false
		}
		c.tentRoomKinds[p[3]] = p[2]
		c.tentKind = c.tentRoomKinds[c.housing.Floor]
		c.rebuildTentScene()
		return true
	}
	if p[0] == protocol.CommandTentFurniture && p[1] == protocol.TentOpened {
		// Native AC62:59 also includes the owner's tent music inside.
		return len(p) >= 3
	}
	if !c.housing.Receive(p) {
		return false
	}
	if p[0] == protocol.CommandTent {
		c.tentSprites = nil
		if own, ok := c.housing.Signs[c.World.Player.ID]; ok {
			c.openTentItem = own.Item
		}
		if p[1] == protocol.TentClosed && binary.LittleEndian.Uint32(p[2:]) == c.World.Player.ID {
			c.openTentItem = 0
		}
	} else if game.IsTentMap(c.World.Player.Map) {
		c.rebuildTentScene()
		if name := c.tentOwnerName(c.housing.Owner); name != "" {
			c.World.SceneName = fmt.Sprintf(localization.UI("ui.app.tent_home"), name) + " " + fmt.Sprintf(localization.UI("ui.app.tent_floor"), c.housing.Floor+1)
		}
	}
	return true
}

func (c *Client) tentSceneSprites() []world.SceneSprite {
	if c.World == nil {
		return nil
	}
	var out []world.SceneSprite
	owners := make([]uint32, 0, len(c.housing.Signs))
	for id := range c.housing.Signs {
		owners = append(owners, id)
	}
	sort.Slice(owners, func(i, j int) bool { return owners[i] < owners[j] })
	for _, id := range owners {
		v := c.housing.Signs[id]
		if c.tentSprites == nil {
			c.tentSprites = map[uint32]*role.Tent{}
		}
		t := c.tentSprites[id]
		if t == nil {
			t = &role.Tent{Lib: c.lib, Sprite: c.items[v.Item].Sprites[0]}
			c.tentSprites[id] = t
		}
		out = append(out, world.SceneSprite{X: int(v.X), Y: int(v.Y), Draw: func(dst *surface.Surface, x, y int) {
			t.Draw(dst, x, y)
			if name := c.tentOwnerName(v.Owner); name != "" && c.Env != nil && c.Env.Text != nil {
				text := clientassets.Big5Text(fmt.Sprintf(localization.UI("ui.app.tent_home"), name))
				c.Env.Text.Draw(x-len(text)*4, y-120, 0, false, true, dst, text, 0, len(text)*8+8, 0, 0x7e0, 2)
			}
		}})
	}
	if !game.IsTentMap(c.World.Player.Map) {
		return out
	}
	indices := make([]int, 0, len(c.housing.Furniture))
	for index := range c.housing.Furniture {
		indices = append(indices, int(index))
	}
	sort.Ints(indices)
	for _, index := range indices {
		f := c.housing.Furniture[uint16(index)]
		if f.Parent != 0 || f.Floor != c.housing.Floor {
			continue
		}
		pic, bounds := c.tentFurnitureImage(f)
		if pic < 0 {
			continue
		}
		at := housing.Pixel(int(f.X), int(f.Y), int(f.Z))
		out = append(out, world.SceneSprite{X: at.X, Y: at.Y, Draw: func(dst *surface.Surface, x, y int) {
			c.Pics.Draw(dst, pic, x+bounds.Min.X-at.X, y+bounds.Min.Y-at.Y, true)
		}})
	}
	return out
}

func (c *Client) tentOwnerName(id uint32) string {
	if c.World != nil && id == c.World.Player.ID {
		return string(c.World.Player.Name)
	}
	if c.World != nil {
		return string(c.playerName(id))
	}
	return ""
}

func (c *Client) clickTent(x, y int) bool {
	if c.World == nil {
		return false
	}
	// Picking must not depend on whether this session has been drawn yet.
	if c.tentSprites == nil && len(c.housing.Signs) != 0 {
		c.tentSceneSprites()
	}
	cx, cy := c.World.Camera()
	for id, t := range c.tentSprites {
		v, ok := c.housing.Signs[id]
		if ok && t.Hit(int(v.X)-cx, int(v.Y)-cy, x, y) {
			c.pendingNPC = nil
			c.floorPendingPickup = 0
			c.resetGroundHold()
			c.tentPending = id
			if !c.enterNearbyTent() {
				c.walkTo(int(v.X)-cx, int(v.Y)-cy)
			}
			return true
		}
	}
	if game.IsTentMap(c.World.Player.Map) {
		exit := housing.Exit(c.tentKind)
		if abs(x+cx-exit.X) <= tentExitReach && abs(y+cy-exit.Y) <= tentExitReach {
			c.World.StopWalk()
			c.resetGroundHold()
			c.Net.Send([]byte{protocol.CommandTent, protocol.TentExit})
			c.tentLeaving = true
			return true
		}
		if index := c.tentFurnitureAt(x+cx, y+cy); index != 0 {
			if c.items[c.housing.Furniture[index].Item].Definition.TentFloorSwitch {
				c.World.StopWalk()
				c.resetGroundHold()
				c.Net.Send(protocol.Builder{protocol.CommandTentFurniture, protocol.TentFloorSurface}.U16(index))
				return true
			}
			if c.housing.Owner == c.World.Player.ID {
				c.World.StopWalk()
				c.resetGroundHold()
				c.tentDrag = &tentDrag{index: index, start: image.Pt(x+cx, y+cy)}
			}
			return true
		}
	}
	c.tentPending = 0
	return false
}

func (c *Client) enterNearbyTent() bool {
	v, ok := c.housing.Signs[c.tentPending]
	if !ok {
		c.tentPending = 0
		return false
	}
	if abs(c.World.Player.X-int(v.X)) > tentReach || abs(c.World.Player.Y-int(v.Y)) > tentReach {
		return false
	}
	c.World.StopWalk()
	c.World.Marker.Hide()
	c.resetGroundHold()
	c.Net.Send(protocol.Builder{protocol.CommandTent, protocol.TentEnter}.U32(v.Owner))
	c.tentPending = 0
	return true
}

func (c *Client) tentTick() {
	if c.World == nil || !c.mapReady || c.held || c.event.active {
		return
	}
	if c.tentPending != 0 {
		c.enterNearbyTent()
	}
	if game.IsTentMap(c.World.Player.Map) && !c.tentLeaving {
		exit := housing.Exit(c.tentKind)
		if abs(c.World.Player.X-exit.X) < 20 && abs(c.World.Player.Y-exit.Y) < 20 {
			c.World.StopWalk()
			c.Net.Send([]byte{protocol.CommandTent, protocol.TentExit})
			c.tentLeaving = true
		}
	}
}

func tentFootprint(it assets.NativeItem, f housing.Furniture) image.Rectangle {
	w, h := max(1, int(it.Record[tentFurnitureWidthOffset])), max(1, int(it.Record[tentFurnitureHeightOffset]))
	if f.Rotation&1 != 0 {
		w, h = h, w
	}
	return image.Rect(int(f.X), int(f.Y), int(f.X)+w, int(f.Y)+h)
}

func (c *Client) tentPicture(id uint16, rotation byte) int {
	if c.Pics == nil {
		return -1
	}
	it := c.items[id]
	at := tentFurniturePictureOffset
	if rotation&1 != 0 {
		at = tentFurnitureOtherPictureOffset
	}
	resource := binary.LittleEndian.Uint16(it.Record[at:])
	if resource == 0 {
		return -1
	}
	name := strconv.Itoa(int(resource))
	c.loadPictures(name)
	return c.Pics.Find(name)
}

func (c *Client) tentFurnitureImage(f housing.Furniture) (int, image.Rectangle) {
	pic := c.tentPicture(f.Item, f.Rotation)
	if pic < 0 {
		return -1, image.Rectangle{}
	}
	w, h := c.Pics.Size(pic)
	at := housing.Pixel(int(f.X), int(f.Y), int(f.Z))
	// Native TSe_itemObject paint centers the complete image
	// on the projected cell and adds half a tile vertically. Transparent
	// padding is part of that anchor; anchoring its bottom lifts furniture.
	left, top := at.X-w/2, at.Y-h/2+housing.CellHeight/2
	return pic, image.Rect(left, top, left+w, top+h)
}

func (c *Client) rebuildTentScene() {
	if c.World == nil || !game.IsTentMap(c.World.Player.Map) {
		return
	}
	room := housing.Room(c.tentKind)
	var blocked []image.Rectangle
	for _, f := range c.housing.Furniture {
		if f.Floor == c.housing.Floor && f.Parent == 0 {
			blocked = append(blocked, tentFootprint(c.items[f.Item], f))
		}
	}
	c.World.Scene.SetTentWalkGrid(room, blocked)
	c.World.Areas = []world.Area{{Light: world.SmallLight, LightAt: housing.Exit(c.tentKind)}}
	if c.Pics == nil {
		return
	}
	style := tentStyle{c.tentKind, c.housing.FloorSurface, c.housing.Wallpaper}
	if c.tentBackdrop != nil && style == c.tentBackdropStyle {
		c.World.Scene.Layers = []world.Layer{{Image: c.tentBackdrop}}
		return
	}
	c.tentBackdropStyle = style
	if c.tentBackdrop != nil {
		c.tentBackdrop.Close()
	}
	bg := surface.New(housing.SceneWidth, housing.SceneHeight)
	c.tentBackdrop = bg
	base, err := world.TentBackground(c.Assets, c.tentKind)
	if err == nil {
		bg.Draw(0, 0, base, false)
	}
	floor := c.housing.FloorSurface
	if floor == 0 {
		floor = tentDefaultFloorItem
	}
	if pic := c.tentPicture(floor, 0); floor != tentDefaultFloorItem && pic >= 0 {
		overlay := surface.New(bg.W, bg.H)
		defer overlay.Close()
		w, h := c.Pics.Size(pic)
		for x := room.Min.X - tentFloorPatternWidth; x < room.Max.X+tentFloorPatternWidth; x += tentFloorPatternWidth {
			for y := room.Min.Y - tentFloorPatternHeight; y < room.Max.Y+tentFloorPatternHeight; y += tentFloorPatternHeight {
				at := housing.Pixel(x, y, 1)
				c.Pics.Draw(overlay, pic, at.X-w/2, at.Y-h/2, true)
			}
		}
		for y := 0; y < bg.H; y++ {
			for x := 0; x < bg.W; x++ {
				if housing.Cell(x, y).In(room) && overlay.Pix[y*bg.W+x] != 0 {
					bg.Pix[y*bg.W+x] = overlay.Pix[y*bg.W+x]
				}
			}
		}
	}
	wall := c.housing.Wallpaper
	if wall == 0 {
		wall = tentDefaultWallpaperItem
	}
	for side := byte(0); side < 2; side++ {
		if wall == tentDefaultWallpaperItem {
			break
		}
		pic := c.tentPicture(wall, side)
		if pic < 0 {
			continue
		}
		w, h := c.Pics.Size(pic)
		if side == 0 {
			for x := room.Min.X; x < room.Max.X; x++ {
				at := housing.Pixel(x, room.Min.Y, 1)
				c.Pics.Draw(bg, pic, at.X-w/2, at.Y+housing.CellHeight/2-h, true)
			}
		} else {
			for y := room.Min.Y; y < room.Max.Y; y++ {
				at := housing.Pixel(room.Min.X, y, 1)
				c.Pics.Draw(bg, pic, at.X-w/2, at.Y+housing.CellHeight/2-h, true)
			}
		}
	}
	c.World.Scene.Layers = []world.Layer{{Image: bg}}
}

func (c *Client) tentFurnitureAt(x, y int) uint16 {
	var selected uint16
	depth := -1
	for index, f := range c.housing.Furniture {
		if f.Floor != c.housing.Floor || f.Parent != 0 {
			continue
		}
		pic, r := c.tentFurnitureImage(f)
		if pic >= 0 && image.Pt(x, y).In(r) && c.Pics.Opaque(pic, x-r.Min.X, y-r.Min.Y) && (r.Max.Y > depth || r.Max.Y == depth && index > selected) {
			selected = index
			depth = r.Max.Y
		}
	}
	return selected
}

func (c *Client) holdTentFurniture(held bool, x, y int) bool {
	if c.tentDrag == nil {
		return false
	}
	d := c.tentDrag
	if held {
		return true
	}
	c.tentDrag = nil
	if c.World == nil || !c.mapReady || c.held || c.event.active || c.Input.Hovered != nil || c.housing.Owner != c.World.Player.ID {
		return true
	}
	f, ok := c.housing.Furniture[d.index]
	if !ok {
		return true
	}
	x, y = c.World.ScenePoint(x, y)
	cx, cy := c.World.Camera()
	at := image.Pt(x+cx, y+cy)
	if abs(at.X-d.start.X)+abs(at.Y-d.start.Y) < 4 {
		return true
	}
	from := housing.Cell(d.start.X, d.start.Y)
	to := housing.Cell(at.X, at.Y)
	next := f
	next.X = uint32(int(f.X) + to.X - from.X)
	next.Y = uint32(int(f.Y) + to.Y - from.Y)
	if !tentFootprint(c.items[f.Item], next).In(housing.Room(c.tentKind)) {
		return true
	}
	c.Net.Send(protocol.Builder{protocol.CommandTentFurniture, protocol.TentMoveFurniture}.U16(d.index).U32(next.X).U32(next.Y).U32(f.Z).U8(f.Rotation))
	return true
}

func (c *Client) tentFloorPacket(p []byte) {
	if c.World == nil || len(p) != 11 || binary.LittleEndian.Uint32(p) != c.World.Player.ID || !game.IsTentMap(binary.LittleEndian.Uint16(p[4:])) {
		return
	}
	if c.housing.Floor != p[10] {
		if int(p[10]) >= len(c.tentRoomKinds) {
			return
		}
		c.housing.Floor = p[10]
		c.tentKind = c.tentRoomKinds[c.housing.Floor]
		c.rebuildTentScene()
	}
}

// AC12's optional tent tail is a sequence of (room size, floor) pairs.
// The captured packet sets size 1 on floors 0 and 1; these bytes are not flags.
func (c *Client) tentWarpMetadata(p []byte) {
	if len(p) <= tentWarpFloorOffset {
		return
	}
	if floor := p[tentWarpFloorOffset]; int(floor) < len(c.tentRoomKinds) {
		c.housing.Floor = floor
	}
	for at := tentWarpLayoutsOffset; at+1 < len(p); at += 2 {
		kind, floor := p[at], p[at+1]
		if kind >= 1 && kind <= 3 && int(floor) < len(c.tentRoomKinds) {
			c.tentRoomKinds[floor] = kind
		}
	}
	c.tentKind = c.tentRoomKinds[c.housing.Floor]
}

func (c *Client) placeTentFurniture(slot byte, it game.Item, x, y int) bool {
	if c.World == nil || !game.IsTentMap(c.World.Player.Map) || c.housing.Owner != c.World.Player.ID {
		return false
	}
	kind := c.items[it.ID].Definition.Type
	if kind != game.ItemTypeTentFurniture && kind != game.ItemTypeAppliance && kind != game.ItemTypeTentDecoration {
		return false
	}
	if c.InventoryState == nil || slot < 1 || slot > game.BagSize || c.InventoryState.Bag[slot-1] != it || it.Locked {
		return true
	}
	x, y = c.World.ScenePoint(x, y)
	cx, cy := c.World.Camera()
	cell := housing.Cell(x+cx, y+cy)
	if !cell.In(housing.Room(c.tentKind)) {
		return true
	}
	c.Net.Send(protocol.Builder{protocol.CommandTentFurniture, protocol.TentPlaceFurniture, slot}.U32(uint32(cell.X)).U32(uint32(cell.Y)).U32(1).U8(c.housing.Floor))
	return true
}
