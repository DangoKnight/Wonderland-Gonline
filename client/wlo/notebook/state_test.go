package notebook

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNotebookNativeJournalAndDiscovery(t *testing.T) {
	s := State{Marks: map[uint16]Mark{12018: {ID: 12018, Name: "Dinner Incident", Description: "#01Find pepper#02Deliver pepper#99Finished"}, 12019: {ID: 12019, Flag: 13, Journal: 12018}}, Monsters: map[uint16]Monster{10001: {ID: 10001, Index: 3, Name: "Banana"}, 10002: {ID: 10002, Index: 1, Name: "Kiwi"}}}
	for _, p := range [][]byte{{24, 6, 1, 242, 46, 2}, {53, 9, 17, 39, 0, 0}, {53, 9, 18, 39, 0, 0}, {53, 9, 18, 39, 0, 0}} {
		if !s.Apply(p) {
			t.Fatal("native packet rejected", p)
		}
	}
	if got := s.Entries(); !reflect.DeepEqual(got, []Entry{{12018, "Dinner Incident", "Deliver pepper", false}}) {
		t.Fatal(got)
	}
	if got := s.MonsterList(); len(got) != 2 || got[0].ID != 10002 || got[1].ID != 10001 {
		t.Fatal("book order/discovery dedup", got)
	}
	if !s.Apply([]byte{24, 7, 2, 16}) {
		t.Fatal("completion snapshot")
	}
	if got := s.Entries(); len(got) != 1 || !got[0].Completed || got[0].Description != "Finished" {
		t.Fatal(got)
	}
	if !s.Apply([]byte{24, 4, 242, 46}) || len(s.Entries()) != 1 {
		t.Fatal("removing active mark removed completion")
	}
	if !s.Apply([]byte{24, 5, 13, 0, 0}) || len(s.Entries()) != 0 {
		t.Fatal("completion unset")
	}
	before := s.Flags
	if s.Apply([]byte{24, 7, 1, 255, 0, 0}) || s.Flags != before {
		t.Fatal("malformed snapshot partly applied")
	}
	s.Reset()
	if len(s.MonsterList()) != 0 || len(s.Entries()) != 0 || len(s.Marks) == 0 {
		t.Fatal("reset must clear owned state, retain definitions")
	}
}
func TestNotebookExportAndDescriptionSteps(t *testing.T) {
	dir := t.TempDir()
	marks := filepath.Join(dir, "mark.json")
	npcs := filepath.Join(dir, "npc.json")
	os.WriteFile(marks, []byte(`{"records":[{"name":{"text":"Pirate"},"description":{"text":"#01Rescue#99Free"},"fields":{"id":13004,"completion_flag":0}},{"name":{"text":"Pirate"},"fields":{"id":13005,"completion_flag":18}}]}`), 0600)
	os.WriteFile(npcs, []byte(`{"records":[{"name":{"text":"Kiwi Monster"},"fields":{"id":10001,"book_index":2,"level":3,"hp":42,"sp":100,"strength":1,"constitution":2,"intelligence":3,"wisdom":4,"agility":5}}]}`), 0600)
	var s State
	if err := s.Load(marks, npcs); err != nil {
		t.Fatal(err)
	}
	if s.Marks[13005].Journal != 13004 || s.Monsters[10001].HP != 42 || s.Monsters[10001].Stats != [5]uint16{1, 2, 3, 4, 5} {
		t.Fatal(s)
	}
	if StepText("#01Rescue#99Free", 99) != "Free" || StepText("#01Rescue#99Free", 2) != "" {
		t.Fatal("wrong stage description")
	}
	if got := wrap("中文ABC", 4); !reflect.DeepEqual(got, []string{"中文", "ABC"}) {
		t.Fatal("Chinese width", got)
	}
}
