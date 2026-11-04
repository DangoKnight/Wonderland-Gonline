package app

import (
	"os"
	"reflect"
	"testing"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/internal/protocol"
)

func comboDamageMotions() *clientbattle.Assets {
	a := clientbattle.Motion{DamageMode: 2, Frames: []clientbattle.MotionFrame{{Hold: time.Second}}, Events: []clientbattle.MotionEvent{{Trigger: 150, Percent: 33}, {Trigger: 300, Percent: 33}, {Trigger: 450, Percent: 34}}}
	return &clientbattle.Assets{Motions: map[uint16]clientbattle.Motion{
		10001: {Frames: []clientbattle.MotionFrame{{Hold: time.Second}}, Events: []clientbattle.MotionEvent{{Trigger: 120, Percent: 100}}},
		11001: a,
		11002: {DamageMode: 2, Frames: []clientbattle.MotionFrame{{Hold: time.Second}}, Events: []clientbattle.MotionEvent{{Trigger: 200, Percent: 20}, {Trigger: 400, Percent: 40}, {Trigger: 600, Percent: 40}}},
		11003: a, // Different skills remain separate even with equal timing.
	}}
}

func damagePlay(skill uint16, amount uint32) battlePlay {
	return battlePlay{action: clientbattle.Action{Batch: 7, Skill: skill, Targets: []clientbattle.Target{{Cell: clientbattle.Cell{X: 2, Y: 2}, Generation: 1, Result: 1, Stats: []clientbattle.StatResult{{Stat: 25, Amount: amount, Mode: 1}}}}}}
}

func TestBattleDamageComboGrouping(t *testing.T) {
	for _, test := range []struct {
		name  string
		plays []battlePlay
		want  [][]uint64
	}{
		{"singles", []battlePlay{damagePlay(10001, 100), damagePlay(10001, 200)}, [][]uint64{{300}}},
		{"single plus multi", []battlePlay{damagePlay(10001, 200), damagePlay(11001, 100)}, [][]uint64{{99, 99, 102}}},
		{"identical multis", []battlePlay{damagePlay(11001, 100), damagePlay(11001, 200)}, [][]uint64{{99, 99, 102}}},
		{"identical multis plus single", []battlePlay{damagePlay(11001, 100), damagePlay(10001, 100), damagePlay(11001, 100)}, [][]uint64{{99, 99, 102}}},
		{"different multis", []battlePlay{damagePlay(11001, 100), damagePlay(11002, 200)}, [][]uint64{{33, 33, 34}, {40, 80, 80}}},
		{"different multis plus singles", []battlePlay{damagePlay(11001, 100), damagePlay(11002, 200), damagePlay(10001, 50), damagePlay(10001, 70)}, [][]uint64{{33, 33, 34}, {40, 80, 80}, {120}}},
		{"mixed repeated families", []battlePlay{damagePlay(11001, 100), damagePlay(11002, 100), damagePlay(11001, 200), damagePlay(10001, 50)}, [][]uint64{{99, 99, 102}, {20, 40, 40}, {50}}},
		{"different skills same timing", []battlePlay{damagePlay(11001, 100), damagePlay(11003, 200)}, [][]uint64{{33, 33, 34}, {66, 66, 68}}},
		{"wide total", []battlePlay{damagePlay(10001, 4294967295), damagePlay(10001, 4294967295)}, [][]uint64{{8589934590}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			planBattleDamage(test.plays, comboDamageMotions())
			var got [][]uint64
			for _, play := range test.plays {
				for _, group := range play.damage {
					got = append(got, group.parts)
				}
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("groups %v, want %v", got, test.want)
			}
		})
	}
}

func TestBattleDamageGroupingBoundaries(t *testing.T) {
	for _, boundary := range []string{"packet", "unbatched", "target", "generation", "stat"} {
		t.Run(boundary, func(t *testing.T) {
			plays := []battlePlay{damagePlay(10001, 30), damagePlay(10001, 40)}
			switch boundary {
			case "packet":
				plays[1].action.Batch++
			case "unbatched":
				plays[0].action.Batch, plays[1].action.Batch = 0, 0
			case "target":
				plays[1].action.Targets[0].X = 1
			case "generation":
				plays[1].action.Targets[0].Generation++
			case "stat":
				plays[1].action.Targets[0].Stats[0].Stat = 26
			}
			planBattleDamage(plays, comboDamageMotions())
			if len(plays[0].damage) != 1 || len(plays[1].damage) != 1 {
				t.Fatal("unrelated results combined")
			}
		})
	}
	plays := []battlePlay{damagePlay(10001, 30), damagePlay(11001, 40), damagePlay(11002, 100)}
	plays[2].action.Targets[0].Result = 0 // A missed skill is not a damage family.
	plays[0].action.Targets[0].Stats[0].Mode = 2
	planBattleDamage(plays, comboDamageMotions())
	if len(plays[1].damage) != 1 || !plays[1].damage[0].critical || len(plays[2].damage) != 0 {
		t.Fatal("miss/critical grouping")
	}
	plays = []battlePlay{damagePlay(10001, 30), damagePlay(11001, 40)}
	plays[1].action.Targets[0].Stats[0].Mode = 3
	planBattleDamage(plays, comboDamageMotions())
	if len(plays[0].damage) != 0 || len(plays[1].damage) != 0 {
		t.Fatal("damage/recovery stat order was grouped")
	}
}

func TestBattleDamageNativeMotionFamilies(t *testing.T) {
	assets, err := clientbattle.Load(login.NewAssets("../../../data"))
	if err != nil {
		t.Fatal(err)
	}
	plays := []battlePlay{damagePlay(10001, 100), damagePlay(11004, 100)} // Water Hit: four contacts.
	planBattleDamage(plays, assets)
	if len(plays[0].damage) != 0 || len(plays[1].damage) != 1 || !reflect.DeepEqual(plays[1].damage[0].parts, []uint64{50, 50, 50, 50}) {
		t.Fatal("native Water Hit did not pool the single")
	}
	plays = []battlePlay{damagePlay(11004, 100), damagePlay(11076, 100), damagePlay(10001, 100)}
	planBattleDamage(plays, assets)
	for i, expected := range [][]uint64{{25, 25, 25, 25}, {33, 33, 34}, {100}} {
		if len(plays[i].damage) != 1 || !reflect.DeepEqual(plays[i].damage[0].parts, expected) {
			t.Fatalf("native mixed family %d: %v", i, plays[i].damage)
		}
	}
}

func TestBattleDamagePopupStackAndIdentity(t *testing.T) {
	c := &Client{}
	cell := clientbattle.Cell{X: 2, Y: 2}
	c.battle.shown = map[clientbattle.Cell]clientbattle.Fighter{cell: {Cell: cell, HP: 1000, MaxHP: 1000, Generation: 1}}
	at := time.Unix(100, 0)
	group := &battleDamageGroup{target: cell, generation: 1, stat: 25, parts: []uint64{33, 33, 34}}
	for cue := 0; cue < 3; cue++ {
		c.battleDamageHit(group, cue, at.Add(time.Second), at.Add(time.Duration(cue)*150*time.Millisecond))
	}
	if c.battle.shown[cell].HP != 900 || len(c.battle.floats) != 3 {
		t.Fatal("lost split damage")
	}
	for i, want := range []int{3, -4, 5} {
		f := c.battle.floats[i]
		if f.flight != want || f.rise != 0 {
			t.Fatal("multi-hit did not fan out", i, f.flight, f.rise)
		}
	}
	group.generation = 2
	c.battleDamageHit(group, 0, at, at)
	if c.battle.shown[cell].HP != 900 || len(c.battle.floats) != 3 {
		t.Fatal("stale target was damaged")
	}
}

func TestBattleDamagePreservesInterleavedRecovery(t *testing.T) {
	plays := []battlePlay{damagePlay(10001, 50), damagePlay(10001, 10)}
	plays[0].action.Targets[0].Stats = append(plays[0].action.Targets[0].Stats, clientbattle.StatResult{Stat: 25, Amount: 25, Mode: 3})
	planBattleDamage(plays, comboDamageMotions())
	if len(plays[0].damage) != 0 || len(plays[1].damage) != 0 {
		t.Fatal("interleaved recovery was reordered")
	}
	c := &Client{}
	cell := clientbattle.Cell{X: 2, Y: 2}
	c.battle.shown = map[clientbattle.Cell]clientbattle.Fighter{cell: {Cell: cell, Generation: 1, HP: 40, MaxHP: 100}}
	at := time.Unix(100, 0)
	for i := range plays {
		c.battleHit(&plays[i], at, at, clientbattle.Motion{}, 0)
	}
	if c.battle.shown[cell].HP != 15 {
		t.Fatal("damage/recovery clamping changed", c.battle.shown[cell].HP)
	}
}

func TestBattleDamageNativeAndChainContact(t *testing.T) {
	for _, chain := range []bool{false, true} {
		name := "native"
		if chain {
			name = "chain"
		}
		t.Run(name, func(t *testing.T) {
			c, now, _ := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
			c.battleFrame(false)
			motions := comboDamageMotions()
			motions.Backgrounds = c.battle.assets.Backgrounds
			c.battle.assets = motions
			start := *now
			// Independent native combo: 60 multi-hit damage + 30 single-hit.
			packet := []byte{50, 1, 17, 0, 4, 2, 249, 42, 0, 1, 2, 2, 1, 0, 1, 25, 60, 0, 0, 0, 1, 17, 0, 3, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 30, 0, 0, 0, 1}
			if chain {
				c.combat.enabled = true
				deliverCombatRound(t, c, 2, []protocol.CombatStep{{Packets: [][]byte{packet}}})
			} else {
				c.dispatch(packet)
			}
			c.battleFrame(false)
			cell := clientbattle.Cell{X: 2, Y: 2}
			for _, step := range []struct {
				ms     int
				hp     uint32
				floats int
			}{{120, 100, 0}, {149, 100, 0}, {150, 70, 1}, {300, 40, 2}, {450, 10, 3}} {
				*now = start.Add(time.Duration(step.ms) * time.Millisecond)
				c.battleFrame(false)
				if hp := c.battle.shown[cell].HP; hp != step.hp || len(c.battle.floats) != step.floats {
					t.Fatalf("at %dms HP %d floats %d", step.ms, hp, len(c.battle.floats))
				}
				c.battleFrame(false)
				if c.battle.shown[cell].HP != step.hp || len(c.battle.floats) != step.floats {
					t.Fatal("duplicate contact")
				}
			}
			for _, popup := range c.battle.floats {
				if popup.amount != 30 {
					t.Fatal("combo damage did not split evenly", popup.amount)
				}
			}
			for i, want := range []int{3, -4, 5} {
				if c.battle.floats[i].flight != want {
					t.Fatal("combo fan direction", i, c.battle.floats[i].flight)
				}
			}
			if path := os.Getenv("BATTLE_DAMAGE_SNAPSHOT"); path != "" && !chain {
				c.battleFrame(true)
				savePNG(t, path, c)
			}
		})
	}
}

func TestBattleDamageFlightMirrorsAndUsesContactAge(t *testing.T) {
	at := time.Unix(100, 0)
	for _, x := range []byte{2, 4} {
		c := &Client{}
		cell := clientbattle.Cell{X: x, Y: 2}
		for hit := 0; hit < 4; hit++ {
			c.battleDamagePopup(cell, 1, 25, false, 30, at.Add(time.Second), at, hit, 4)
		}
		for i, f := range c.battle.floats {
			start := f.displacement(at)
			mid := f.displacement(at.Add(500 * time.Millisecond))
			end := f.displacement(at.Add(time.Second))
			if start.X != 0 || start.Y != 0 || mid.X == 0 || mid.Y >= 0 || end.Y <= mid.Y {
				t.Fatal("damage flight did not arc", start, mid, end)
			}
			want := []int{3, -4, 5, -6}[i]
			if x == 4 {
				want = -want
			}
			if f.flight != want || mid.X != 24*want {
				t.Fatal("wrong mirrored lane", x, i, mid)
			}
		}
	}
	f := battleFloat{at: at}
	if got := f.displacement(at.Add(300 * time.Millisecond)); got.X != 0 || got.Y != -10 {
		t.Fatal("single-hit/recovery motion changed", got)
	}
}
