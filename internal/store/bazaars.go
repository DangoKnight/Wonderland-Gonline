package store

import (
	"context"
	"database/sql"
	"errors"
	"gorm.io/gorm"
	"math"
	"slices"
	"strings"
	"wonderland-gonline/internal/game"
)

const (
	BazaarNameMaxBytes               = 14
	BazaarTitleMaxBytes              = 64
	BazaarItems               byte   = 1
	BazaarPets                byte   = 2
	BazaarVehicles            byte   = 3
	BazaarVendorFirstID       uint32 = 9000001
	BazaarListingLimit               = 5
	BazaarPetListingLimit            = 15
	BazaarVehicleListingLimit        = 3
	BazaarRangePixels                = 200
)

var ErrBazaarUnavailable = errors.New("bazaar or quoted stock is unavailable")

type Bazaar struct {
	VendorID uint32 `gorm:"primaryKey;autoIncrement:false"`
	Name     string
	Title    string
	Map      uint16
	X, Y     uint16
	Kind     byte
	Infinite bool
	Listings []BazaarListing `gorm:"-"`
}

func (Bazaar) TableName() string { return "bazaar_stalls" }

type BazaarListing struct {
	VendorID uint32 `gorm:"primaryKey;autoIncrement:false"`
	Index    byte   `gorm:"column:listing_index;primaryKey;autoIncrement:false"`
	ItemID   uint16
	Quantity uint32
	Price    uint32
	Infinite bool
}

func (BazaarListing) TableName() string { return "bazaar_listings" }

type BazaarOrder struct {
	Index, Count byte
	ItemID       uint16
	Price        uint32
}

func bazaarLimit(kind byte) int {
	switch kind {
	case BazaarItems:
		return BazaarListingLimit
	case BazaarPets:
		return BazaarPetListingLimit
	case BazaarVehicles:
		return BazaarVehicleListingLimit
	}
	return 0
}

// SaveBazaar is used by authorized server administration. Stock and settings live
// in gameplay SQL; no player inventories or upstream operator rows are imported.
func (s *Store) SaveBazaar(ctx context.Context, b Bazaar) (Bazaar, error) {
	if b.Kind == 0 || bazaarLimit(b.Kind) == 0 || b.Map == 0 || b.Name == "" || len(b.Name) > BazaarNameMaxBytes || b.Title == "" || len(b.Title) > BazaarTitleMaxBytes || len(b.Listings) == 0 || len(b.Listings) > bazaarLimit(b.Kind) {
		return b, ErrBazaarUnavailable
	}
	for _, value := range []byte(b.Name) {
		if value < asciiPrintableMin || value > asciiPrintableMax {
			return b, ErrBazaarUnavailable
		}
	}
	if strings.TrimSpace(b.Title) == "" || strings.ContainsAny(b.Title, "\x00\r\n") {
		return b, ErrBazaarUnavailable
	}
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if b.VendorID == 0 {
			var last uint32
			if err := tx.Model(&Bazaar{}).Select("COALESCE(MAX(vendor_id), ?)", BazaarVendorFirstID-1).Scan(&last).Error; err != nil {
				return err
			}
			if last == math.MaxUint32 {
				return ErrBazaarUnavailable
			}
			b.VendorID = last + 1
		}
		if b.VendorID < BazaarVendorFirstID {
			return ErrBazaarUnavailable
		}
		if err := tx.Omit("Listings").Save(&b).Error; err != nil {
			return err
		}
		if err := tx.Where("vendor_id = ?", b.VendorID).Delete(&BazaarListing{}).Error; err != nil {
			return err
		}
		for i := range b.Listings {
			r := &b.Listings[i]
			r.Index = byte(i)
			r.VendorID = b.VendorID
			if r.ItemID == 0 || r.Quantity == 0 || r.Price == 0 || r.Price > game.MaxGold {
				return ErrBazaarUnavailable
			}
		}
		return tx.Create(&b.Listings).Error
	})
	return b, err
}
func (s *Store) DeleteBazaar(ctx context.Context, id uint32) error {
	return s.transaction(ctx, func(tx *gorm.DB) error { return tx.Delete(&Bazaar{}, id).Error })
}
func loadBazaar(tx *gorm.DB, id uint32) (Bazaar, error) {
	var b Bazaar
	if err := tx.First(&b, id).Error; err != nil {
		return b, err
	}
	err := tx.Where("vendor_id = ?", id).Order("listing_index").Find(&b.Listings).Error
	return b, err
}
func (s *Store) Bazaar(ctx context.Context, id uint32) (Bazaar, error) {
	var b Bazaar
	err := s.transaction(ctx, func(tx *gorm.DB) error { var err error; b, err = loadBazaar(tx, id); return err })
	return b, err
}
func (s *Store) Bazaars(ctx context.Context, mapID uint16) ([]Bazaar, error) {
	var bs []Bazaar
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		q := tx.Order("vendor_id")
		if mapID != 0 {
			q = q.Where("map = ?", mapID)
		}
		if err := q.Find(&bs).Error; err != nil {
			return err
		}
		for i := range bs {
			if err := tx.Where("vendor_id = ?", bs[i].VendorID).Order("listing_index").Find(&bs[i].Listings).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return bs, err
}

// BuyBazaar commits the entire quoted cart, authoritative balance and finite
// stock together. Native pet/vehicle requests use the quote saved on browsing.
func (s *Store) BuyBazaar(ctx context.Context, ref CharacterRef, id uint32, orders []BazaarOrder, items map[uint16]game.ItemDefinition, pets map[uint16]game.PetTemplate, position game.Character) (game.Character, Bazaar, []game.Addition, error) {
	var next game.Character
	var b Bazaar
	var adds []game.Addition
	if len(orders) == 0 || len(orders) > BazaarPetListingLimit {
		return next, b, nil, ErrBazaarUnavailable
	}
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var err error
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if err = preserveItemReservations(&next, []game.Character{position}); err != nil {
			return err
		}
		b, err = loadBazaar(tx, id)
		if err != nil {
			return err
		}
		if position.ID != ref.ID || position.Map != next.Map || position.Map != b.Map || position.TentReturn != nil || math.Hypot(float64(int(position.X)-int(b.X)), float64(int(position.Y)-int(b.Y))) > BazaarRangePixels {
			return ErrBazaarUnavailable
		}
		var total uint64
		for _, order := range orders {
			i := slices.IndexFunc(b.Listings, func(row BazaarListing) bool { return row.Index == order.Index })
			if i < 0 || order.Count == 0 {
				return ErrBazaarUnavailable
			}
			row := &b.Listings[i]
			if row.ItemID != order.ItemID || row.Price != order.Price || !b.Infinite && !row.Infinite && row.Quantity < uint32(order.Count) {
				return ErrBazaarUnavailable
			}
			total += uint64(row.Price) * uint64(order.Count)
			if total > uint64(next.Gold) {
				return game.ErrInvalidItem
			}
			if b.Kind == BazaarPets {
				template, known := pets[row.ItemID]
				if !known || order.Count != 1 || next.FreePetSlot() == 0 {
					return game.ErrInvalidItem
				}
				for _, group := range [][]game.Pet{next.Pets, next.HotelPets, next.ReservePets} {
					for _, pet := range group {
						if game.SamePet(pet.ID, uint32(row.ItemID)) {
							return game.ErrInvalidItem
						}
					}
				}
				pet := game.NewPet(uint32(row.ItemID), "Companion", next.FreePetSlot(), template, items)
				next.Pets = append(next.Pets, pet)
			} else {
				definition, known := items[row.ItemID]
				if !known {
					return game.ErrInvalidItem
				}
				granted, err := next.Bag.Grant(game.Item{ID: row.ItemID}, int(order.Count), definition.StackLimit(), items)
				if err != nil {
					return err
				}
				adds = append(adds, granted...)
			}
			if !b.Infinite && !row.Infinite {
				row.Quantity -= uint32(order.Count)
				if err := tx.Model(&BazaarListing{}).Where("vendor_id = ? AND listing_index = ?", b.VendorID, row.Index).Update("quantity", row.Quantity).Error; err != nil {
					return err
				}
			}
		}
		next.Gold -= uint32(total)
		return saveCharacter(tx, ref, next)
	})
	return next, b, adds, err
}

// migrateBazaars preserves existing owned state while upgrading its SQL schema.
func migrateBazaars(tx *sql.Tx) error {
	if _, e := tx.Exec(`CREATE TABLE IF NOT EXISTS bazaar_stalls(vendor_id INTEGER PRIMARY KEY,name TEXT NOT NULL,title TEXT NOT NULL,map INTEGER NOT NULL,x INTEGER NOT NULL,y INTEGER NOT NULL,kind INTEGER NOT NULL CHECK(kind BETWEEN 1 AND 3),infinite INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS bazaar_listings(vendor_id INTEGER NOT NULL REFERENCES bazaar_stalls(vendor_id) ON DELETE CASCADE,listing_index INTEGER NOT NULL,item_id INTEGER NOT NULL,quantity INTEGER NOT NULL CHECK(quantity>=0),price INTEGER NOT NULL CHECK(price BETWEEN 1 AND 999999),infinite INTEGER NOT NULL,PRIMARY KEY(vendor_id,listing_index));`); e != nil {
		return e
	}
	return nil
}
