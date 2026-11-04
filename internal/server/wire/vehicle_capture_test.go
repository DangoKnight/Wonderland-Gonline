package wire

import (
	"bytes"
	"testing"

	"wonderland-gonline/internal/game"
)

// Independent raw-byte expectations from raft-reference-20261103-01.pcapng.
func TestVehicleLegacyCapture(t *testing.T) {
	c := game.Character{ID: 4617590, VehicleSlot: 12, ActiveVehicle: 48016}
	if got := VehicleMountPacket(&c); !bytes.Equal(got, []byte{15, 10, 12, 118, 117, 70, 0, 144, 187}) {
		t.Fatalf("mount = %v", got)
	}
	if got := VehicleDismountPacket(&c); !bytes.Equal(got, []byte{15, 11, 12, 118, 117, 70, 0}) {
		t.Fatalf("dismount = %v", got)
	}
}
