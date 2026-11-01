package server

import (
	"context"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

// The reference server's alternative AC63:3 creation layout shares its subcommand with the
// native two-byte cancellation. Keep cancellation intact and validate the longer
// authenticated request against the same allocation/name/ownership rules as AC9.
func (s *Server) createCompactCharacter(ctx context.Context, c *Session, p []byte) error {
	if c.account.ID == 0 || c.character != nil {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	slot, name := r.U8(), r.String()
	appearance := game.Appearance{Body: uint16(r.U8()), Element: r.U8()}
	appearance.Base = game.Attributes{Strength: uint16(r.U8()), Constitution: uint16(r.U8()), Intelligence: uint16(r.U8()), Wisdom: uint16(r.U8()), Agility: uint16(r.U8())}
	if r.Err() != nil || r.Remaining() != 0 || slot < 1 || slot > 2 {
		return protocol.ErrMalformed
	}
	fail := func() error {
		return c.send([]byte{protocol.CommandHandshake, protocol.HandshakeCompactCreationResult, protocol.CompactCreationFailed})
	}
	if game.ValidateCharacterName(name) != nil || appearance.Validate() != nil || appearance.ValidateCreationAllocation() != nil {
		return fail()
	}
	s.catalogMu.RLock()
	defer s.catalogMu.RUnlock()
	if len(s.Assets.StarterItems) == 0 || len(s.Assets.Items) == 0 {
		return fail()
	}
	c.slot = slot
	available, err := s.reserveName(ctx, c, name)
	if err != nil {
		return err
	}
	if !available {
		return fail()
	}
	defer s.releaseName(c)
	char, err := game.NewCharacter(c.account.CharacterID(slot), slot, name, appearance, s.Assets.StarterItems, s.Assets.Items, time.Now())
	if err != nil {
		return fail()
	}
	s.unlockQualifiedSkills(&char, false)
	view, pets := world.NewView(), newPetRoster()
	packets, err := s.worldEntryPackets(char, view, pets)
	if err != nil {
		return fail()
	}
	if err = s.Store.CreateCharacterWithCode(ctx, c.account, char, ""); err != nil {
		return fail()
	}
	if err = c.send([]byte{protocol.CommandHandshake, protocol.HandshakeCompactCreationResult, protocol.CompactCreationSucceeded}); err != nil {
		return err
	}
	return s.enterWorld(c, char, view, pets, packets)
}
