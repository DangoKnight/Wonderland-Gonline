package world

import (
	"bytes"
	"sync"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

func simulationFixture() *World {
	c := &assets.Catalog{Maps: map[uint16]assets.Map{12000: {ID: 12000, NPCs: []assets.MapNPC{
		{ClickID: 1, Template: 17001, Name: "Rabbit", X: 100, Y: 100, Flags: 1, WalkBehavior: 2, WalkSteps: []assets.WalkStep{{X: 200, Y: 100, Delay: 1000}}},
		{ClickID: 2, Template: 19001, Name: "Chest", X: 100, Y: 100, Flags: 1, WalkBehavior: 2, WalkSteps: []assets.WalkStep{{X: 200, Y: 100}}},
		{ClickID: 3, Template: 17002, Name: "Duck", X: 100, Y: 100, Flags: 1, WalkBehavior: 3, WalkSteps: []assets.WalkStep{{X: uint32(0xffffffff), Y: uint32(0xffffffff)}, {X: 20, Y: 20}}},
	}}}, NPCs: map[uint16]assets.NPC{17001: {ID: 17001, Name: "Rabbit"}, 17002: {ID: 17002, Name: "Duck"}}}
	return New(c)
}
func TestWorldSimulationPatrolVisibilityAndRespawn(t *testing.T) {
	w := simulationFixture()
	now := time.Unix(1000, 0)
	random := func(int) int { return 0 }
	if len(w.AdvanceActors(12000, now, nil, random)) != 0 {
		t.Fatal("initial delay ignored")
	}
	moves := w.AdvanceActors(12000, now.Add(time.Second), map[uint16]bool{3: true}, random)
	if len(moves) != 1 || !bytes.Equal(moves[0].Packet(), []byte{22, 2, 1, 0, 200, 0, 100, 0, 2}) {
		t.Fatal(moves)
	}
	n, _ := w.NPC(12000, 1)
	if n.X != 200 {
		t.Fatal(n)
	}
	spawn, _ := w.maps[12000].NPC(1)
	if spawn.X != 100 {
		t.Fatal("immutable spawn changed", spawn)
	}
	x, y := w.coordinates(12000, 1)
	if x != 200 || y != 100 {
		t.Fatal("show uses stale coordinates", x, y)
	}
	char := &game.Character{Map: 12000}
	view := NewView()
	scene := w.MapInfo(char, view, nil)[1]
	if scene[6] != 200 {
		t.Fatal("entry position", scene)
	}
	if len(w.AdvanceActors(12000, now.Add(2*time.Second), map[uint16]bool{3: true}, random)) != 0 {
		t.Fatal("minimum waypoint delay")
	}
	moves = w.AdvanceActors(12000, now.Add(3*time.Second), map[uint16]bool{3: true}, random)
	if len(moves) != 1 || moves[0].X != 100 {
		t.Fatal("single waypoint return", moves)
	}
	w.Defeat(12000, 1, now.Add(4*time.Second))
	if len(w.AdvanceActors(12000, now.Add(20*time.Second), map[uint16]bool{3: true}, random)) != 0 {
		t.Fatal("defeated actor moved")
	}
	revived := now.Add(4*time.Second + MonsterRespawn)
	w.Revive(revived)
	n, _ = w.NPC(12000, 1)
	if n.X != 100 {
		t.Fatal("respawn origin", n)
	}
	if len(w.AdvanceActors(12000, revived.Add(2*time.Second), map[uint16]bool{3: true}, random)) != 0 {
		t.Fatal("respawn grace")
	}
	if got := w.AdvanceActors(12000, revived.Add(3*time.Second), map[uint16]bool{3: true}, random); len(got) != 1 || got[0].X != 200 {
		t.Fatal("patrol reset", got)
	}
}
func TestWorldSimulationBoundedWanderingAndPause(t *testing.T) {
	w := simulationFixture()
	now := time.Unix(1, 0)
	random := func(int) int { return 0 }
	w.AdvanceActors(12000, now, nil, random)
	got := w.AdvanceActors(12000, now.Add(time.Second), map[uint16]bool{1: true}, random)
	if len(got) != 1 || got[0].Click != 3 || got[0].X != 99 || got[0].Y != 99 {
		t.Fatal("signed bounds/static prop/pause", got)
	}
}
func TestWorldSimulationTerrainConstraints(t *testing.T) {
	cells := make([]byte, 25)
	cells[2*5+2] = assets.TerrainBlockedMask
	w := New(&assets.Catalog{Terrains: map[uint16]assets.Terrain{1: {Width: 100, Height: 100, GridWidth: 5, GridHeight: 5, Cells: cells}}})
	for _, tt := range []struct {
		fx, fy, x, y uint16
		ok           bool
	}{{10, 10, 90, 10, true}, {10, 50, 90, 50, false}, {10, 10, 50, 50, false}, {10, 10, 100, 10, false}, {50, 50, 10, 50, true}, {50, 50, 51, 51, false}} {
		if got := w.CanMove(1, tt.fx, tt.fy, tt.x, tt.y); got != tt.ok {
			t.Fatal(tt, got)
		}
	}
	if !w.CanMove(2, 10, 10, 200, 200) {
		t.Fatal("missing terrain broke compatibility")
	}
	if !RoamingTown(12001) || RoamingTown(12000) {
		t.Fatal("wilderness/town boundary")
	}
}

func TestWorldSimulationConcurrentSnapshots(t *testing.T) {
	w := simulationFixture()
	now := time.Unix(1, 0)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			w.AdvanceActors(12000, now.Add(time.Duration(i)*time.Second), nil, func(int) int { return 0 })
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			w.NPC(12000, 1)
			w.NPCs(12000)
			w.ShowActor(NewView(), 12000, 1)
		}
	}()
	wg.Wait()
}

func TestWorldSimulationPatrolLoopAndBlockedLeg(t *testing.T) {
	cells := make([]byte, 25)
	cells[2*5+1] = assets.TerrainBlockedMask
	w := New(&assets.Catalog{
		Maps:     map[uint16]assets.Map{12000: {ID: 12000, NPCs: []assets.MapNPC{{ClickID: 1, Template: 17001, Name: "Guard", X: 10, Y: 30, WalkBehavior: 5, WalkSteps: []assets.WalkStep{{X: 90, Y: 30, Delay: 2000}, {X: 10, Y: 70, Delay: 2000}, {X: 10, Y: 10, Delay: 2000}}}}}},
		Terrains: map[uint16]assets.Terrain{12000: {Width: 100, Height: 100, GridWidth: 5, GridHeight: 5, Cells: cells}},
	})
	now := time.Unix(1, 0)
	draw := func(int) int { return 0 }
	w.AdvanceActors(12000, now, nil, draw)
	if got := w.AdvanceActors(12000, now.Add(time.Second), nil, draw); len(got) != 0 {
		t.Fatal("patrol crossed blocked leg", got)
	}
	n, _ := w.NPC(12000, 1)
	if n.X != 10 || n.Y != 30 {
		t.Fatal("blocked leg changed position", n)
	}
	for _, tt := range []struct {
		after time.Duration
		y     uint16
	}{{3 * time.Second, 70}, {5 * time.Second, 10}, {9 * time.Second, 70}} {
		got := w.AdvanceActors(12000, now.Add(tt.after), nil, draw)
		if len(got) != 1 || got[0].X != 10 || got[0].Y != tt.y {
			t.Fatal("patrol loop", tt, got)
		}
		// Skip the blocked waypoint when the route wraps.
		if tt.after == 5*time.Second {
			w.AdvanceActors(12000, now.Add(7*time.Second), nil, draw)
		}
	}
}

func TestWorldSimulationRoamingLeashAndTown(t *testing.T) {
	def := assets.MapNPC{ClickID: 1, Template: 17001, Name: "Guard", X: 100, Y: 100, WalkBehavior: 4}
	w := New(&assets.Catalog{Maps: map[uint16]assets.Map{12000: {ID: 12000, NPCs: []assets.MapNPC{def}}, 12001: {ID: 12001, NPCs: []assets.MapNPC{def}}}})
	now := time.Unix(1, 0)
	draw := func(n int) int { return n - 1 }
	w.AdvanceActors(12000, now, nil, draw)
	w.AdvanceActors(12001, now, nil, draw)
	for _, tt := range []struct {
		after time.Duration
		pos   uint16
	}{{10 * time.Second, 140}, {20 * time.Second, 120}} {
		got := w.AdvanceActors(12000, now.Add(tt.after), nil, draw)
		if len(got) != 1 || got[0].X != tt.pos || got[0].Y != tt.pos {
			t.Fatal("roam/leash", tt, got)
		}
		if got = w.AdvanceActors(12001, now.Add(tt.after), nil, draw); len(got) != 0 {
			t.Fatal("town actor roamed", got)
		}
	}
}
