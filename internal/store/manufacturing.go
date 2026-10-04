package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

var ErrManufacturingBusy = errors.New("a manufacturing job already exists")
var ErrManufacturingNotDue = errors.New("manufacturing is paused or not complete")

const manufacturedFurnitureCoordinate = 42

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
	var removes []game.Addition
	for _, input := range inputs {
		if input.ItemID == 0 {
			continue
		}
		remaining := int(input.Count)
		for i, item := range c.Bag {
			if item.ID != input.ItemID || item.Empty() || item.Locked || (c.ActiveVehicle != 0 && c.VehicleSlot == byte(i+1)) {
				continue
			}
			take := byte(min(remaining, int(item.Count)))
			if take == 0 {
				continue
			}
			if err := c.Bag.Remove(byte(i+1), take); err != nil {
				return nil, err
			}
			removes = append(removes, game.Addition{Slot: byte(i + 1), Count: take})
			remaining -= int(take)
		}
		if remaining != 0 {
			return nil, game.ErrInvalidItem
		}
	}
	return removes, nil
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
	}
	return false
}
func (s *Store) StartManufacturing(ctx context.Context, ref CharacterRef, bench byte, f assets.ManufacturingFormula, now time.Time, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, *ManufactureJob, []game.Addition, error) {
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
		if next.Map != game.TentMapID {
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
		for _, item := range tent.Items {
			if item.Slot == uint16(bench) && (f.ToolID == 0 || item.ItemID == f.ToolID) {
				validBench = true
				floor = item.Floor
			}
		}
		if bench > 0 && int(bench) <= len(next.Bag) && next.Bag[bench-1].ID == f.ToolID && !next.Bag[bench-1].Empty() && !next.Bag[bench-1].Locked {
			validBench = true
		}
		if !validBench {
			return game.ErrInvalidItem
		}
		removes, err = RemoveManufacturingInputs(&next, f.Inputs[:])
		if err != nil {
			return err
		}
		if f.TentOutput {
			if len(tent.Items)+int(f.Output.Count) > TentItemLimit {
				return game.ErrInventoryFull
			}
		} else {
			probe := next.Bag
			if _, err = probe.Grant(game.Item{ID: f.Output.ItemID}, int(f.Output.Count), items[f.Output.ItemID].StackLimit()); err != nil {
				return err
			}
		}
		job = &ManufactureJob{CharacterID: ref.ID, Bench: bench, Formula: f.ID, ItemID: f.Output.ItemID, Count: f.Output.Count, TentOutput: f.TentOutput, Floor: floor, DueAt: now.Add(time.Duration(f.DurationSeconds) * time.Second).UnixMilli()}
		if err = next.Validate(); err != nil {
			return err
		}
		if err = saveCharacter(tx, ref, next); err != nil {
			return err
		}
		return tx.Create(job).Error
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
		if job.TentOutput {
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
			for i := 0; i < int(job.Count); i++ {
				row := TentItem{OwnerID: ref.ID, Slot: slot + uint16(i), ItemID: job.ItemID, Count: 1, Metadata: make([]byte, game.ItemMetadataBytes), X: manufacturedFurnitureCoordinate, Y: manufacturedFurnitureCoordinate, Floor: job.Floor}
				if err = tx.Create(&row).Error; err != nil {
					return err
				}
			}
		} else {
			adds, err = next.Bag.Grant(game.Item{ID: job.ItemID}, int(job.Count), definition.StackLimit())
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
