package server

import (
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

// publishNPCService hands an EVE interaction to a native form/presentation.
// A native event can arrive with its server-wait flag (+0x7109) set. Merely
// showing a form does not clear that wait: every handoff must send AC20:9.
// This releases the wait, not the interaction; close still owns AC20:6/8.
func (s *Server) publishNPCService(c *Session, packets ...[]byte) error {
	return s.sendAll(c, append(packets, []byte{protocol.CommandEvent, protocol.EventChoice}))
}

// holdNPCService keeps native AC20:6 completion paired with its actual window
// or treatment animation. There is no timeout and no client-specific exception.
// All callers hold worldMu; a service cancellation/warp ends the same token.
func (s *Server) holdNPCService(c *Session, kind uint16) {
	if c.event == nil {
		c.event = &eventSession{mapID: c.character.Map}
	}
	c.event.onChoice = nil
	c.event.serviceOwner, c.event.serviceKind = c.character.ID, kind
}

func (s *Server) clearServiceAuthority(c *Session, kind uint16) {
	switch kind {
	case world.ServicePropsKeeper, world.ServiceStockKeeper:
		c.storageMap = 0
	case world.ServiceWeaponShop, world.ServicePropsShop, world.ServiceArmorShop:
		c.saleMode, c.saleMap = -1, 0
		if c.view != nil {
			c.view.ShopItems = nil
		}
	case world.ServicePetHotel:
		c.hotelMap = 0
	case world.ServiceClinic:
		c.restMap = 0
	}
}

func ownsNPCShop(c *Session) bool {
	es := c.event
	if es == nil || es.shopPicker || es.serviceOwner != c.character.ID || es.mapID != c.character.Map {
		return false
	}
	switch es.serviceKind {
	case world.ServiceWeaponShop, world.ServicePropsShop, world.ServiceArmorShop:
		return true
	}
	return false
}
