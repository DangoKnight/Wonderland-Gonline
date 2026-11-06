package assets

import "testing"

func TestAnimationContactsNativeBytes(t *testing.T) {
	// One indexed 42-byte record: mode 2, three five-byte contact cues.
	record := make([]byte, 42)
	record[24], record[25] = 2, 3
	archive := append(append([]byte(nil), record...), []byte{0x51, 0xc3, 0, 0, 0, 0, 42, 0, 0, 0, 1, 0}...)
	counts, err := ParseAnimationContacts(archive)
	if err != nil || counts[50001] != 3 {
		t.Fatal(counts, err)
	}
	record[24] = 0
	if count, err := AnimationContactCount(record); err != nil || count != 1 {
		t.Fatal("single damage mode", count, err)
	}
	record[24], record[25] = 2, 0
	if count, err := AnimationContactCount(record[:27]); err != nil || count != 1 {
		t.Fatal("empty events", count, err)
	}
	record[25] = 4
	if _, err := AnimationContactCount(record); err == nil {
		t.Fatal("truncated contacts accepted")
	}
	record[25] = 51
	if _, err := AnimationContactCount(record); err == nil {
		t.Fatal("excessive contacts accepted")
	}
	archive[len(archive)-10] = 1 // Non-contiguous first record offset.
	if _, err := ParseAnimationContacts(archive); err == nil {
		t.Fatal("broken index accepted")
	}
}

func TestImportedSkillContactsPreserveAuthoredCounts(t *testing.T) {
	skills := map[uint16]Skill{1: {ID: 1}, 2: {ID: 2, ContactEvents: 7}}
	ApplySkillContacts(skills, map[uint16]byte{1: 3, 2: 4, 3: 5})
	if skills[1].ContactEvents != 3 || skills[2].ContactEvents != 7 || len(skills) != 2 {
		t.Fatal(skills)
	}
}
