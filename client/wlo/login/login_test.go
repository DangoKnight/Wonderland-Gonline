package login

import (
	"bytes"
	"testing"
)

func TestBuildLoginGolden(t *testing.T) {
	p := BuildLogin([]byte(" ABC "), []byte("pw"), nil)
	// 63/4, version 0x04bb, "ABC", "pw", empty token; the key byte is random.
	want := []byte{63, 4, 0xbb, 0x04, 3, 'A', 'B', 'C', 2, 'p', 'w', 0}
	if len(p) != len(want)+1 || !bytes.Equal(p[:len(want)], want) {
		t.Fatalf("got % x", p)
	}
	p = BuildLogin([]byte("ABCDEFGHIJKL"), nil, []byte("12"))
	key := p[len(p)-3]
	if p[4] != 10 || string(p[5:15]) != "ABCDEFGHIJ" || p[len(p)-4] != 2 ||
		p[len(p)-2] != '1'^key || p[len(p)-1] != '2'^key {
		t.Fatalf("got % x", p)
	}
}

func TestValidAccount(t *testing.T) {
	for s, want := range map[string]bool{
		"AB": false, "ABC": true, "ABCDEFGHIJ": true, "ABCDEFGHIJK": false,
		" A": false, "WR123": true, "wr123": true, "WR0": false, "WR4500001": true,
		"WRX": false, "WR9000000": true, "WR9000001": false,
	} {
		if got := ValidAccount([]byte(s)); got != want {
			t.Errorf("%q: got %v", s, got)
		}
	}
}

func TestReadStatusKeepsPartialRecords(t *testing.T) {
	g := NewGlobals()
	// The reply header parses as an ID of 10000 or more and is dropped.
	g.ReadStatus([]byte{0xc9, 0xff, 0x00, 101, 0})
	g.ReadStatus([]byte{2, 102, 0, 3})
	if g.Status[101] != 2 || g.Status[102] != 3 {
		t.Fatalf("status 101=%d 102=%d", g.Status[101], g.Status[102])
	}
}

func TestReceiveReframes(t *testing.T) {
	n := NewNet()
	enc := func(b []byte) []byte {
		out := make([]byte, len(b))
		for i, c := range b {
			out[i] = c ^ 0xad
		}
		return out
	}
	// Two stray bytes, a frame split across reads, then a whole frame.
	got := n.receive(enc([]byte{1, 2, 0xf4, 0x44, 3, 0, 1, 9}))
	got = append(got, n.receive(enc([]byte{7, 0xf4, 0x44, 1, 0, 5}))...)
	if len(got) != 2 || !bytes.Equal(got[0], []byte{1, 9, 7}) || !bytes.Equal(got[1], []byte{5}) {
		t.Fatalf("got %v", got)
	}
}

func TestParseRegionAndServer(t *testing.T) {
	name, color, flag := parseRegion([]byte("01[Rhodes Island]<G>12"))
	if string(name) != "Rhodes Island" || color != tagGreen || flag != 12 {
		t.Fatalf("got %q %x %d", name, color, flag)
	}
	if _, _, flag := parseRegion([]byte("02[Lion]")); flag != 0 {
		t.Fatalf("flag %d", flag)
	}
	if n, a, ok := parseServer([]byte("Lion1*47.238.172.210")); string(n) != "Lion1" || string(a) != "47.238.172.210" || !ok {
		t.Fatalf("got %q %q", n, a)
	}
	if n, _, ok := parseServer([]byte("*1.2.3.4")); string(n) != "No Name" || !ok {
		t.Fatalf("got %q", n)
	}
	if _, _, ok := parseServer([]byte("Only*")); ok {
		t.Fatal("an address is required")
	}
}

func TestDisconnectReason(t *testing.T) {
	if got := string(DisconnectReason(1, 0)); got != "D/c:1" {
		t.Fatalf("got %q", got)
	}
	if got := string(DisconnectReason(3, 0)); got != "Wrong Pwd:3" {
		t.Fatalf("got %q", got)
	}
	if got := string(DisconnectReason(39, 0)); got != "Connection lost:255" {
		t.Fatalf("got %q", got)
	}
	if got := string(DisconnectReason(1, 6)); got != "Too Many Packets:1" {
		t.Fatalf("got %q", got)
	}
}

func TestSplitLines(t *testing.T) {
	got := splitLines([]byte("a\r\nb\nc\rd"))
	if len(got) != 4 || string(got[3]) != "d" {
		t.Fatalf("got %q", got)
	}
}

func TestParseSlotRecord(t *testing.T) {
	// slot 1, "Ann", level 7, element 3, six u32 values, body 2, head 5,
	// colours, reborn 1, job 4, six equipment IDs.
	rec := []byte{1, 3, 'A', 'n', 'n', 7, 3}
	for v := uint32(10); v <= 60; v += 10 {
		rec = append(rec, byte(v), 0, 0, 0)
	}
	rec = append(rec, 2, 0, 5, 0, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 1, 4)
	for id := uint16(1001); id <= 1006; id++ {
		rec = append(rec, byte(id), byte(id>>8))
	}
	if len(rec) != 3+slotRecordTail {
		t.Fatalf("record is %d bytes", len(rec))
	}
	c := parseSlot(rec, 3)
	if string(c.Name) != "Ann" || c.Level != 7 || c.Element != 3 || c.Value[slotHP] != 20 || c.Value[slotGold] != 60 ||
		c.Body != 2 || c.Head != 5 || c.Color1 != 0x44332211 || c.Reborn != 1 || c.Job != 4 ||
		c.Equipment[1] != 1001 || c.Equipment[6] != 1006 || c.Direction != defaultDirection {
		t.Fatalf("got %+v", c)
	}
}

func TestFormulaLevelExp(t *testing.T) {
	f := &Formula{ExpPower: 3.1, ExpOffset: 5}
	if got := f.LevelExp(1, false); got != 6 {
		t.Fatalf("level 1: %d", got)
	}
	if got := f.LevelExp(2, true); got != 59 {
		t.Fatalf("reborn level 2: %d", got)
	}
	if p := f.Progress(1, 6+6, false); p <= 0 || p >= 1 {
		t.Fatalf("progress %v", p)
	}
}

func TestAlreadyLoggedInReason(t *testing.T) {
	if got := string(DisconnectReason(19, 0)); got != "Character is already logged in:19" {
		t.Fatal(got)
	}
}
