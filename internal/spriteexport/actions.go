package spriteexport

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// Some updated installations append JMA sprites without updating the JXA.
// Preserve the actual table byte-for-byte; absence is not a zero-filled record
// or permission to borrow another sprite's animation boundaries.
func decodeActions(source string, data []byte, sprites int) (map[string]any, int, error) {
	if len(data) < spriteHeaderBytes || string(data[:2]) != "jx" {
		return nil, 0, fmt.Errorf("invalid JXA signature")
	}
	count := binary.LittleEndian.Uint32(data[spriteCountOffset:])
	if uint64(spriteHeaderBytes)+uint64(count)*actionRecordBytes != uint64(len(data)) || uint64(count) > uint64(sprites) {
		return nil, 0, fmt.Errorf("invalid JXA record count: %d actions for %d sprites (%d bytes)", count, sprites, len(data))
	}
	rows := make([]any, 0, count)
	for i := range int(count) {
		row := data[spriteHeaderBytes+i*actionRecordBytes:][:actionRecordBytes]
		counts := make([]int, len(row))
		for j, value := range row {
			counts[j] = int(value)
		}
		rows = append(rows, map[string]any{"sprite_index": i, "action_frame_counts": counts, "record_hex": hex.EncodeToString(row)})
	}
	missing := sprites - int(count)
	return map[string]any{"schema_version": exportSchemaVersion, "source": source, "source_sha256": hashBytes(data), "header_hex": hex.EncodeToString(data[:spriteHeaderBytes]), "records": rows, "sprite_count": sprites, "missing_action_records": missing}, missing, nil
}
