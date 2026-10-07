package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/protocol"
)

func TestInstanceRoomsCapacityAuthorityAndDisconnect(t *testing.T) {
	s, p, w := partyFixture(t)
	ctx := context.Background()
	s.Assets.Instances = []assets.InstanceDefinition{{ID: 30001, Name: "Trial", Capacity: 2, MinimumLevel: 1, Minutes: 40, EntryMap: 40001}}
	create := []byte{85, 3, 49, 117, 4, 'T', 'e', 's', 't'}
	if err := s.worldCommand(ctx, p[0], create); err != nil {
		t.Fatal(err)
	}
	room := s.instanceOf(p[0])
	if room == nil || room.Name != "Test" || room.ID != 61501 {
		t.Fatal("room creation", room)
	}
	if !hasPacket(w[0].packets(t), []byte{85, 14, 1, 49, 117, 1}) {
		t.Fatal("native definitions reply")
	}
	join := []byte{85, 200, 61, 240}
	if err := s.worldCommand(ctx, p[1], join); err != nil {
		t.Fatal(err)
	}
	if len(room.Members) != 2 {
		t.Fatal("join failed")
	}
	if err := s.worldCommand(ctx, p[2], join); err != nil {
		t.Fatal(err)
	}
	if s.instanceOf(p[2]) != nil || !hasPacket(w[2].packets(t), []byte{85, 2, 1}) {
		t.Fatal("capacity not enforced")
	}
	before := p[0].character.Clone()
	if err := s.worldCommand(ctx, p[1], []byte{85, 202}); err != nil {
		t.Fatal(err)
	}
	if !hasPacket(w[1].packets(t), []byte{85, 2, 255}) {
		t.Fatal("nonleader started room")
	}
	if err := s.worldCommand(ctx, p[0], []byte{85, 202}); err != nil {
		t.Fatal(err)
	}
	if !hasPacket(w[0].packets(t), []byte{85, 2, 11}) || before.Map != p[0].character.Map || before.Gold != p[0].character.Gold {
		t.Fatal("unverified dungeon executed")
	}
	s.instanceLeave(p[0])
	if room.Members[0] != p[1] {
		t.Fatal("leader handoff")
	}
	s.instanceLeave(p[1])
	if len(s.instanceRooms) != 0 {
		t.Fatal("empty room retained")
	}
}
func TestInstanceRequestsRejectMalformedAndLowLevel(t *testing.T) {
	s, p, w := partyFixture(t)
	s.Assets.Instances = []assets.InstanceDefinition{{ID: 30001, Name: "Trial", Capacity: 2, MinimumLevel: 50, Minutes: 40, EntryMap: 40001}}
	for _, packet := range [][]byte{{85, 3}, {85, 3, 49, 117, 2, 'x'}, {85, 200}, {85, 201, 1}, {85, 1, 1, 1}} {
		if err := s.instanceCommand(context.Background(), p[0], packet); err != protocol.ErrMalformed {
			t.Fatal(packet, err)
		}
	}
	if err := s.instanceCommand(context.Background(), p[0], []byte{85, 3, 49, 117, 0}); err != nil {
		t.Fatal(err)
	}
	if len(s.instanceRooms) != 0 || !hasPacket(w[0].packets(t), []byte{85, 2, 2}) {
		t.Fatal("minimum level bypassed")
	}
}
func TestTeammateStatsNativeFieldIDs(t *testing.T) {
	s, p, _ := partyFixture(t)
	p[0].character.Level = 1
	p[0].character.HP = 181
	packets := s.teammateStats(p[0])
	var level, hp bool
	for _, packet := range packets {
		if len(packet) != 16 || packet[7] != 1 {
			t.Fatal(packet)
		}
		if packet[6] == 35 {
			level = packet[8] == 1
		}
		if packet[6] == 25 {
			hp = packet[8] == 181
		}
	}
	if !level || !hp {
		t.Fatal("native level and HP fields")
	}
}
