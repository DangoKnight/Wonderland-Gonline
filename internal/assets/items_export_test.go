package assets

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestNativeExportPreservesHeaderRecordsAndUnknownFields(t *testing.T) {
	data, decoded := nativeItemGolden(t)
	// File order intentionally differs from ID order.
	other := bytes.Clone(data[451:])
	other[16] = 0xd9
	other[17] = 0xc8 // 10001
	data = append(data, other...)
	export, err := ExportNativeItems(data, "fixture/Item.dat")
	if err != nil {
		t.Fatal(err)
	}
	if len(export.Items) != 2 || export.SourceBytes != len(data) || export.NativeVersion != 9 || export.RecordBytes != 451 || export.Items[0].Definition.ID != 10002 || export.Items[1].Definition.ID != 10001 {
		t.Fatal(export)
	}
	header, err := hex.DecodeString(export.HeaderHex)
	if err != nil || !bytes.Equal(header, data[:451]) {
		t.Fatal("header lost", err)
	}
	record, err := hex.DecodeString(export.Items[0].DecodedHex)
	if err != nil || !bytes.Equal(record, decoded) {
		t.Fatal("decoded bytes lost", err)
	}
	if export.Items[1].RecordIndex != 2 || export.Items[1].FileOffset != 902 {
		t.Fatal("file order lost")
	}
	encoded, err := json.Marshal(export)
	if err != nil {
		t.Fatal(err)
	}
	var restored NativeItemExport
	if err = json.Unmarshal(encoded, &restored); err != nil || restored.Items[0].Definition.Values[1] != -2 || restored.Items[0].Definition.Name != "測試Item" || restored.Items[0].DecodedHex != export.Items[0].DecodedHex {
		t.Fatal("JSON lost data", err)
	}
	duplicate := append(bytes.Clone(data), data[451:902]...)
	if _, err = ExportNativeItems(duplicate, "fixture"); err == nil {
		t.Fatal("duplicate silently omitted")
	}
}
