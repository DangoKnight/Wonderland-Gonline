// Command battle-backdrop-report inventories native backdrop data for every map.
// It is an offline audit tool, not a runtime source of server definitions.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"wonderland-gonline/internal/assets"
)

type mapEntry struct {
	Map        uint16                `json:"map"`
	EventScene uint16                `json:"event_scene,omitempty"`
	HasEvents  bool                  `json:"has_eve_record"`
	Backdrop   assets.BattleBackdrop `json:"backdrop"`
}

func main() {
	source := flag.String("source", "data", "directory containing Ground.MMG and eve.Emg")
	output := flag.String("output", "var/reports/battle-backdrops.json", "audit output")
	flag.Parse()
	if err := run(*source, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(source, output string) error {
	ground, err := os.ReadFile(filepath.Join(source, "Ground.MMG"))
	if err != nil {
		return err
	}
	backdrops, err := assets.ParseBattleBackdrops(ground)
	if err != nil {
		return err
	}
	eve, err := os.ReadFile(filepath.Join(source, "eve.Emg"))
	if err != nil {
		return err
	}
	maps, err := assets.ParseEVE(eve)
	if err != nil {
		return err
	}
	var entries []mapEntry
	regionCount, regionalMaps := 0, 0
	for id, b := range backdrops {
		m, ok := maps[id]
		entries = append(entries, mapEntry{id, m.Scene, ok, b})
		regionCount += len(b.Regions)
		if len(b.Regions) > 0 {
			regionalMaps++
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Map < entries[j].Map })
	var missing []uint16
	for id := range maps {
		if _, ok := backdrops[id]; !ok {
			missing = append(missing, id)
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
	report := struct {
		GroundSHA256 string     `json:"ground_sha256"`
		EveSHA256    string     `json:"eve_sha256"`
		EventMaps    int        `json:"eve_maps"`
		GroundMaps   int        `json:"ground_maps"`
		RegionalMaps int        `json:"maps_with_regions"`
		Regions      int        `json:"region_count"`
		Missing      []uint16   `json:"eve_maps_without_ground"`
		Maps         []mapEntry `json:"maps"`
	}{fmt.Sprintf("%x", sha256.Sum256(ground)), fmt.Sprintf("%x", sha256.Sum256(eve)), len(maps), len(backdrops), regionalMaps, regionCount, missing, entries}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	if err = os.WriteFile(output, append(raw, '\n'), 0644); err != nil {
		return err
	}
	fmt.Printf("%d event maps, %d terrain maps, %d regional maps, %d rectangles; %d event maps missing terrain. Report: %s\n", len(maps), len(backdrops), regionalMaps, regionCount, len(missing), output)
	return nil
}
