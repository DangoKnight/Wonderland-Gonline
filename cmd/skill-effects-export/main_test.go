package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"wonderland-go/internal/assets"
)

func TestExportPreservesNativeFieldsAndSharesDefinitions(t *testing.T) {
	dir := t.TempDir()
	skills, effects := filepath.Join(dir, "skills.json"), filepath.Join(dir, "effects.json")
	input := `{"schema_version":1,"format":"native-records","source":"original/Skill.dat","source_sha256":"abc","records":[{"name":{"text":"Shield Defence"},"decoded_hex":"deadbeef","fields":{"id":50000,"effect_layer":19,"unknown_u8_offset_52":52,"unknown_u8_offset_51":3,"unknown_u32_offset_144":123}},{"name":{"text":"Other skill"},"decoded_hex":"cafebabe","fields":{"id":50001,"effect_layer":19,"unknown_u8_offset_52":52,"unknown_u8_offset_51":3}}]}`
	if err := os.WriteFile(skills, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(skills, effects); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(effects)
	if err != nil {
		t.Fatal(err)
	}
	defs, err := assets.ParseEffectCatalog(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 1 || defs["native_19_52_rounds_4"].Effects[0].Modifiers[0].Percent != 10 {
		t.Fatal("incorrect or duplicated definitions", defs)
	}
	var doc struct {
		Records []struct {
			DecodedHex string `json:"decoded_hex"`
			Fields     struct {
				ID      uint16   `json:"id"`
				Unknown int      `json:"unknown_u32_offset_144"`
				Refs    []string `json:"effect_refs"`
			} `json:"fields"`
		} `json:"records"`
	}
	raw, err = os.ReadFile(skills)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Records[0].DecodedHex != "deadbeef" || doc.Records[0].Fields.Unknown != 123 || len(doc.Records[1].Fields.Refs) != 1 || doc.Records[1].Fields.Refs[0] != doc.Records[0].Fields.Refs[0] {
		t.Fatal("native data or references changed", doc)
	}
}

func TestExportUsesAuthoredConversionPolicy(t *testing.T) {
	rules, err := assets.ParseNativeEffectRules([]byte(`{"schema_version":1,"policy":"authored-v2","rules":[{"name":"Custom","layer":201,"code":202,"effects":[{"target":"self","rounds":2,"modifiers":[{"stat":"atk","percent":37}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	skills, effects := filepath.Join(dir, "skills.json"), filepath.Join(dir, "effects.json")
	input := `{"source":"original/Skill.dat","source_sha256":"abc","records":[{"name":{"text":"Custom ability"},"fields":{"id":50000,"effect_layer":201,"unknown_u8_offset_52":202,"unknown_u8_offset_51":3}}]}`
	if err := os.WriteFile(skills, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runWithRules(skills, effects, rules); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(effects)
	if err != nil {
		t.Fatal(err)
	}
	var catalog assets.EffectCatalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.Policy != "authored-v2" || len(catalog.Records) != 1 || catalog.Records[0].ID != "native_201_202_rounds_5" || catalog.Records[0].Effects[0].Modifiers[0].Percent != 37 {
		t.Fatal(catalog)
	}
}
