package server

import (
	"context"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

func bankBalancePacket(character *game.Character, operation byte) []byte {
	return protocol.Builder{protocol.CommandBank, operation}.U32(character.BankGold).U32(character.Gold)
}

// bankCommand implements AC45:8/9/10 under worldMu. The registry owns loading,
// battle/minigame and trade gates; scripted interactions retain their ownership.
func (s *Server) bankCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	switch p[1] {
	case protocol.BankBalance:
		if len(p) != protocol.BankBalanceRequestBytes {
			return protocol.ErrMalformed
		}
	case protocol.BankDeposit, protocol.BankWithdraw:
		if len(p) != protocol.BankAmountRequestBytes {
			return protocol.ErrMalformed
		}
	case protocol.BankSetPIN, protocol.BankTransfer:
		// These source handlers only acknowledge success; there is no implemented
		// authentication or transfer to port. Never claim those operations succeeded.
	default:
		return ErrUnsupported
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	if p[1] == protocol.BankSetPIN || p[1] == protocol.BankTransfer {
		return c.send(headBanner("Bank PIN changes and character transfers are unavailable."))
	}
	if p[1] == protocol.BankBalance {
		return c.send(bankBalancePacket(c.character, protocol.BankBalance))
	}
	amount := protocol.NewReader(p[2:]).U32()
	next := c.character.Clone()
	changed := false
	if p[1] == protocol.BankDeposit {
		changed = next.DepositGoldToBank(amount)
	} else {
		changed = next.WithdrawGoldFromBank(amount)
	}
	if !changed {
		return s.sendAll(c, [][]byte{headBanner("The requested bank transaction is unavailable."), bankBalancePacket(c.character, protocol.BankBalance)})
	}
	// The existing character transaction saves both balances together. Publish
	// an authoritative wallet balance instead of the reference's pre-save debit.
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	return s.sendAll(c, [][]byte{protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold), bankBalancePacket(&next, p[1])})
}
