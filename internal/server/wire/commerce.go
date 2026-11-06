package wire

import (
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

// ArcadeOutcome is a play's AC71 result. Eggs carry the prize and quantity,
// slots the three reels too. Which fruit-machine reply the original server
// chose is unrecorded; this compatibility rule sends three matching reels
// with a prize as reply 3 (the bingo picture flashes) and anything else as
// reply 4, whose lamp blinks for every prize but index 0.
func ArcadeOutcome(kind byte, reward assets.ArcadeReward) []byte {
	outcome := []byte{protocol.CommandArcadeGame, kind, protocol.ArcadeAccepted, reward.Index}
	switch {
	case kind == protocol.ArcadeEgg || kind == protocol.ArcadeEgg2:
	case assets.ArcadeMario(kind):
		r := reward.Reels
		if reward.Index != 0 && r[0] == r[1] && r[1] == r[2] {
			outcome[2] = protocol.ArcadeMarioJackpot
			outcome = append(outcome, r[:]...)
		} else {
			outcome[2] = protocol.ArcadeMarioPrize
			lamp := byte(0)
			if reward.Index != 0 {
				lamp = protocol.ArcadeMarioLampWin
			}
			outcome = append(append(outcome, r[:]...), lamp)
		}
	default:
		outcome = append(outcome, reward.Reels[:]...)
	}
	return append(outcome, reward.Quantity)
}

func BankBalancePacket(character *game.Character, operation byte) []byte {
	return protocol.Builder{protocol.CommandBank, operation}.U32(character.BankGold).U32(character.Gold)
}

// MallWireBalance uses the native signed point range for every balance reply.
func MallWireBalance(value int64) uint32 {
	return uint32(max(0, min(value, int64(game.MaxMallPoints))))
}

func MallBalancePackets(balances store.MallBalances) [][]byte {
	packet := func(sub byte, value int64) []byte {
		return protocol.Builder{protocol.CommandMall, sub}.U32(MallWireBalance(value)).U32(0).U16(0).U8(0)
	}
	return [][]byte{packet(protocol.MallPointsBalance, balances.Points), packet(protocol.MallBonusBalance, balances.Bonus)}
}

// MallCheckoutPackets resumes the native cart using the same point snapshot as
// AC75:3/9. Both accepted modes use ordinary IM points, as AC34 does in the source.
func MallCheckoutPackets(balances store.MallBalances) [][]byte {
	resume := protocol.Builder{protocol.CommandCharacterSelection, protocol.CharacterSelectionMallBalance}.U32(MallWireBalance(balances.Points)).U32(0).Bytes(make([]byte, protocol.MallCheckoutReservedBytes))
	return append(MallBalancePackets(balances), resume)
}

// TradeMessage is a notice shared by the trading interactions (stalls,
// manufacturing, social commands).
func TradeMessage(message string) []byte {
	p, _ := protocol.Builder{protocol.CommandInventory, protocol.InventoryMessage, 0}.String(message)
	return p
}

func TradeEnded(result byte) []byte {
	return []byte{protocol.CommandTrade, protocol.TradeEnded, result}
}

// TradeOfferPacket is AC25:3 as FUN_003c704c reads it: the gold, then per
// item its ID, count and damage and the bag record's 26 metadata bytes with
// the u32/u16 pair (bytes 2..7) ahead of bytes 0..1.
func TradeOfferPacket(offer game.TradeOffer) []byte {
	p := protocol.Builder{protocol.CommandTrade, protocol.TradePartnerOffer}.U32(offer.Gold)
	for _, it := range offer.Items {
		m := it.Item.Metadata
		p = p.U16(it.Item.ID).U8(it.Count).U8(it.Item.Damage).Bytes(m[2:8]).Bytes(m[0:2]).Bytes(m[8:])
	}
	return p
}
