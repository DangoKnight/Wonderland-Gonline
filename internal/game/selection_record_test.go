package game

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestSelectionRecordNativeFieldsAndEquipment(t *testing.T) {
	c := Character{ID: 10001, Slot: 1, Name: "Hero", Level: 7, Element: 3, HP: 12, MaxHP: 100, SP: 8, MaxSP: 50, EXP: 0x01020304, Gold: 0x05060708, Body: 4, Head: 7, Color1: 0x11223344, Color2: 0x55667788}
	for i := range c.Equipment {
		c.Equipment[i] = Item{ID: uint16(0x1001 + i), Count: 1}
	}
	got, err := c.SelectionRecord()
	if err != nil {
		t.Fatal(err)
	}
	// Independent native record: maxima/current pairs, separate EXP/gold,
	// rebirth/job placeholders, then all six equipment IDs with no extra slot.
	want := []byte{1, 4, 'H', 'e', 'r', 'o', 7, 3,
		100, 0, 0, 0, 12, 0, 0, 0, 50, 0, 0, 0, 8, 0, 0, 0,
		4, 3, 2, 1, 8, 7, 6, 5, 4, 0, 7, 0, 0x44, 0x33, 0x22, 0x11, 0x88, 0x77, 0x66, 0x55,
		0, 0, 1, 0x10, 2, 0x10, 3, 0x10, 4, 0x10, 5, 0x10, 6, 0x10}
	if !bytes.Equal(got, want) {
		t.Fatalf("roster bytes\ngot  %x\nwant %x", got, want)
	}
	c.Slot = 2
	c.Name = "SecondHero"
	second, err := c.SelectionRecord()
	if err != nil {
		t.Fatal(err)
	}
	joined := append(append([]byte{}, got...), second...)
	offset := 0
	for _, name := range []string{"Hero", "SecondHero"} {
		n := int(joined[offset+1])
		record := joined[offset : offset+54+n]
		if string(record[2:2+n]) != name || record[40+n] != 0 || record[41+n] != 0 || binary.LittleEndian.Uint16(record[42+n:]) != 0x1001 {
			t.Fatal("native decoder alignment", record)
		}
		offset += 54 + n
	}
	if offset != len(joined) {
		t.Fatal("roster contains trailing equipment or padding")
	}
}
