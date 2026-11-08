package protocol

import "encoding/binary"

// Native TGiftExchangeCardForm / Gec.Dat catalog layout.
const (
	GiftCardIndex              byte = 1
	GiftCardDetails            byte = 2
	GiftCardRedeemed           byte = 3
	GiftCardOpen               byte = 4
	GiftCardVersion            byte = 1
	GiftCardCatalogRequest     byte = 90 // Shares AC90 with the two-byte fishing commands.
	GiftCardCatalogMode        byte = 1  // Native AC90:1 payload prefix before the UInt16 count.
	GiftCardCatalogHeaderBytes      = 5
	GiftCardCatalogIDBytes          = 2
)

type GiftCardReward struct {
	ItemID   uint16
	Quantity byte
}
type GiftCard struct {
	ID      uint16
	Version byte
	Rewards []GiftCardReward
}

func GiftCardDetailsPacket(cards []GiftCard) []byte {
	p := Builder{CommandInventory, InventoryGiftCardReply, GiftCardDetails}.U16(uint16(len(cards)))
	for _, c := range cards {
		p = p.U16(c.ID).U8(c.Version).U8(byte(len(c.Rewards)))
		for _, r := range c.Rewards {
			p = p.U16(r.ItemID).U8(r.Quantity)
		}
	}
	return p
}

func ParseGiftCardDetails(p []byte) ([]GiftCard, bool) {
	if len(p) < 5 || p[0] != CommandInventory || p[1] != InventoryGiftCardReply || p[2] != GiftCardDetails {
		return nil, false
	}
	count := int(binary.LittleEndian.Uint16(p[3:5]))
	p = p[5:]
	cards := make([]GiftCard, 0, count)
	for i := 0; i < count; i++ {
		if len(p) < 4 {
			return nil, false
		}
		c := GiftCard{ID: binary.LittleEndian.Uint16(p), Version: p[2]}
		n := int(p[3])
		p = p[4:]
		if n == 0 || len(p) < 3*n {
			return nil, false
		}
		for j := 0; j < n; j++ {
			if p[2] == 0 {
				return nil, false
			}
			c.Rewards = append(c.Rewards, GiftCardReward{binary.LittleEndian.Uint16(p), p[2]})
			p = p[3:]
		}
		cards = append(cards, c)
	}
	return cards, len(p) == 0
}
