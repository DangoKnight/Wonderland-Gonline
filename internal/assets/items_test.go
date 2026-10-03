package assets

import (
	"encoding/binary"
	"testing"
)

func TestConvertedItemRecordBoundaries(t *testing.T) {
	data := make([]byte, 47*3)
	b := data[47:94]
	copy(b[1:21], "Starter sword")
	b[21] = 1
	binary.LittleEndian.PutUint16(b[22:], 10002)
	binary.LittleEndian.PutUint16(b[28:], 3)
	binary.LittleEndian.PutUint16(b[30:], 12)
	binary.LittleEndian.PutUint16(b[35:], 210)
	binary.LittleEndian.PutUint16(b[37:], 214)
	binary.LittleEndian.PutUint32(b[39:], 120)
	binary.LittleEndian.PutUint32(b[43:], 0xfffffffe)
	copy(data[94:], b)
	data[94+21] = 2 // The first duplicate wins, matching the original lookup.
	items, err := ParseConvertedItems(data)
	if err != nil {
		t.Fatal(err)
	}
	item := items[10002]
	if len(items) != 1 || item.Name != "Starter sword" || item.Type != 1 || item.EquipSlot != 3 || item.Level != 12 || item.Status != [2]uint16{210, 214} || item.Values != [2]int32{120, -2} {
		t.Fatal(items)
	}
	for _, invalid := range [][]byte{nil, data[:len(data)-1], data[:46]} {
		if _, err := ParseConvertedItems(invalid); err == nil {
			t.Fatal("accepted incomplete item record")
		}
	}
}

func TestStarterItemOrderAndValidation(t *testing.T) {
	grants, err := ParseStarterItems([]byte(`[{"OrderIdx":3,"ItemID":32176,"Count":50},{"OrderIdx":1,"ItemID":34038,"Count":1}]`))
	if err != nil || len(grants) != 2 || grants[0].ID != 34038 || grants[1].Count != 50 {
		t.Fatal(grants, err)
	}
	for _, invalid := range []string{`null`, `[]`, `[{"ItemID":0,"Count":1}]`, `[{"ItemID":1,"Count":0}]`, `[{"ItemID":1,"Count":2501}]`, `[{"ItemID":65536,"Count":1}]`, `[] []`} {
		if _, err := ParseStarterItems([]byte(invalid)); err == nil {
			t.Fatal("accepted invalid starter grants", invalid)
		}
	}
}
