package store

import (
	"context"
	"database/sql"
	"errors"
	"gorm.io/gorm"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

var ErrManufacturingBusy = errors.New("a manufacturing job already exists")
var ErrManufacturingNotDue = errors.New("manufacturing is paused or not complete")

const maxManufacturingOutputCount = 255

type ManufactureJob struct {
	CharacterID     uint32 `gorm:"primaryKey;autoIncrement:false"`
	Bench           byte
	Formula         uint16
	ItemID          uint16
	Count           byte
	TentOutput      bool
	Floor           byte
	DueAt           int64
	RemainingMillis int64
	Paused          bool
	Native          bool
	Batch           byte
	Surface         byte
}

func (ManufactureJob) TableName() string { return "manufacture_jobs" }
func (s *Store) ManufacturingJob(ctx context.Context, ref CharacterRef) (*ManufactureJob, error) {
	var result *ManufactureJob
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if _, err := loadCharacter(tx, ref); err != nil {
			return err
		}
		var job ManufactureJob
		err := tx.First(&job, ref.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		result = &job
		return nil
	})
	return result, err
}

// RemoveManufacturingInputs aggregates repeated material IDs and avoids reserved
// items/active vehicles. Called only against an authoritative SQL snapshot.
func RemoveManufacturingInputs(c *game.Character, inputs []assets.ManufacturingInput) ([]game.Addition, error) {
	removes, _, err := removeManufacturingBatch(c, inputs, 1)
	return removes, err
}

type ManufactureInput struct {
	CharacterID uint32 `gorm:"primaryKey;autoIncrement:false"`
	Ordinal     uint16 `gorm:"primaryKey;autoIncrement:false"`
	ItemID      uint16
	Count       byte
	Damage      byte
	Metadata    []byte
}

func (ManufactureInput) TableName() string { return "manufacture_inputs" }

func removeManufacturingBatch(c *game.Character, inputs []assets.ManufacturingInput, batch byte) ([]game.Addition, []ManufactureInput, error) {
	var removes []game.Addition
	var refunds []ManufactureInput
	for _, input := range inputs {
		if input.ItemID == 0 {
			continue
		}
		remaining := int(input.Count) * int(batch)
		for i, item := range c.Bag {
			if item.ID != input.ItemID || item.Empty() || item.Locked || c.ActiveVehicle != 0 && c.VehicleSlot == byte(i+1) {
				continue
			}
			take := byte(min(remaining, int(item.Count)))
			if take == 0 {
				continue
			}
			if err := c.Bag.Remove(byte(i+1), take); err != nil {
				return nil, nil, err
			}
			removes = append(removes, game.Addition{Slot: byte(i + 1), Count: take})
			refunds = append(refunds, ManufactureInput{CharacterID: c.ID, Ordinal: uint16(len(refunds)), ItemID: item.ID, Count: take, Damage: item.Damage, Metadata: append([]byte(nil), item.Metadata[:]...)})
			remaining -= int(take)
		}
		if remaining != 0 {
			return nil, nil, game.ErrInvalidItem
		}
	}
	return removes, refunds, nil
}

func manufactureRequirement(c game.Character, tent Tent, id uint16) bool {
	if id == 0 {
		return true
	}
	for _, item := range c.Bag {
		if item.ID == id && !item.Empty() {
			return true
		}
	}
	for _, item := range tent.Items {
		if item.ItemID == id {
			return true
		}
		for _, tool := range item.Tools {
			if tool.ItemID == id {
				return true
			}
		}
	}
	return false
}
func (s *Store) StartManufacturing(ctx context.Context, ref CharacterRef, bench byte, f assets.ManufacturingFormula, now time.Time, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, *ManufactureJob, []game.Addition, error) {
	return s.startManufacturing(ctx, ref, bench, f, 1, false, 0, now, items, reservations...)
}

func (s *Store) StartNativeManufacturing(ctx context.Context, ref CharacterRef, bench uint16, f assets.ManufacturingFormula, batch, surface byte, now time.Time, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, *ManufactureJob, []game.Addition, error) {
	if bench > maxManufacturingOutputCount {
		return game.Character{}, nil, nil, game.ErrInvalidItem
	}
	if batch == 0 {
		batch = 1
	}
	return s.startManufacturing(ctx, ref, byte(bench), f, batch, true, surface, now, items, reservations...)
}
func (s *Store) startManufacturing(ctx context.Context, ref CharacterRef, bench byte, f assets.ManufacturingFormula, batch byte, native bool, surface byte, now time.Time, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, *ManufactureJob, []game.Addition, error) {
	if int(f.Output.Count)*int(batch) > maxManufacturingOutputCount || uint64(f.DurationSeconds)*uint64(batch) > assets.ManufacturingMaxDurationSeconds || surface != 0 && surface != assets.TentSurfaceFloor && surface != assets.TentSurfaceWallpaper {
		return game.Character{}, nil, nil, game.ErrInvalidItem
	}
	var next game.Character
	var job *ManufactureJob
	var removes []game.Addition
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var err error
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if err = preserveItemReservations(&next, reservations); err != nil {
			return err
		}
		if err = assets.ValidateManufacturing(map[uint16]assets.ManufacturingFormula{f.ID: f}, items); err != nil {
			return err
		}
		if !game.IsTentMap(next.Map) || native && next.TentReturn == nil {
			return game.ErrInvalidItem
		}
		var count int64
		if err = tx.Model(&ManufactureJob{}).Where("character_id = ?", ref.ID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrManufacturingBusy
		}
		tent, err := loadTent(tx, ref.ID)
		if err != nil {
			return err
		}
		if !manufactureRequirement(next, tent, f.PlanID) || !manufactureRequirement(next, tent, f.ToolID) {
			return game.ErrInvalidItem
		}
		validBench := f.ToolID == 0
		var floor byte
		for index, item := range tent.Items {
			toolPresent := f.ToolID == 0 || item.ItemID == f.ToolID
			for _, tool := range item.Tools {
				toolPresent = toolPresent || tool.ItemID == f.ToolID
			}
			if ((!native && item.Slot == uint16(bench)) || (native && nativeTentIndex(uint16(bench)) == index)) && toolPresent {
				validBench = true
				floor = item.Floor
			}
		}
		if !native && bench > 0 && int(bench) <= len(next.Bag) && next.Bag[bench-1].ID == f.ToolID && !next.Bag[bench-1].Empty() && !next.Bag[bench-1].Locked {
			validBench = true
		}
		if !validBench {
			return game.ErrInvalidItem
		}
		var refunds []ManufactureInput
		removes, refunds, err = removeManufacturingBatch(&next, f.Inputs[:], batch)
		if err != nil {
			return err
		}
		outputCount := int(f.Output.Count) * int(batch)
		if f.TentOutput && surface == 0 {
			if len(tent.Items)+outputCount > TentItemLimit || tentTreeNodes(tent)+outputCount > maxTentTreeNodes {
				return game.ErrInventoryFull
			}
		} else if outputCount > boolCount(surface != 0) {
			probe := next.Bag
			if _, err = probe.Grant(game.Item{ID: f.Output.ItemID}, outputCount-boolCount(surface != 0), items[f.Output.ItemID].StackLimit(), items); err != nil {
				return err
			}
		}
		job = &ManufactureJob{CharacterID: ref.ID, Bench: bench, Formula: f.ID, ItemID: f.Output.ItemID, Count: byte(outputCount), TentOutput: f.TentOutput && surface == 0, Floor: floor, Native: native, Batch: batch, Surface: surface, DueAt: now.Add(time.Duration(f.DurationSeconds) * time.Duration(batch) * time.Second).UnixMilli()}
		if err = next.Validate(); err != nil {
			return err
		}
		if err = saveCharacter(tx, ref, next); err != nil {
			return err
		}
		if err = tx.Create(job).Error; err != nil {
			return err
		}
		if len(refunds) > 0 {
			return tx.Create(&refunds).Error
		}
		return nil
	})
	return next, job, removes, err
}
func (s *Store) PauseManufacturing(ctx context.Context, ref CharacterRef, bench byte, pause bool, now time.Time) (*ManufactureJob, error) {
	var job ManufactureJob
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if _, err := loadCharacter(tx, ref); err != nil {
			return err
		}
		if err := tx.First(&job, ref.ID).Error; err != nil {
			return err
		}
		if job.Bench != bench {
			return game.ErrInvalidItem
		}
		if pause && !job.Paused {
			job.RemainingMillis = max(int64(0), job.DueAt-now.UnixMilli())
			job.Paused = true
		}
		if !pause && job.Paused {
			job.DueAt = now.UnixMilli() + job.RemainingMillis
			job.RemainingMillis = 0
			job.Paused = false
		}
		return tx.Save(&job).Error
	})
	return &job, err
}
func (s *Store) CompleteManufacturing(ctx context.Context, ref CharacterRef, now time.Time, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, *ManufactureJob, []game.Addition, error) {
	var next game.Character
	var job ManufactureJob
	var adds []game.Addition
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var err error
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if err = preserveItemReservations(&next, reservations); err != nil {
			return err
		}
		if err = tx.First(&job, ref.ID).Error; err != nil {
			return err
		}
		if job.Paused || now.UnixMilli() < job.DueAt {
			return ErrManufacturingNotDue
		}
		definition, known := items[job.ItemID]
		if !known {
			return game.ErrInvalidItem
		}
		if job.Surface != 0 {
			tent, err := loadTent(tx, ref.ID)
			if err != nil {
				return err
			}
			if job.Surface == assets.TentSurfaceWallpaper {
				tent.Wallpaper = job.ItemID
			} else {
				tent.Floor = job.ItemID
			}
			if err = tx.Omit("Items").Save(&tent).Error; err != nil {
				return err
			}
			if job.Count > 1 {
				adds, err = next.Bag.Grant(game.Item{ID: job.ItemID}, int(job.Count)-1, definition.StackLimit(), items)
				if err != nil {
					return err
				}
			}
		} else if job.TentOutput {
			tent, err := loadTent(tx, ref.ID)
			if err != nil {
				return err
			}
			var slot uint16
			if len(tent.Items) > 0 {
				last := tent.Items[len(tent.Items)-1].Slot
				if int(last)+int(job.Count) > TentItemLimit {
					return game.ErrInventoryFull
				}
				slot = last + 1
			}
			if len(tent.Items)+int(job.Count) > TentItemLimit {
				return game.ErrInventoryFull
			}
			if tentTreeNodes(tent)+int(job.Count) > maxTentTreeNodes {
				return game.ErrInventoryFull
			}
			for i := 0; i < int(job.Count); i++ {
				row, found := findTentFurnitureSpot(tent, TentItem{OwnerID: ref.ID, Slot: slot + uint16(i), ItemID: job.ItemID, Count: 1, Metadata: make([]byte, game.ItemMetadataBytes), Floor: job.Floor}, items)
				if !found {
					return game.ErrInventoryFull
				}
				tent.Items = append(tent.Items, row)
				if err = tx.Create(&row).Error; err != nil {
					return err
				}
			}
		} else {
			adds, err = next.Bag.Grant(game.Item{ID: job.ItemID}, int(job.Count), definition.StackLimit(), items)
			if err != nil {
				return err
			}
		}
		if err = next.Validate(); err != nil {
			return err
		}
		if err = saveCharacter(tx, ref, next); err != nil {
			return err
		}
		return tx.Delete(&job).Error
	})
	return next, &job, adds, err
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

// CancelNativeManufacturing restores every consumed instance and removes the job
// atomically. If the bag cannot hold all refunds the recoverable job remains.
func (s *Store) CancelNativeManufacturing(ctx context.Context, ref CharacterRef, bench byte, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, []game.Addition, error) {
	var next game.Character
	var adds []game.Addition
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var err error
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if err = preserveItemReservations(&next, reservations); err != nil {
			return err
		}
		var job ManufactureJob
		if err = tx.First(&job, ref.ID).Error; err != nil {
			return err
		}
		if !job.Native || job.Bench != bench {
			return game.ErrInvalidItem
		}
		var inputs []ManufactureInput
		if err = tx.Where("character_id = ?", ref.ID).Order("ordinal").Find(&inputs).Error; err != nil {
			return err
		}
		for _, input := range inputs {
			definition, known := items[input.ItemID]
			if !known {
				return game.ErrInvalidItem
			}
			item := game.Item{ID: input.ItemID, Damage: input.Damage}
			copy(item.Metadata[:], input.Metadata)
			granted, err := next.Bag.Grant(item, int(input.Count), definition.StackLimit(), items)
			if err != nil {
				return err
			}
			adds = append(adds, granted...)
		}
		if err = saveCharacter(tx, ref, next); err != nil {
			return err
		}
		return tx.Delete(&job).Error
	})
	return next, adds, err
}

// migrateManufacturing preserves existing owned state while upgrading its SQL schema.
func migrateManufacturing(tx *sql.Tx) error {
	if _, e := tx.Exec(`CREATE TABLE IF NOT EXISTS manufacture_jobs(character_id INTEGER PRIMARY KEY REFERENCES characters(id) ON DELETE CASCADE,bench INTEGER NOT NULL,formula INTEGER NOT NULL,item_id INTEGER NOT NULL,count INTEGER NOT NULL,tent_output INTEGER NOT NULL,floor INTEGER NOT NULL,due_at INTEGER NOT NULL,remaining_millis INTEGER NOT NULL,paused INTEGER NOT NULL);`); e != nil {
		return e
	}
	for _, column := range []string{"native", "batch", "surface"} {
		var present int
		if e := tx.QueryRow("SELECT count(*) FROM pragma_table_info('manufacture_jobs') WHERE name=?", column).Scan(&present); e != nil {
			return e
		}
		if present == 0 {
			if _, e := tx.Exec("ALTER TABLE manufacture_jobs ADD COLUMN " + column + " INTEGER NOT NULL DEFAULT 0"); e != nil {
				return e
			}
		}
	}
	if _, e := tx.Exec(`CREATE TABLE IF NOT EXISTS manufacture_inputs (
 character_id INTEGER NOT NULL REFERENCES manufacture_jobs(character_id) ON DELETE CASCADE,
 ordinal INTEGER NOT NULL, item_id INTEGER NOT NULL, count INTEGER NOT NULL CHECK(count BETWEEN 1 AND 255),
 damage INTEGER NOT NULL,metadata BLOB NOT NULL CHECK(length(metadata)=26), PRIMARY KEY(character_id,ordinal));`); e != nil {
		return e
	}
	return nil
}
