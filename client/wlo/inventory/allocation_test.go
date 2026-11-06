package inventory

import (
	"bytes"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
)

func TestInventoryPointDraftAndNativeRequest(t *testing.T) {
	now := time.Unix(100, 0)
	f := &Form{Stats: &world.Stats{STR: 2, CON: 3, Points: 3}, Now: func() time.Time { return now }}
	f.Visible = true
	var sent [][]byte
	f.Send = func(p []byte) { sent = append(sent, append([]byte(nil), p...)) }
	f.AddPoint(0)
	f.AddPoint(1)
	f.AddPoint(1)
	f.AddPoint(4)
	if f.PendingPoints != ([5]uint16{1, 2, 0, 0, 0}) || f.Stats.STR != 2 || f.Stats.Points != 3 {
		t.Fatal("draft mutated authoritative stats or exceeded budget")
	}
	f.CancelPoints()
	if f.draftTotal() != 0 || f.Stats.Points != 3 {
		t.Fatal("cancel consumed points")
	}
	f.AddPoint(0)
	f.AddPoint(4)
	f.SubmitAllocation()
	if len(sent) != 1 || !bytes.Equal(sent[0], []byte{8, 1, 0, 2, 28, 1, 0, 0, 0, 30, 1, 0, 0, 0}) {
		t.Fatalf("native allocation %x", sent)
	}
	f.SubmitAllocation()
	f.AddPoint(1)
	if len(sent) != 1 || f.draftTotal() != 0 || !f.AllocationWaiting {
		t.Fatal("duplicate submission while waiting")
	}
	f.Stats.STR = 3
	f.Stats.AGI = 1
	f.Stats.Points = 1
	f.AllocationReply()
	f.AddPoint(1)
	if f.AllocationWaiting || f.PendingPoints[1] != 1 {
		t.Fatal("reply did not allow next allocation")
	}
}

func TestInventoryPointBatchesReachServerParser(t *testing.T) {
	// Every nonempty subset includes adjacent/nonadjacent attribute pairs and
	// mixed amounts. Exercise the actual server decoder and allocation rules,
	// rather than merely comparing a client packet against its own encoder.
	for mask := 1; mask < 1<<allocationAttributes; mask++ {
		f := &Form{Stats: &world.Stats{Points: 15}, Now: time.Now}
		f.Visible = true
		var packet []byte
		f.Send = func(p []byte) { packet = append([]byte(nil), p...) }
		var want [5]uint16
		spent := uint16(0)
		for i := range want {
			if mask&(1<<i) == 0 {
				continue
			}
			want[i] = uint16(i + 1)
			spent += want[i]
			for range want[i] {
				f.AddPoint(i)
			}
		}
		f.SubmitAllocation()
		if len(packet) == 0 {
			t.Fatal("missing allocation", mask)
		}
		target, requests := game.ParseStatAllocation(packet[1], packet[2:])
		character := game.Character{StatPoints: 15}
		if target != 0 || !character.Allocate(requests) {
			t.Fatal("client allocation rejected by server", mask, requests)
		}
		got := [5]uint16{character.Base.Strength, character.Base.Constitution, character.Base.Intelligence, character.Base.Wisdom, character.Base.Agility}
		if got != want || character.StatPoints != 15-spent {
			t.Fatal("server misread client allocation", mask, requests, got, want, character.StatPoints)
		}
	}
}

func TestInventoryPointUnavailableAndTimeout(t *testing.T) {
	now := time.Unix(100, 0)
	f := &Form{Stats: &world.Stats{STR: 65535, Points: 2}, Now: func() time.Time { return now }, Send: func([]byte) {}}
	f.Visible = true
	f.AddPoint(0)
	f.AddPoint(-1)
	f.AddPoint(5)
	if f.draftTotal() != 0 {
		t.Fatal("overflow or unknown attribute accepted")
	}
	f.CanAct = func() bool { return false }
	f.AddPoint(1)
	if f.draftTotal() != 0 {
		t.Fatal("blocked allocation")
	}
	f.CanAct = nil
	f.AddPoint(1)
	f.SubmitAllocation()
	now = now.Add(5 * time.Second)
	f.syncAllocationControls()
	if f.AllocationWaiting {
		t.Fatal("missing reply left controls blocked forever")
	}
	f.Stats.Points = 0
	f.AddPoint(1)
	if f.draftTotal() != 0 {
		t.Fatal("unavailable points consumed")
	}
}
