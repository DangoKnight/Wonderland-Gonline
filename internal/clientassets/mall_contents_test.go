package clientassets

import "testing"

func TestChobitsContainerContents(t *testing.T) {
	packs := DefaultMallContents()
	for _, id := range []uint16{35450, 35451} {
		r := packs[id]
		if len(r) != 3 || r[2] != (MallReward{22593, 1}) {
			t.Fatal("Chobits bundle preview", id, r)
		}
	}
	if !MallIncompleteContents()[56276] || len(packs[56276]) != 3 {
		t.Fatal("winter bag must identify partial contents")
	}
}

func TestParseMallContentsNativeLayout(t *testing.T) {
	// One bag, two entries, native metadata 2; U16 IDs and U8 quantities.
	raw := []byte{1, 0, 2, 94, 215, 2, 120, 105, 1, 91, 133, 3}
	packs, err := ParseMallContents(raw)
	if err != nil || len(packs) != 1 || packs[0].ItemID != 55134 || packs[0].Kind != 2 || len(packs[0].Rewards) != 2 || packs[0].Rewards[0] != (MallReward{27000, 1}) || packs[0].Rewards[1] != (MallReward{34139, 3}) {
		t.Fatal(packs, err)
	}
	for n := 0; n < len(raw); n++ {
		if _, err := ParseMallContents(raw[:n]); err == nil {
			t.Fatalf("accepted truncation %d", n)
		}
	}
	if _, err := ParseMallContents(append(raw, 0)); err == nil {
		t.Fatal("accepted trailing byte")
	}
	raw[len(raw)-1] = 0
	if _, err := ParseMallContents(raw); err == nil {
		t.Fatal("accepted zero quantity")
	}
}
func TestPetUpgradeScreenshotContents(t *testing.T) {
	r := DefaultMallContents()[55134]
	if len(r) != 26 {
		t.Fatal(len(r))
	}
	// Column-major list: Banana Boat, Fairy Bean, Passion Water, and scroll x3.
	if r[0] != (MallReward{27000, 1}) || r[10] != (MallReward{27009, 1}) || r[20] != (MallReward{27020, 1}) || r[24] != (MallReward{34139, 3}) {
		t.Fatal(r)
	}
}

func TestMallAdditionalScreenshotDefinitions(t *testing.T) {
	previews := DefaultMallContents()
	for _, want := range []struct {
		id          uint16
		count       int
		first, last MallReward
	}{
		{55073, 8, MallReward{45001, 1}, MallReward{45008, 1}},
		{55074, 9, MallReward{42010, 1}, MallReward{42023, 1}},
		{57819, 14, MallReward{35135, 1}, MallReward{63115, 1}},
		{56307, 11, MallReward{35135, 1}, MallReward{10101, 1}},
		{55079, 9, MallReward{35137, 1}, MallReward{34236, 1}},
	} {
		r := previews[want.id]
		if len(r) != want.count || r[0] != want.first || r[len(r)-1] != want.last {
			t.Fatalf("bag %d: %v", want.id, r)
		}
	}
	if !MallCompanionContents()[34172] || DefaultMallVouchers()[30240] == 0 {
		t.Fatal("missing companion egg presentation")
	}
	if DefaultMallFitting()[26017] != 21174 || DefaultMallFitting()[26013] != 25192 {
		t.Fatal("incorrect fitting contents")
	}
}
