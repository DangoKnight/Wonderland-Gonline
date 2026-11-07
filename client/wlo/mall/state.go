// Package mall implements the native Item Mall catalog and shopping cart.
package mall

import (
	"encoding/binary"
	"fmt"
	"math"
	"wonderland-gonline/internal/protocol"
)

const (
	CatalogRows     = 12 // FUN_00244e20.
	CartRows        = protocol.MallCartMaxRows
	MaximumQuantity = 50 // FUN_00243304 editor limit.
)

type Entry struct {
	ID                        uint16
	Count                     byte
	Cost                      uint16
	Discount, Badge, Category byte
	Order                     uint16
}

func (e Entry) Price() uint64 {
	return uint64(math.RoundToEven(float64(e.Cost) * float64(e.Discount) / 100))
}

type Row struct {
	Entry    Entry
	Quantity byte
}
type State struct {
	Catalog                        [2][]Entry
	Balances                       [2]uint32
	Cart                           []Row
	Bonus                          bool
	Enabled                        bool
	WaitingBalance, WaitingReceipt bool
	pending                        []Row
}

func (s *State) Index() int {
	if s.Bonus {
		return 1
	}
	return 0
}
func (s *State) Busy() bool { return s.WaitingBalance || s.WaitingReceipt }
func (s *State) Total() uint64 {
	var total uint64
	for _, row := range s.Cart {
		total += row.Entry.Price() * uint64(row.Quantity)
	}
	return total
}
func (s *State) Reset() { *s = State{} }
func (s *State) Add(e Entry) bool {
	if s.Busy() || len(s.Cart) >= CartRows {
		return false
	}
	// Native FUN_002426e8 rejects duplicate catalog identities.
	for _, r := range s.Cart {
		if r.Entry.ID == e.ID && r.Entry.Category == e.Category && r.Entry.Order == e.Order {
			return false
		}
	}
	s.Cart = append(s.Cart, Row{Entry: e, Quantity: 1})
	return true
}
func (s *State) Quantity(index, amount int) {
	if s.Busy() || index < 0 || index >= len(s.Cart) {
		return
	}
	s.Cart[index].Quantity = byte(max(1, min(amount, MaximumQuantity)))
}
func (s *State) Remove(index int) {
	if !s.Busy() && index >= 0 && index < len(s.Cart) {
		s.Cart = append(s.Cart[:index], s.Cart[index+1:]...)
	}
}
func (s *State) RequestCheckout() []byte {
	if s.Busy() || len(s.Cart) == 0 || !s.Enabled {
		return nil
	}
	if s.Bonus {
		return s.Submit()
	}
	s.WaitingBalance = true
	return []byte{protocol.CommandMallCheckout, protocol.MallCheckoutBalanceRequest, protocol.MallCheckoutModeWireCode1}
}
func (s *State) Submit() []byte {
	if len(s.Cart) == 0 || s.WaitingReceipt || !s.Enabled || s.Total() > uint64(s.Balances[s.Index()]) {
		s.WaitingBalance = false
		return nil
	}
	s.WaitingBalance = false
	sub := byte(protocol.MallBuyPoints)
	if s.Bonus {
		sub = protocol.MallBuyBonus
	}
	p := protocol.Builder{protocol.CommandMall, sub, byte(len(s.Cart))}
	for _, row := range s.Cart {
		p = p.U16(row.Entry.ID).U8(row.Entry.Category).U8(row.Quantity).U16(row.Entry.Order)
	}
	s.pending = append([]Row(nil), s.Cart...)
	s.WaitingReceipt = true
	return p
}

// Apply validates complete frames before changing state. Catalog replacement
// invalidates the cart, while balance-only checkout responses retain it.
func (s *State) Apply(p []byte) error {
	if len(p) < 2 || p[0] != protocol.CommandMall {
		return fmt.Errorf("invalid mall frame")
	}
	switch p[1] {
	case protocol.MallPointsCatalog, protocol.MallBonusCatalog:
		if len(p) < protocol.MallCatalogHeaderBytes {
			return fmt.Errorf("short mall catalog")
		}
		count := int(binary.LittleEndian.Uint16(p[2:]))
		if len(p) != protocol.MallCatalogHeaderBytes+count*protocol.MallCatalogRecordBytes {
			return fmt.Errorf("invalid mall catalog length")
		}
		entries := make([]Entry, 0, count)
		seen := map[[3]uint16]bool{}
		for off := protocol.MallCatalogHeaderBytes; off < len(p); off += protocol.MallCatalogRecordBytes {
			r := p[off : off+protocol.MallCatalogRecordBytes]
			e := Entry{ID: binary.LittleEndian.Uint16(r), Count: r[2], Cost: binary.LittleEndian.Uint16(r[3:]), Discount: r[5], Badge: r[6], Category: r[7], Order: binary.LittleEndian.Uint16(r[8:])}
			key := [3]uint16{e.ID, uint16(e.Category), e.Order}
			if e.ID == 0 || e.Count == 0 || e.Discount > 100 || e.Category < protocol.MallCategoryWeaponry || e.Category > protocol.MallCategoryForging || seen[key] {
				return fmt.Errorf("invalid mall catalog row")
			}
			seen[key] = true
			entries = append(entries, e)
		}
		index := 0
		if p[1] == protocol.MallBonusCatalog {
			index = 1
		}
		s.Catalog[index] = entries
		if index == s.Index() && !s.WaitingReceipt {
			s.Cart = nil
			s.WaitingBalance = false
		}
	case protocol.MallPointsBalance, protocol.MallBonusBalance:
		if len(p) != 13 {
			return fmt.Errorf("invalid mall balance")
		}
		balance := int64(int32(binary.LittleEndian.Uint32(p[2:]))) + int64(int32(binary.LittleEndian.Uint32(p[6:])))
		if balance < 0 || balance > 1<<31-1 {
			return fmt.Errorf("invalid mall point range")
		}
		index := 0
		if p[1] == protocol.MallBonusBalance {
			index = 1
		}
		s.Balances[index] = uint32(balance)
	case protocol.MallCartReceipt:
		if len(p) != 9 || p[8] > protocol.MallPurchaseSucceeded {
			return fmt.Errorf("invalid mall receipt")
		}
		if !s.WaitingReceipt {
			return nil
		}
		id, order := binary.LittleEndian.Uint16(p[2:]), binary.LittleEndian.Uint16(p[6:])
		for i, r := range s.pending {
			if r.Entry.ID == id && r.Entry.Category == p[4] && r.Quantity == p[5] && r.Entry.Order == order {
				if p[8] == protocol.MallPurchaseSucceeded {
					for j, row := range s.Cart {
						if row == r {
							s.Cart = append(s.Cart[:j], s.Cart[j+1:]...)
							break
						}
					}
				}
				s.pending = append(s.pending[:i], s.pending[i+1:]...)
				s.WaitingReceipt = len(s.pending) > 0
				return nil
			}
		}
	case protocol.MallSettings:
		if len(p) != 4 {
			return fmt.Errorf("invalid mall settings")
		}
	case protocol.MallStatus:
		if len(p) != 3 {
			return fmt.Errorf("invalid mall status")
		}
		s.Enabled = p[2] == protocol.MallStatusEnabled
	default:
		return fmt.Errorf("unhandled mall subcode %d", p[1])
	}
	return nil
}

func (s *State) CheckoutBalance(p []byte) bool {
	if len(p) != 18 || p[0] != protocol.CommandCharacterSelection || p[1] != protocol.CharacterSelectionMallBalance {
		return false
	}
	value := int64(int32(binary.LittleEndian.Uint32(p[2:]))) + int64(int32(binary.LittleEndian.Uint32(p[6:])))
	if value < 0 || value > 1<<31-1 {
		return false
	}
	s.Balances[0] = uint32(value)
	return true
}
