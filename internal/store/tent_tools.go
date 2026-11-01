package store

import (
	"context"
	"database/sql"
	"gorm.io/gorm"
	"slices"
	"wonderland-gonline/internal/game"
)

type TentTool struct {
	OwnerID  uint32 `gorm:"primaryKey;autoIncrement:false"`
	HostSlot uint16 `gorm:"primaryKey;autoIncrement:false"`
	Ordinal  byte   `gorm:"primaryKey;autoIncrement:false"`
	ItemID   uint16
	Damage   byte
	Metadata []byte
}

func (TentTool) TableName() string { return "tent_tools" }

// AttachTentTool consumes an authoritative unlocked appliance item and saves its
// complete instance as a child of an owned host in the same transaction.
func (s *Store) AttachTentTool(ctx context.Context, ref CharacterRef, bagSlot byte, hostIndex uint16, hosts []uint16, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, Tent, error) {
	var next game.Character
	var tent Tent
	if bagSlot < 1 || bagSlot > game.BagSize {
		return next, tent, game.ErrInvalidItem
	}
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var err error
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if err = preserveItemReservations(&next, reservations); err != nil {
			return err
		}
		if next.Map != game.TentMapID || next.TentReturn == nil {
			return game.ErrInvalidItem
		}
		tent, err = loadTent(tx, ref.ID)
		if err != nil {
			return err
		}
		if tentTreeNodes(tent) >= maxTentTreeNodes {
			return game.ErrInventoryFull
		}
		index := nativeTentIndex(hostIndex)
		if index >= len(tent.Items) {
			return game.ErrInvalidItem
		}
		host := &tent.Items[index]
		item := next.Bag[bagSlot-1]
		definition, known := items[item.ID]
		if !slices.Contains(hosts, host.ItemID) || item.Empty() || item.Locked || !known || definition.Type != game.ItemTypeAppliance || len(host.Tools) >= game.MaxTentToolAttachments {
			return game.ErrInvalidItem
		}
		for _, tool := range host.Tools {
			if tool.ItemID == item.ID {
				return game.ErrInvalidItem
			}
		}
		tool := TentTool{OwnerID: ref.ID, HostSlot: host.Slot, Ordinal: byte(len(host.Tools)), ItemID: item.ID, Damage: item.Damage, Metadata: append([]byte(nil), item.Metadata[:]...)}
		if err = next.Bag.Remove(bagSlot, 1); err != nil {
			return err
		}
		if err = tx.Create(&tool).Error; err != nil {
			return err
		}
		host.Tools = append(host.Tools, tool)
		return saveCharacter(tx, ref, next)
	})
	return next, tent, err
}

// migrateTentTools preserves existing owned state while upgrading its SQL schema.
func migrateTentTools(tx *sql.Tx) error {
	if _, e := tx.Exec(`CREATE TABLE IF NOT EXISTS tent_tools (
 owner_id INTEGER NOT NULL,
 host_slot INTEGER NOT NULL,
 ordinal INTEGER NOT NULL CHECK(ordinal BETWEEN 0 AND 9),
 item_id INTEGER NOT NULL,
 damage INTEGER NOT NULL,
 metadata BLOB NOT NULL CHECK(length(metadata)=26),
 PRIMARY KEY(owner_id,host_slot,ordinal),
 UNIQUE(owner_id,host_slot,item_id),
 FOREIGN KEY(owner_id,host_slot) REFERENCES tent_items(owner_id,slot) ON DELETE CASCADE);`); e != nil {
		return e
	}
	return nil
}
