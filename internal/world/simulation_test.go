package world

import (
	"bytes"
	"fmt"
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
	moves = w.AdvanceActors(12000, now.Add(5*time.Second), map[uint16]bool{3: true}, random)
	if len(moves) != 1 || moves[0].X != 100 {
		t.Fatal("single waypoint return", moves)
	}
	w.Defeat(12000, 1, now.Add(6*time.Second))
	if len(w.AdvanceActors(12000, now.Add(20*time.Second), map[uint16]bool{3: true}, random)) != 0 {
		t.Fatal("defeated actor moved")
	}
	revived := now.Add(6*time.Second + MonsterRespawn)
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
	}{{3 * time.Second, 70}, {5 * time.Second, 10}, {12 * time.Second, 70}} {
		got := w.AdvanceActors(12000, now.Add(tt.after), nil, draw)
		if len(got) != 1 || got[0].X != 10 || got[0].Y != tt.y {
			t.Fatal("patrol loop", tt, got)
		}
		// Skip the blocked waypoint when the route wraps.
		if tt.after == 5*time.Second {
			w.AdvanceActors(12000, now.Add(8*time.Second), nil, draw)
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
	}{{3 * time.Second, 250}, {13 * time.Second, 50}} {
		if tt.pos == 50 {
			draw = func(int) int { return 0 }
		}
		got := w.AdvanceActors(12000, now.Add(tt.after), nil, draw)
		if len(got) != 1 || got[0].X != tt.pos || got[0].Y != tt.pos {
			t.Fatal("roam/leash", tt, got)
		}
		if got = w.AdvanceActors(12001, now.Add(tt.after), nil, draw); len(got) != 0 {
			t.Fatal("town actor roamed", got)
		}
	}
}

func TestWorldSimulationBoundedLongerLegsAndFrequency(t *testing.T) {
	w := simulationFixture()
	now := time.Unix(1000, 0)
	w.AdvanceActors(12000, now, nil, func(int) int { return 0 })
	// The first candidate is nearby; subsequent candidates reach the far edge.
	draws := 0
	random := func(n int) int {
		draws++
		if draws == 2 || draws == 3 {
			return 0
		}
		return n - 1
	}
	moves := w.AdvanceActors(12000, now.Add(time.Second), map[uint16]bool{1: true}, random)
	if len(moves) != 1 || moves[0].X != 120 || moves[0].Y != 120 {
		t.Fatal("longest leg not selected", moves)
	}
	if got := w.AdvanceActors(12000, now.Add(3*time.Second), map[uint16]bool{1: true}, func(int) int { return 0 }); len(got) != 0 {
		t.Fatal("moved before deadline", got)
	}
	if got := w.AdvanceActors(12000, now.Add(5*time.Second), map[uint16]bool{1: true}, func(int) int { return 0 }); len(got) != 1 || got[0].X != 99 {
		t.Fatal("did not resume within three seconds", got)
	}
	bounds, ok := w.ActorTargetArea(12000, 3)
	if !ok || bounds.MinX != 99 || bounds.MaxX != 120 || bounds.Contains(121, 100) {
		t.Fatal(bounds, ok)
	}
}

func TestWorldSimulationBoundedPursuitLimitsAndReservations(t *testing.T) {
	w := simulationFixture()
	def := w.maps[12000].data.NPCs[2]
	def.WalkSteps = []assets.WalkStep{{X: uint32(0xffffffce), Y: uint32(0xffffffce)}, {X: 300, Y: 300}}
	w.maps[12000].data.NPCs[2] = def
	now := time.Unix(1000, 0)
	random := func(int) int { return 0 }
	targets := map[uint16]ActorPosition{3: {X: 400, Y: 100}}
	moves := w.AdvanceActorsToward(12000, now, nil, targets, random)
	if len(moves) != 1 || moves[0].X != 200 || moves[0].Y != 100 {
		t.Fatal("pursuit did not take a bounded step", moves)
	}
	bounds, _ := w.ActorTargetArea(12000, 3)
	if bounds.MaxX != 400 {
		t.Fatal("bounds drifted with actor", bounds)
	}
	if got := w.AdvanceActorsToward(12000, now.Add(2*time.Second), map[uint16]bool{1: true, 3: true}, targets, random); len(got) != 0 {
		t.Fatal("reserved actor chased", got)
	}
	w.HoldEncounter(12000, 3)
	if got := w.AdvanceActorsToward(12000, now.Add(3*time.Second), map[uint16]bool{1: true}, targets, random); len(got) != 0 {
		t.Fatal("hidden actor chased", got)
	}
	if got := w.Revive(now.Add(time.Hour)); len(got) != 0 {
		t.Fatal("battle reservation expired", got)
	}
	w.ReleaseEncounter(12000, 3, now)
	w.ReleaseEncounter(12000, 3, now.Add(time.Hour))
	if got := w.Revive(now.Add(EncounterRespawn - time.Millisecond)); len(got) != 0 {
		t.Fatal("early respawn", got)
	}
	if got := w.Revive(now.Add(EncounterRespawn)); len(got[12000]) != 1 {
		t.Fatal("short respawn missing", got)
	}
	n, _ := w.NPC(12000, 3)
	if n.X != 100 || n.Y != 100 {
		t.Fatal("respawn did not restore spawn", n)
	}
}

func TestWorldSimulationBoundedPursuitRejectsWallsAndOutsideTargets(t *testing.T) {
	w := simulationFixture()
	cells := make([]byte, 100)
	for y := range 10 {
		cells[5*10+y] = assets.TerrainBlockedMask
	}
	w.catalog.Terrains = map[uint16]assets.Terrain{12000: {Width: 200, Height: 200, GridWidth: 10, GridHeight: 10, Cells: cells}}
	// Move the spawn off the blocked column and author a rectangle across it.
	w.maps[12000].NPCs[2].X = 80
	w.maps[12000].data.NPCs[2].X = 80
	w.maps[12000].data.NPCs[2].WalkSteps = []assets.WalkStep{{}, {X: 100, Y: 60}}
	now := time.Unix(1000, 0)
	random := func(int) int { return 0 }
	w.AdvanceActors(12000, now, nil, random)
	w.AdvanceActorsToward(12000, now.Add(time.Second), map[uint16]bool{1: true}, map[uint16]ActorPosition{3: {X: 150, Y: 150}}, random)
	n, _ := w.NPC(12000, 3)
	if n.X != 80 || n.Y != 150 {
		t.Fatal("blocked diagonal failed to slide along clear axis", n)
	}
	w.AdvanceActorsToward(12000, now.Add(4*time.Second), map[uint16]bool{1: true}, map[uint16]ActorPosition{3: {X: 190, Y: 190}}, random)
	n, _ = w.NPC(12000, 3)
	if n.X != 80 || n.Y != 100 {
		t.Fatal("outside target was chased", n)
	}
}

func TestWorldSimulationRoamerAndPatrolPursuitAreas(t *testing.T) {
	for _, tt := range []struct {
		name     string
		behavior byte
		path     []assets.WalkStep
		outside  bool
	}{
		{"roamer", 4, nil, false}, {"roamer outside", 4, nil, true},
		{"patrol", 5, []assets.WalkStep{{X: 1300, Y: 1000}, {X: 1000, Y: 1000}}, false},
		{"pathless patrol", 5, nil, false}, {"pathless patrol outside", 5, nil, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			def := assets.MapNPC{ClickID: 1, Template: 17001, Name: "Rabbit", X: 1000, Y: 1000, WalkBehavior: tt.behavior, WalkSteps: tt.path}
			w := New(&assets.Catalog{Maps: map[uint16]assets.Map{60000: {ID: 60000, NPCs: []assets.MapNPC{def}}}})
			now := time.Unix(1000, 0)
			random := func(int) int { return 0 }
			target := ActorPosition{X: 1050, Y: 1000}
			if len(tt.path) > 0 {
				target.X = 1150
			}
			if tt.outside {
				target.X = 1151
			}
			area, ok := w.ActorTargetArea(60000, 1)
			if !ok || area.Contains(target.X, target.Y) == tt.outside {
				t.Fatal("incorrect acquisition area", area)
			}
			moves := w.AdvanceActorsToward(60000, now, nil, map[uint16]ActorPosition{1: target}, random)
			if tt.outside {
				moves = w.AdvanceActorsToward(60000, now.Add(time.Second), nil, map[uint16]ActorPosition{1: target}, random)
			}
			if len(moves) != 1 {
				t.Fatal(moves)
			}
			wantX, wantY := uint16(1050), uint16(1000)
			if len(tt.path) > 0 {
				wantX = 1100
			}
			if tt.outside {
				wantX, wantY = 850, 850
			}
			if moves[0].X != wantX || moves[0].Y != wantY {
				t.Fatal("pursuit/territory", moves)
			}
			if len(tt.path) > 0 {
				moves = w.AdvanceActors(60000, now.Add(4*time.Second), nil, random)
				if len(moves) != 1 || moves[0].X != 1300 {
					t.Fatal("patrol cursor did not resume", moves)
				}
			}
			if tt.outside {
				// Moving nearer to an outside target must not move the territory.
				area, _ = w.ActorTargetArea(60000, 1)
				if area.MaxX != 1150 || area.MinX != 850 {
					t.Fatal("territory drifted", area)
				}
			}
		})
	}
}

func TestWorldSimulationStationaryWaitAfterArrival(t *testing.T) {
	def := assets.MapNPC{ClickID: 1, Template: 17001, Name: "Rabbit", X: 1000, Y: 1000, WalkBehavior: 5, WalkSteps: []assets.WalkStep{{X: 1100, Y: 1000, Delay: 10000}}}
	w := New(&assets.Catalog{Maps: map[uint16]assets.Map{60000: {ID: 60000, NPCs: []assets.MapNPC{def}}}})
	now := time.Unix(1000, 0)
	low := func(int) int { return 0 }
	high := func(n int) int { return n - 1 }
	w.AdvanceActors(60000, now, nil, low)
	if got := w.AdvanceActors(60000, now.Add(time.Second), nil, high); len(got) != 1 || got[0].X != 1100 {
		t.Fatal(got)
	}
	// Arrival is 3.5 seconds; maximum stationary wait ends at 6.5.
	if got := w.AdvanceActors(60000, now.Add(6500*time.Millisecond-time.Millisecond), nil, low); len(got) != 0 {
		t.Fatal("maximum wait ended early", got)
	}
	if got := w.AdvanceActors(60000, now.Add(6500*time.Millisecond), nil, low); len(got) != 1 || got[0].X != 1000 {
		t.Fatal("maximum wait did not end", got)
	}
	// Return arrival is 9 seconds; minimum stationary wait ends at 10.
	if got := w.AdvanceActors(60000, now.Add(10*time.Second-time.Millisecond), nil, low); len(got) != 0 {
		t.Fatal("minimum wait ended early", got)
	}
	if got := w.AdvanceActors(60000, now.Add(10*time.Second), nil, low); len(got) != 1 || got[0].X != 1100 {
		t.Fatal("minimum wait did not end", got)
	}
}

func TestWorldSimulationWalkingDestinationHeldUntilArrival(t *testing.T) {
	w := New(&assets.Catalog{Maps: map[uint16]assets.Map{60000: {ID: 60000, NPCs: []assets.MapNPC{{ClickID: 1, Template: 17001, Name: "Rabbit", X: 1000, Y: 1000, WalkBehavior: 5, WalkSteps: []assets.WalkStep{{X: 1300, Y: 1000}}}}}}})
	now := time.Unix(1000, 0)
	low := func(int) int { return 0 }
	w.AdvanceActors(60000, now, nil, low)
	if got := w.AdvanceActors(60000, now.Add(time.Second), nil, low); len(got) != 1 || got[0].X != 1300 {
		t.Fatal("walking leg not issued", got)
	}
	// Native speed byte 2 is 40 px/s: this 300-pixel leg needs 7.5 s.
	arrival := now.Add(8500 * time.Millisecond)
	if !w.ActorMoving(60000, 1, arrival.Add(-time.Millisecond)) || w.ActorMoving(60000, 1, arrival) {
		t.Fatal("incorrect native arrival estimate")
	}
	noDraw := func(int) int { t.Fatal("unfinished walking leg selected another destination"); return 0 }
	if got := w.AdvanceActors(60000, now.Add(3*time.Second), nil, noDraw); len(got) != 0 {
		t.Fatal("pursuit replaced walking destination", got)
	}
	if got := w.AdvanceActors(60000, arrival, nil, noDraw); len(got) != 0 {
		t.Fatal("arrival skipped stationary wait", got)
	}
	if got := w.AdvanceActors(60000, arrival.Add(time.Second), nil, low); len(got) != 1 || got[0].X != 1000 {
		t.Fatal("patrol did not resume on arrival", got)
	}
	w.HoldEncounter(60000, 1)
	w.ReleaseEncounter(60000, 1, arrival)
	w.Revive(arrival.Add(EncounterRespawn))
	if w.ActorMoving(60000, 1, arrival.Add(EncounterRespawn)) {
		t.Fatal("respawn retained old walking leg")
	}
}

func TestWorldSimulationNativeWalkingTravelTime(t *testing.T) {
	for _, tt := range []struct {
		x, y uint16
		want time.Duration
	}{
		{1000, 1000, 0}, {1100, 1000, 2500 * time.Millisecond},
		{1300, 1400, 12500 * time.Millisecond}, {900, 1000, 2500 * time.Millisecond},
	} {
		if got := actorTravelTime(1000, 1000, tt.x, tt.y); got != tt.want {
			t.Fatal("native travel timing", tt, got)
		}
	}
}

func TestWorldSimulationPlayerInterruptsWandering(t *testing.T) {
	for _, targetX := range []uint16{1100, 1080} {
		t.Run(fmt.Sprint(targetX), func(t *testing.T) {
			w := New(&assets.Catalog{Maps: map[uint16]assets.Map{60000: {ID: 60000, NPCs: []assets.MapNPC{{ClickID: 1, Template: 17001, Name: "Rabbit", X: 1000, Y: 1000, WalkBehavior: 5, WalkSteps: []assets.WalkStep{{X: 1300, Y: 1000}}}}}}})
			now := time.Unix(1000, 0)
			low := func(int) int { return 0 }
			w.AdvanceActors(60000, now, nil, low)
			w.AdvanceActors(60000, now.Add(time.Second), nil, low)
			at := now.Add(3 * time.Second)
			n, _ := w.NPCAt(60000, 1, at)
			if n.X != 1080 {
				t.Fatal("incorrect walking position", n)
			}
			got := w.AdvanceActorsToward(60000, at, nil, map[uint16]ActorPosition{1: {X: targetX, Y: 1000}}, low)
			if len(got) != 1 || got[0].X != targetX {
				t.Fatal("player did not interrupt wandering", got)
			}
			n, _ = w.NPCAt(60000, 1, at)
			if n.X != 1080 {
				t.Fatal("interruption teleported actor", n)
			}
			if targetX == 1100 {
				n, _ = w.NPCAt(60000, 1, at.Add(250*time.Millisecond))
				if n.X != 1090 {
					t.Fatal("incorrect interrupted interpolation", n)
				}
				noDraw := func(int) int { t.Fatal("active player leg interrupted"); return 0 }
				w.AdvanceActorsToward(60000, at.Add(250*time.Millisecond), nil, map[uint16]ActorPosition{1: {X: 1200, Y: 1000}}, noDraw)
			} else if w.ActorMoving(60000, 1, at) {
				t.Fatal("actor did not stop at player")
			}
		})
	}
}
