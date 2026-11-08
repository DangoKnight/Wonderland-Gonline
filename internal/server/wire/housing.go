package wire

import (
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

func ManufactureStartPacket(job *store.ManufactureJob, now time.Time) []byte {
	seconds := max(int64(0), (job.DueAt-now.UnixMilli()+time.Second.Milliseconds()-1)/time.Second.Milliseconds())
	return protocol.Builder{protocol.CommandTentManufacture, protocol.TentManufactureStart, job.Bench}.U16(job.ItemID).U32(uint32(seconds)).U8(max(byte(1), job.Batch))
}

// TentTreeRecord is the native item's 57-byte body after its UInt16 client index.
func TentTreeRecord(index, id uint16, x, y, z uint32, rotation byte, parent uint16, children []uint16, floor byte) []byte {
	p := protocol.Builder{}.U16(index).U16(id).U32(x).U32(y).U32(z).U8(rotation).U16(parent).U16(0)
	for i := 0; i < game.MaxTentToolAttachments; i++ {
		child := uint16(0)
		if i < len(children) {
			child = children[i]
		}
		p = p.U16(child)
	}
	return p.Bytes(make([]byte, tentTreeBeforeFloorPadding)).U8(floor).Bytes(make([]byte, tentTreeAfterFloorPadding))
}

// NativeTentTree sends roots followed by child tools. Client indices are dense
// and independent of SQL slots, which remain stable when a sibling is removed.
func NativeTentTree(t store.Tent) []byte { return NativeTentTreeFloor(t, -1) }

// NativeTentTreeFloor sends the furniture layout for one floor. A negative
// floor keeps every item, which is what entering the tent does.
func NativeTentTreeFloor(t store.Tent, floor int) []byte {
	p := protocol.Builder{protocol.CommandTentFurniture, protocol.TentFurnitureTree}.U32(t.OwnerID)
	next := uint16(len(t.Items) + 1)
	var children [][]byte
	for i, row := range t.Items {
		if floor >= 0 && int(row.Floor) != floor {
			continue
		}
		index := uint16(i + 1)
		var slots []uint16
		for _, tool := range row.Tools {
			slots = append(slots, next)
			children = append(children, TentTreeRecord(next, tool.ItemID, 0, 0, 0, 0, index, nil, tentTreeAttachedFloor))
			next++
		}
		p = p.Bytes(TentTreeRecord(index, row.ItemID, uint32(row.X), uint32(row.Y), tentTreeGroundZ, row.Rotation, 0, slots, row.Floor))
	}
	for _, record := range children {
		p = p.Bytes(record)
	}
	return p
}

const (
	tentTreeBeforeFloorPadding = 8
	tentTreeAfterFloorPadding  = 9
	tentTreeGroundZ            = 1
	tentTreeAttachedFloor      = 1
)
