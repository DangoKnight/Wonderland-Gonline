package server

import (
	"context"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
)

// mallCheckoutPackets resumes the native cart using the same point snapshot as
// AC75:3/9. Both accepted modes use ordinary IM points, as AC34 does in the source.
func mallCheckoutPackets(balances store.MallBalances) [][]byte {
	resume := protocol.Builder{protocol.CommandCharacterSelection, protocol.CharacterSelectionMallBalance}.U32(mallWireBalance(balances.Points)).U32(0).Bytes(make([]byte, protocol.MallCheckoutReservedBytes))
	return append(mallBalancePackets(balances), resume)
}

// mallCheckoutCommand implements AC34:1 under worldMu. Catalog refreshes clear
// the client's pending cart, so this request must emit only balance/resume packets.
// The read-only query uses the registry's normal world interaction gates.
func (s *Server) mallCheckoutCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.MallCheckoutRequestBytes {
		return protocol.ErrMalformed
	}
	if p[1] != protocol.MallCheckoutBalanceRequest {
		return ErrUnsupported
	}
	switch p[2] {
	case protocol.MallCheckoutModeWireCode0, protocol.MallCheckoutModeWireCode1:
	default:
		return protocol.ErrMalformed
	}
	balances, err := s.Store.MallBalances(ctx, c.account.ID)
	if err != nil {
		return err
	}
	c.account.IM, c.account.IMBonus = balances.Points, balances.Bonus
	return s.sendAll(c, mallCheckoutPackets(balances))
}
