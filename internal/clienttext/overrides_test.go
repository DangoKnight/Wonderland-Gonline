package clienttext

import (
	"strings"
	"testing"
)

func TestIGGOverrideCatalogAndCustomText(t *testing.T) {
	rows, err := parseOverrides(iggCSV)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[Table]int{}
	for k, p := range rows {
		counts[k.table]++
		if k.field == Name && strings.Contains(strings.ToLower(p.english), "(quest)") {
			t.Fatal("quest annotation in English name", k)
		}
		if got := Override(k.table, k.id, k.field, p.original); got != p.english {
			t.Fatal("verified override missing", k)
		}
		if got := Override(k.table, k.id, k.field, "Custom translation"); got != "Custom translation" {
			t.Fatal("custom text replaced", k)
		}
		if got := Override(k.table, k.id, k.field, p.english); got != p.english {
			t.Fatal("override is not idempotent", k)
		}
	}
	for table, want := range map[Table]int{NPC: 2379, Item: 4051, Skill: 472, Scene: 233, Talk: 2952, Mark: 807} {
		if counts[table] != want {
			t.Fatalf("%s: %d overrides, want %d", table, counts[table], want)
		}
	}
	// Verified IGG names now take precedence over earlier authored corrections.
	for id, want := range map[uint32]string{17738: "Half Beast Ghost", 30310: "Yellow Rock Owl", 40121: "Pineapple Guard"} {
		if rows[key{NPC, id, Name}].english != want {
			t.Fatal("IGG priority missing", id)
		}
	}
	if value, ok := Resolve(NPC, 12073, Name, "Cool Ronin"); !ok || value != "Cool Ronin" {
		t.Fatal("unchanged IGG name did not suppress fallback corrections")
	}
}

func TestMalformedIGGOverridesAreRejected(t *testing.T) {
	lines := strings.Split(iggCSV, "\n")
	valid := lines[0] + "\n" + lines[1] + "\n"
	for _, data := range []string{"", "wrong,header\n", valid + lines[1] + "\n", strings.Replace(valid, "IGG_6", "", 1), lines[0] + "\nnpc,1,name\n"} {
		if _, err := parseOverrides(data); err == nil {
			t.Fatal("invalid overrides accepted")
		}
	}
}
