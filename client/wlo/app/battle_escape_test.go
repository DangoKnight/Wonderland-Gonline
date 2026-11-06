package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

func TestBattleEscapeFailurePlaysNativeFall(t *testing.T) {
	for _, pet := range []bool{false, true} {
		t.Run(map[bool]string{false: "player", true: "pet"}[pet], func(t *testing.T) {
			c, now, _ := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
			if bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE"); bundle != "" {
				useCompiledPetAssets(t, c, bundle)
			}
			c.npcTemplates, _ = world.NPCTemplates(c.Assets)
			c.battleFrame(false)
			cell := clientbattle.Cell{X: 4, Y: 2}
			if pet {
				cell.X = 3
			}
			// Native motion 60046, no stat results: failure trips without damage.
			p := []byte{50, 1, 11, 0, cell.X, cell.Y, 142, 234, 0, 1, cell.X, cell.Y, 1, 0, 0}
			c.dispatch(p)
			c.battleFrame(false)
			if c.battle.lastActions[cell].skill != 60041 {
				t.Fatal("failed flee replaced the repeat command with its animation ID")
			}
			motion := c.battle.assets.Motions[60046]
			if len(c.battle.floats) != 0 {
				t.Fatal("failure notification appeared before falling")
			}
			var fall time.Duration
			for at := time.Duration(0); at < motion.Duration(); at += 10 * time.Millisecond {
				pose, _ := motion.PoseAt(cell, clientbattle.Cell{}, at, c.battleBody(*c.battle.state.At(cell)).moveKind)
				if pose == 26 || pose == 27 {
					fall = at
					break
				}
			}
			if fall == 0 {
				t.Fatal("native failure motion has no falling pose")
			}
			*now = c.battle.playing[0].at.Add(fall)
			c.battleFrame(false)
			if len(c.battle.floats) != 1 || !c.battle.floats[0].failure {
				t.Fatal("missing failure notification at fall")
			}
			c.battleFrame(false)
			if len(c.battle.floats) != 1 {
				t.Fatal("failure notification duplicated")
			}
			if !c.battle.state.Active || c.battle.state.At(cell).HP != 100 || len(c.battle.playing) != 1 || len(c.battle.departures) != 0 {
				t.Fatal("escape failure damaged or removed actor")
			}
			if dir := os.Getenv("BATTLE_ESCAPE_SNAPSHOT"); dir != "" {
				c.battleFrame(true)
				c.Sprites.WaitNative()
				c.battleFrame(true)
				savePNG(t, filepath.Join(dir, map[bool]string{false: "player", true: "pet"}[pet]+"-failed-escape.png"), c)
			}
			*now = now.Add(motion.Duration() + time.Second)
			c.battleFrame(false)
			if !c.battle.state.Active || c.battle.state.At(cell).Removed {
				t.Fatal("failure ended combat")
			}
		})
	}
}

func TestBattleEscapeOwnerPetAndPeerScope(t *testing.T) {
	for _, own := range []bool{false, true} {
		for _, bundled := range []bool{false, true} {
			owner := uint32(10002)
			y := byte(3)
			if own {
				owner = 10001
				y = 2
			}
			c, now, _ := enterBattle(t, remoteRecord(5, 2, 4, 3, 10002, 0), remoteRecord(5, 4, 3, y, 14162, owner))
			c.battleFrame(false)
			packets := [][]byte{{11, 1, 3, y}, {11, 1, 4, y}}
			if bundled {
				c.combat.enabled = true
				deliverCombatRound(t, c, 1, []protocol.CombatStep{{Packets: packets}})
			} else {
				for _, p := range packets {
					c.dispatch(p)
				}
			}
			c.battleFrame(false)
			if len(c.battle.departures) != 2 || len(c.battle.transitionPackets) != 0 {
				t.Fatal("owner and pet did not depart together")
			}
			closePacket := []byte{11, 0, byte(owner), byte(owner >> 8), byte(owner >> 16), byte(owner >> 24), 0, 0}
			c.dispatch(closePacket)
			*now = now.Add(c.battleDepartureMotion().Duration() + time.Second)
			for i := 0; i < 4; i++ {
				c.battleFrame(false)
			}
			if own && c.battle.state.Active || !own && (!c.battle.state.Active || c.battle.state.At(clientbattle.Cell{X: 4, Y: 2}).Removed) {
				t.Fatal("member escape closed wrong battle", own, bundled)
			}
		}
	}
}
