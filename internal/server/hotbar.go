package server

import (
	"context"
	"encoding/binary"
	"wonderland-gonline/internal/protocol"
)

func isNativeHotbarAssignment(p []byte) bool {
	return len(p) == protocol.HotbarRequestBytes && p[1] == protocol.HotbarAssign
}

// Native AC40:1 is a presentation preference, not synthesis. aLogin installs
// the binding locally before sending FUN_002aa92c. Accept it without emitting
// the incompatible five-byte legacy alchemy result or modifying resources.
// Durable hotbar storage and login replay are pending; acceptance alone must
// not imply that a new skill was learned or an item was acquired.
func (s *Server) hotbarOrAlchemyCommand(ctx context.Context, c *Session, p []byte) error {
	if !isNativeHotbarAssignment(p) {
		return s.alchemyCommand(ctx, c, p)
	}
	kind, page, slot := p[protocol.HotbarKindOffset], p[protocol.HotbarPageOffset], p[protocol.HotbarSlotOffset]
	if (kind != protocol.HotbarItem && kind != protocol.HotbarSkill) || page < 1 || page > protocol.HotbarPages || slot < 1 || slot > protocol.HotbarSlotsPerPage {
		return protocol.ErrMalformed
	}
	s.Log.Debug("native hotbar assignment accepted", "session", c.info.ID, "character", c.character.ID,
		"kind", kind, "id", binary.LittleEndian.Uint16(p[protocol.HotbarIDOffset:]), "page", page, "slot", slot)
	return nil
}
