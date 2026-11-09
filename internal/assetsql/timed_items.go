package assetsql

import (
	_ "embed"
	"encoding/csv"
	"fmt"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

// Durations follow the imported descriptions and administrator policy: Training
// Ticket 34253 grants six hours despite omitting duration in its description.
// Training Fare explicitly grants one hour. See docs/GROWTH_ITEMS_REFERENCE.md.
//
//go:embed timed_items_defaults.csv
var timedItemDefaults string

func defaultTimedItems() (map[uint16]game.ItemDefinition, error) {
	rows, e := csv.NewReader(strings.NewReader(timedItemDefaults)).ReadAll()
	if e != nil {
		return nil, e
	}
	if len(rows) < 2 || strings.Join(rows[0], ",") != "item_id,kind,minutes,exp_tenths" {
		return nil, fmt.Errorf("invalid timed item header")
	}
	out := map[uint16]game.ItemDefinition{}
	for _, row := range rows[1:] {
		if len(row) != 4 {
			return nil, fmt.Errorf("invalid timed item row")
		}
		var n [4]uint64
		for i, s := range row {
			n[i], e = strconv.ParseUint(s, 10, 16)
			if e != nil {
				return nil, e
			}
		}
		id := uint16(n[0])
		if _, ok := out[id]; id == 0 || ok || n[1] < 1 || n[1] > 4 {
			return nil, fmt.Errorf("invalid timed item %d", id)
		}
		out[id] = game.ItemDefinition{ID: id, TimedKind: game.TimedItemKind(n[1]), TimedMinutes: uint32(n[2]), EXPTenths: uint16(n[3])}
	}
	return out, nil
}
func addTimedItemColumns(tx *gorm.DB) error {
	for _, r := range []struct {
		model  any
		fields []string
	}{
		{&NativeItemsRow{}, []string{"ValueDefinitionTimedKind", "ValueDefinitionTimedMinutes", "ValueDefinitionEXPTenths"}},
		{&MapsRow{}, []string{"ValueTrainingArea", "ValueTrainingEntryX", "ValueTrainingEntryY"}},
	} {
		if !tx.Migrator().HasTable(r.model) {
			continue
		}
		for _, f := range r.fields {
			if !tx.Migrator().HasColumn(r.model, f) {
				if e := tx.Migrator().AddColumn(r.model, f); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
func seedTimedItems(c *assets.Catalog) error {
	defs, e := defaultTimedItems()
	if e != nil {
		return e
	}
	for id, d := range defs {
		if item, ok := c.NativeItems[id]; ok {
			item.Definition.TimedKind = d.TimedKind
			item.Definition.TimedMinutes = d.TimedMinutes
			item.Definition.EXPTenths = d.EXPTenths
			c.NativeItems[id] = item
			c.Items[id] = item.Definition
		}
	}
	areas, e := defaultTrainingAreas()
	if e != nil {
		return e
	}
	for id := range areas {
		if m, ok := c.Maps[id]; ok {
			m.TrainingArea = true
			c.Maps[id] = m
		}
	}
	if m, ok := c.Maps[trainingIslandEntryMap]; ok {
		m.TrainingEntryX = trainingIslandEntryX
		m.TrainingEntryY = trainingIslandEntryY
		c.Maps[trainingIslandEntryMap] = m
	}
	return nil
}

// Captured AC12 destination for the native training-control entry request.
const (
	trainingIslandEntryMap uint16 = 13050
	trainingIslandEntryX   uint16 = 2632
	trainingIslandEntryY   uint16 = 1344
)

func migrateTimedItems(tx *gorm.DB) error {
	defs, e := defaultTimedItems()
	if e != nil {
		return e
	}
	if tx.Migrator().HasTable(&NativeItemsRow{}) {
		for id, d := range defs {
			if e = tx.Model(&NativeItemsRow{}).Where("native_items_key = ?", id).Updates(map[string]any{"value_definition_timed_kind": d.TimedKind, "value_definition_timed_minutes": d.TimedMinutes, "value_definition_exp_tenths": d.EXPTenths}).Error; e != nil {
				return e
			}
		}
	}
	if !tx.Migrator().HasTable(&MapsRow{}) {
		return nil
	}
	areas, e := defaultTrainingAreas()
	if e != nil {
		return e
	}
	for id := range areas {
		if e = tx.Model(&MapsRow{}).Where("maps_key = ?", id).Update("value_training_area", true).Error; e != nil {
			return e
		}
	}
	return tx.Model(&MapsRow{}).Where("maps_key = ?", trainingIslandEntryMap).Updates(map[string]any{"value_training_entry_x": trainingIslandEntryX, "value_training_entry_y": trainingIslandEntryY}).Error
}

// v68 assigned every training ticket the observed one-hour grant, although the
// capture did not establish the consumed ID. Voucher 34181 explicitly describes
// six hours. Correct only the prior default, retaining administrator overrides.
func migrateTimedItemDurations(tx *gorm.DB) error {
	return migrateTrainingItemDuration(tx, game.ItemTrainingVoucher)
}

// v70 adopts the administrator's six-hour Training Ticket rule. As with the
// voucher correction, preserve durations that differ from the prior default.
func migrateTrainingTicketDuration(tx *gorm.DB) error {
	return migrateTrainingItemDuration(tx, game.ItemTrainingTicket)
}

func migrateTrainingItemDuration(tx *gorm.DB, item uint16) error {
	if !tx.Migrator().HasTable(&NativeItemsRow{}) {
		return nil
	}
	defs, err := defaultTimedItems()
	if err != nil {
		return err
	}
	const previousTrainingItemMinutes = 60
	return tx.Model(&NativeItemsRow{}).
		Where("native_items_key = ? AND value_definition_timed_kind = ? AND value_definition_timed_minutes = ? AND value_definition_exp_tenths = ?", item, game.TimedItemTraining, previousTrainingItemMinutes, 0).
		Update("value_definition_timed_minutes", defs[item].TimedMinutes).Error
}
