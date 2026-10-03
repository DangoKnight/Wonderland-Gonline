// Export compatibility effect definitions alongside a freshly decoded Skill.dat.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"wonderland-go/internal/assets"
)

func run(input, output string) error {
	return runWithRules(input, output, assets.DefaultNativeEffectRules())
}

func runWithRules(input, output string, rules assets.NativeEffectRules) error {
	raw, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	var doc map[string]json.RawMessage
	if err = json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	var rows []map[string]json.RawMessage
	if err = json.Unmarshal(doc["records"], &rows); err != nil {
		return err
	}
	catalog := assets.EffectCatalog{SchemaVersion: assets.EffectCatalogVersion, Format: assets.EffectCatalogFormat, Policy: rules.Policy, Records: []assets.EffectDefinition{}}
	if err = json.Unmarshal(doc["source"], &catalog.Source); err != nil {
		return err
	}
	if err = json.Unmarshal(doc["source_sha256"], &catalog.SourceSHA256); err != nil {
		return err
	}
	definitions := map[string]assets.EffectDefinition{}
	for _, row := range rows {
		var fields map[string]json.RawMessage
		if err = json.Unmarshal(row["fields"], &fields); err != nil {
			return err
		}
		var skill assets.Skill
		if err = json.Unmarshal(row["fields"], &skill); err != nil {
			return err
		}
		skill.Effects, skill.EffectRefs = nil, nil
		if err := rules.Populate(&skill); err != nil {
			return err
		}
		refs := []string{}
		if len(skill.Effects) > 0 {
			id := fmt.Sprintf("native_%d_%d_rounds_%d", skill.EffectLayer, skill.NativeEffectCode52, skill.Effects[0].Rounds)
			refs = append(refs, id)
			if _, exists := definitions[id]; !exists {
				var name struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(row["name"], &name); err != nil {
					return err
				}
				if name.Text == "" {
					name.Text = id
				}
				definitions[id] = assets.EffectDefinition{ID: id, Name: name.Text, Effects: skill.Effects}
			}
		}
		delete(fields, "effects")
		fields["effect_refs"], err = json.Marshal(refs)
		if err != nil {
			return err
		}
		row["fields"], err = json.Marshal(fields)
		if err != nil {
			return err
		}
	}
	ids := make([]string, 0, len(definitions))
	for id := range definitions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		catalog.Records = append(catalog.Records, definitions[id])
	}
	if _, err = assets.ParseEffectCatalog(mustJSON(catalog)); err != nil {
		return err
	}
	doc["records"], err = json.Marshal(rows)
	if err != nil {
		return err
	}
	if err = write(output, catalog); err != nil {
		return err
	}
	return write(input, doc)
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func write(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
func main() {
	input := flag.String("skills", "", "Fresh native skill JSON export")
	output := flag.String("effects", "", "Effect JSON destination")
	policy := flag.String("rules", "", "Optional authored native effect conversion policy JSON")
	flag.Parse()
	if *input == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "-skills and -effects are required")
		os.Exit(2)
	}
	rules := assets.DefaultNativeEffectRules()
	if *policy != "" {
		raw, err := os.ReadFile(*policy)
		if err == nil {
			rules, err = assets.ParseNativeEffectRules(raw)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := runWithRules(*input, *output, rules); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
