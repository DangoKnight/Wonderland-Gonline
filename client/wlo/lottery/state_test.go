package lottery

import (
	"reflect"
	"testing"
	"time"
)

func TestLuckyDrawNativePackets(t *testing.T) {
	var s State
	// Independent native wire fixtures: IDs 32176 and 34008, quantities 2/3.
	if !s.Apply([]byte{104, 1, 1, 0, 2, 176, 125, 2, 216, 132, 3}) || s.Remaining() != 3 || !reflect.DeepEqual(s.Rewards, []Reward{{32176, 2}, {34008, 3}}) {
		t.Fatal(s)
	}
	now := time.Unix(100, 0)
	if !s.Start(now) || s.Start(now) {
		t.Fatal("duplicate spin accepted")
	}
	if send, _ := s.Advance(now.Add(2799 * time.Millisecond)); send || s.Used != 0 {
		t.Fatal("early request or optimistic allowance")
	}
	if send, _ := s.Advance(now.Add(2800 * time.Millisecond)); !send || !s.Pending {
		t.Fatal("missing request")
	}
	if send, _ := s.Advance(now.Add(3 * time.Second)); send {
		t.Fatal("duplicate request")
	}
	if !s.Apply([]byte{104, 1, 2, 2, 1}) || s.Used != 1 || s.Slot != 2 || s.Busy() {
		t.Fatal(s)
	}
	if reward, ok := s.DisplayReward(); !ok || reward != (Reward{34008, 3}) {
		t.Fatal(reward, ok)
	}
	if !s.Apply([]byte{104, 1, 1, 3, 1, 176, 125, 2}) || s.Remaining() != 0 || s.Start(now) {
		t.Fatal("daily limit ignored")
	}
	if !s.Apply([]byte{104, 1, 1, 0, 1, 176, 125, 2}) || s.Remaining() != 3 {
		t.Fatal("server midnight refresh ignored")
	}
	if !s.Apply([]byte{104, 1, 1, 0, 0}) || s.Start(now) {
		t.Fatal("empty pool enabled a draw")
	}
}

func TestLuckyDrawMalformedIsAtomic(t *testing.T) {
	var s State
	s.Apply([]byte{104, 1, 1, 1, 1, 176, 125, 2})
	s.Start(time.Unix(100, 0))
	before := s
	for _, packet := range [][]byte{
		nil, {104}, {104, 1, 1, 0}, {104, 2, 1, 0, 0}, {104, 1, 3, 0, 0},
		{104, 1, 1, 4, 0}, {104, 1, 1, 0, 21}, {104, 1, 1, 0, 2, 176, 125, 2},
		{104, 1, 1, 0, 0, 99}, {104, 1, 1, 0, 1, 0, 0, 1}, {104, 1, 1, 0, 1, 176, 125, 0},
		{104, 1, 2, 0, 1}, {104, 1, 2, 2, 1}, {104, 1, 2, 1, 0}, {104, 1, 2, 1, 4}, {104, 1, 2, 1, 1, 99},
	} {
		if s.Apply(packet) || !reflect.DeepEqual(s, before) {
			t.Fatalf("malformed %v changed state", packet)
		}
	}
	var empty State
	if empty.Apply([]byte{104, 1, 2, 1, 1}) {
		t.Fatal("result without catalog accepted")
	}
	packet := []byte{104, 1, 1, 0, 20}
	for i := 0; i < 20; i++ {
		packet = append(packet, 176, 125, byte(i+1))
	}
	if !s.Apply(packet) || len(s.Rewards) != 20 || !s.Apply([]byte{104, 1, 2, 20, 2}) {
		t.Fatal("native twentieth slot rejected")
	}
}

func TestLuckyDrawCancelPendingAndTimeout(t *testing.T) {
	var s State
	s.Apply([]byte{104, 1, 1, 0, 1, 176, 125, 2})
	now := time.Unix(100, 0)
	s.Start(now)
	s.CancelAnimation()
	if send, _ := s.Advance(now.Add(time.Minute)); send || s.Busy() || s.Used != 0 {
		t.Fatal("closed animation sent a draw")
	}
	s.Start(now)
	s.Advance(now.Add(2800 * time.Millisecond))
	s.CancelAnimation()
	if s.Start(now) || !s.Pending {
		t.Fatal("closing released in-flight request")
	}
	s.Apply([]byte{104, 1, 1, 0, 1, 176, 125, 2})
	if !s.Pending {
		t.Fatal("unchanged catalog released in-flight request")
	}
	if send, timedOut := s.Advance(now.Add(18 * time.Second)); send || !timedOut || s.Used != 0 || s.Busy() {
		t.Fatal("timeout retried or consumed a draw")
	}
	if !s.Apply([]byte{104, 1, 2, 1, 1}) || s.Used != 1 {
		t.Fatal("late authoritative result ignored")
	}
	s.Reset()
	if s.Ready || s.Rewards != nil || s.Busy() || s.Slot != 0 {
		t.Fatal("reset retained character state")
	}
}
