package role

import (
	"time"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/assets"
)

// TJK_RoleImage's layered body (FUN_002566bc), as character creation draws
// it. Unlike the selection screen's renderer it opens each family's base
// archives directly (no patch lookup) and picks its layer order from the
// slot-2 item.

// roleImageArchives are the archive globals FUN_002566bc reads for each
// body type (PTR_DAT_004c9b8c, PTR_DAT_004c9818, …); body type 0 has only
// its base archive.
var roleImageArchives = [5]struct {
	base, head, hat, hat2, body, weapon, arms, shoes string
}{
	{base: "001"},
	{"002", "002h", "002c", "002c_1", "002e", "002w", "002a", "002s"},
	{"003", "003h", "003c", "003c_1", "003e", "003w", "003a", "003s"},
	{"004", "004h", "004c", "004c_1", "004e", "004w", "004a", "004s"},
	{"005", "005h", "005c", "005c_1", "005e", "005w", "005a", "005s"},
}

// roleImageOrders are the layer tables at DAT_004bd79d, 0x4bd7a5, 0x4bd7ad
// and 0x4bd7b5, read from the sixth entry down. Entries are equipment
// slots; 1 also draws the head, 3 draws the head first if it is still
// missing, 6 draws nothing.
var roleImageOrders = [4][6]byte{
	{3, 1, 2, 4, 5, 6},
	{3, 1, 4, 2, 5, 6},
	{3, 1, 5, 2, 4, 6},
	{3, 1, 4, 5, 2, 6},
}

const (
	roleImageHeadOffset = 100  // head sprites follow the base by 100
	hatSecondArchive    = 0xe9 // hat IDs from x233 on are in …c_1
	uncoloredBody       = 1000 // an item sprite of 1000 means none
	roleImageActionBase = 8    // FUN_0042643c(role, 8)
)

// Creator implements login.RolePainter.
type Creator struct {
	Lib   *Library
	Items map[uint16]assets.NativeItem
	Now   func() time.Time
}

func NewCreator(lib *Library, items map[uint16]assets.NativeItem) *Creator {
	return &Creator{Lib: lib, Items: items, Now: time.Now}
}

// SetNow replaces the animation clock (tests).
func (c *Creator) SetNow(now func() time.Time) { c.Now = now }

// EquipSlot is FUN_003cdc98.
func (c *Creator) EquipSlot(id uint16) byte {
	it, ok := c.Items[id]
	if !ok || id == 0 {
		return 0
	}
	return byte(it.Definition.EquipSlot)
}

// directionAction is FUN_0042646c for the standing directions: 0..7 and
// 8..15 both give the direction.
func directionAction(a byte) byte {
	if a >= 8 && a <= 15 {
		return a - 8
	}
	return a
}

// itemFlags is the item's byte at disk offset 0x198 (+0x19c), 0 without
// an item.
func (c *Creator) itemFlags(id uint16) byte {
	if it, ok := c.Items[id]; ok && id != 0 {
		return it.Record[itemFlagsOffset]
	}
	return 0
}

// itemSprite is the item's sprite for a body type (+0x1a + (type−1)·2), 0
// without an item (record 0 of the table is empty).
func (c *Creator) itemSprite(id uint16, body byte) int {
	if it, ok := c.Items[id]; ok && id != 0 && body >= 1 && body <= 4 {
		return int(it.Sprites[body-1])
	}
	return 0
}

// order is FUN_002566bc's table choice from the slot-2 item's flag byte.
func (c *Creator) order(h *login.CreateHuman) int {
	t := c.itemFlags(h.Items[2])
	switch h.Type {
	case 1, 3:
		if t >= 4 && t <= 7 {
			return int(t - 4)
		}
	case 2, 4:
		if t >= 8 && t <= 11 {
			return int(t - 8)
		}
	}
	if t >= 13 && t <= 15 {
		return int(t - 12)
	}
	return 0
}

// sprite draws one sprite of an archive at the human's frame.
func (c *Creator) sprite(dst *surface.Surface, name string, id, action int, h *login.CreateHuman, frame int) {
	a := c.Lib.open(name, firstID(name))
	if a == nil {
		return
	}
	s := a.sprite(id)
	if s == nil {
		return
	}
	n := s.frameCount(action)
	if n == 0 {
		return
	}
	colors := Colors(h.Colors)
	if f := s.frame(action, frame%n); f != nil {
		s.drawColored(dst, f, h.X, h.Y, id, &colors)
	}
}

// PaintBody is FUN_002566bc's sprite part.
func (c *Creator) PaintBody(dst *surface.Surface, h *login.CreateHuman) {
	if h.Type > 4 {
		return
	}
	arcs := roleImageArchives[h.Type]
	base := baseID(h.Type) + int(h.Look)
	action := int(directionAction(h.Action) + roleImageActionBase)
	// The frame advances every 100 ms (FUN_004122ec) and wraps at the base
	// sprite's count.
	if now := c.Now(); now.Sub(h.FrameAt) > frameInterval {
		h.FrameAt = now
		h.Frame++
	}
	if a := c.Lib.open(arcs.base, firstID(arcs.base)); a != nil {
		if s := a.sprite(base); s != nil && s.frameCount(action) > 0 {
			h.Frame %= s.frameCount(action)
		}
	}
	c.sprite(dst, arcs.base, base, action, h, h.Frame)
	if h.Type == 0 {
		return
	}
	headID := baseID(h.Type) + roleImageHeadOffset + int(h.Head)
	headDone := false
	head := func() {
		headDone = true
		c.sprite(dst, arcs.head, headID, action, h, h.Frame)
	}
	order := roleImageOrders[c.order(h)]
	for k := len(order) - 1; k >= 0; k-- {
		slot := order[k]
		if slot < 1 || slot > 5 {
			continue
		}
		id := c.itemSprite(h.Items[slot], h.Type)
		if id == uncoloredBody {
			continue
		}
		switch slot {
		case 1:
			head()
			if id%idsPerFamily < hatSecondArchive {
				c.sprite(dst, arcs.hat, id, action, h, h.Frame)
			} else {
				c.sprite(dst, arcs.hat2, id, action, h, h.Frame)
			}
		case 2:
			c.sprite(dst, arcs.body, id, action, h, h.Frame)
		case 3:
			if id > baseID(h.Type) {
				if !headDone {
					head()
				}
				c.sprite(dst, arcs.weapon, id-1, action, h, h.Frame)
			}
		case 4:
			c.sprite(dst, arcs.arms, id, action, h, h.Frame)
		case 5:
			c.sprite(dst, arcs.shoes, id, action, h, h.Frame)
		}
	}
	if !headDone {
		c.sprite(dst, arcs.head, headID, action, h, h.Frame)
	}
}

// PaintPortrait is FUN_002586c8 for body types 1..4.
func (c *Creator) PaintPortrait(dst *surface.Surface, h *login.CreateHuman, blinking bool) {
	if h.Type == 0 || h.Type > 4 {
		return
	}
	frame := 0
	if blinking {
		frame = 1
	}
	name := familyName(h.Type) + "f"
	c.sprite(dst, name, baseID(h.Type)+portraitSpriteOffset+int(h.Head), portraitAction, h, frame)
}
