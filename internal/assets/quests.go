package assets

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
)

// ParseMarks reads Mark.dat's native mark IDs and completion-flag indices.
// Reference: NotebookManager.LoadMarks. Record zero is a header; a flag of zero means
// the mark is an active journal entry rather than a completion bit.
func ParseMarks(data []byte) (map[uint16]uint16, error) {
	if len(data) < 553 {
		return nil, fmt.Errorf("Mark.dat: too short")
	}
	out := map[uint16]uint16{}
	for off := 553; off+553 <= len(data); off += 553 {
		id := (le.Uint16(data[off+256:]) ^ 0x2774) - 7
		flag := (le.Uint16(data[off+258:]) ^ 0x2774) - 7
		if id != 0 {
			out[id] = flag
		}
	}
	return out, nil
}

// EventKey identifies an EVE event on a map.
func EventKey(mapID, event uint16) uint32 { return uint32(mapID)<<16 | uint32(event) }

// ParseDisabledEvents reads disabled_quest_events.csv (map,event,reason): events whose
// required game data is unavailable and must be refused before any state change.
func ParseDisabledEvents(data []byte) (map[uint32]string, error) {
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	out := map[uint32]string{}
	for i, row := range rows {
		if i == 0 || len(row) < 2 {
			continue
		}
		m, e1 := strconv.ParseUint(row[0], 10, 16)
		ev, e2 := strconv.ParseUint(row[1], 10, 16)
		if e1 != nil || e2 != nil {
			continue
		}
		reason := ""
		if len(row) > 2 {
			reason = row[2]
		}
		out[EventKey(uint16(m), uint16(ev))] = reason
	}
	return out, nil
}

// Drop is one monster_drops.txt loot entry.
type Drop struct {
	Item     uint16
	Name     string
	Min, Max byte
	Rate     float64 // Configured percentage before the server's calibration.
}

// ParseDrops reads "TID:<monster> | <item>,<name>,<min>,<max>,<rate%> | ..." lines.
// Reference: MonsterDropManager.LoadFromFile. Malformed entries are skipped like C#.
func ParseDrops(data []byte) (map[uint32][]Drop, error) {
	out := map[uint32][]Drop{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "TID:") {
			continue
		}
		parts := strings.Split(line, "|")
		tid, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(parts[0], "TID:")), 10, 32)
		if err != nil {
			continue
		}
		for _, p := range parts[1:] {
			f := strings.Split(strings.TrimSpace(p), ",")
			if len(f) < 5 {
				continue
			}
			item, e1 := strconv.ParseUint(strings.TrimSpace(f[0]), 10, 16)
			lo, e2 := strconv.ParseUint(strings.TrimSpace(f[2]), 10, 8)
			hi, e3 := strconv.ParseUint(strings.TrimSpace(f[3]), 10, 8)
			rate, e4 := strconv.ParseFloat(strings.TrimSpace(f[4]), 64)
			if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
				continue
			}
			out[uint32(tid)] = append(out[uint32(tid)], Drop{uint16(item), strings.TrimSpace(f[1]), byte(lo), byte(hi), rate})
		}
	}
	return out, nil
}

// SalePrice is one npc_sale_prices.csv row; flag 16 marks items NPCs will not buy.
type SalePrice struct {
	Price, Flags uint32
}

// ParseSalePrices reads item_id,price,flags rows (the C# embedded NpcSalePrices table).
func ParseSalePrices(data []byte) (map[uint16]SalePrice, error) {
	out := map[uint16]SalePrice{}
	for i, line := range strings.Split(strings.TrimPrefix(string(data), "\xef\xbb\xbf"), "\n") {
		f := strings.Split(strings.TrimSpace(line), ",")
		if i == 0 || len(f) < 3 {
			continue
		}
		id, e1 := strconv.ParseUint(f[0], 10, 16)
		price, e2 := strconv.ParseUint(f[1], 10, 32)
		flags, e3 := strconv.ParseUint(f[2], 10, 32)
		if e1 != nil || e2 != nil || e3 != nil {
			return nil, fmt.Errorf("npc_sale_prices.csv line %d", i+1)
		}
		out[uint16(id)] = SalePrice{uint32(price), uint32(flags)}
	}
	return out, nil
}
