package server

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/store"
)

var errNPCResourceConflict = errors.New("NPC service resources changed; reopen the service")

// Removing the last pet leaves an empty slice in memory; SQL reloads it as nil.
// Compare roster contents, while retaining exact checks on every pet field.
func equalNPCPetResources(a, b []game.Pet) bool {
	return slices.EqualFunc(a, b, func(a, b game.Pet) bool { return reflect.DeepEqual(a, b) })
}

// Commit only the resources changed by this operation, after checking their
// owned SQL values. Adoption retains unrelated buffered walking and recovery.
func (s *Server) commitNPCResources(ctx context.Context, c *Session, next game.Character) error {
	before := c.character.Clone()
	saved, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: before.ID}, func(current *game.Character) error {
		if before.Gold != next.Gold {
			if current.Gold != before.Gold {
				return errNPCResourceConflict
			}
			current.Gold = next.Gold
		}
		if before.Bag != next.Bag {
			if current.Bag != before.Bag {
				return errNPCResourceConflict
			}
			current.Bag = next.Bag
		}
		if before.Storage != next.Storage {
			if current.Storage != before.Storage {
				return errNPCResourceConflict
			}
			current.Storage = next.Storage
		}
		if !equalNPCPetResources(before.Pets, next.Pets) || !equalNPCPetResources(before.HotelPets, next.HotelPets) {
			expected := before
			if c.autosaveBaseline != nil {
				expected = *c.autosaveBaseline
			}
			if !equalNPCPetResources(current.Pets, expected.Pets) || !equalNPCPetResources(current.HotelPets, expected.HotelPets) {
				return errNPCResourceConflict
			}
			current.Pets, current.HotelPets = next.Pets, next.HotelPets
			current.ActivePet, current.ActiveMount = next.ActivePet, next.ActiveMount
		}
		return nil
	}, before)
	if err != nil {
		return err
	}
	s.adoptSavedCharacter(c, saved)
	return nil
}

// Native item questions authorize only their own listed slots while awaiting
// the cart. Ordinary dialogue events do not acquire purchase authority.
func (s *Server) authorizeShopQuestion(c *Session, es *eventSession, id uint16) {
	es.shopCart = false
	for _, g := range s.Assets.Maps[es.mapID].Interactive {
		if g.ClickID != id || len(g.Header) == 0 || (g.Header[0] != npcShopWeaponsKind && g.Header[0] != npcShopPropsKind) {
			continue
		}
		items := map[uint16]uint16{}
		for at := 0; at+5 <= len(g.Members); at += 5 {
			r := g.Members[at:]
			if r[0] != 0 && r[3] == npcShopItemKind && r[4] == npcShopOptionType {
				id := uint16(r[1]) | uint16(r[2])<<8
				if _, known := s.Assets.Items[id]; known {
					items[uint16(r[0])] = id
				}
			}
		}
		if len(items) > 0 {
			c.view.ShopItems = items
			c.saleMap = c.character.Map
			es.shopCart = true
		}
		return
	}
}
