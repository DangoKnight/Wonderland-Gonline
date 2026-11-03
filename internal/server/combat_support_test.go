package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestCombatManaNativeAndChainPlaybackAndDisconnectSave(t *testing.T) {
	for _, chain := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "chain"}[chain], func(t *testing.T) {
			s, c, w := battleFixture(t, 10000)
			run := c.battle
			run.timer.Stop()
			next := c.character.Clone()
			next.Skills = []game.LearnedSkill{{ID: 15075, Grade: 1}}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			s.Assets.Skills[15075] = assets.Skill{ID: 15075, Name: "Magic Recovery", EffectLayer: 6, SP: 0}
			self := run.member(c).self
			self.SP = 1
			self.Spd = 10000
			if chain {
				c.combatCapabilities = protocol.CombatCapabilityRoundChain
			}
			w.Reset()
			if err := s.worldCommand(context.Background(), c, []byte{50, 1, 4, 2, 4, 2, 227, 58}); err != nil {
				t.Fatal(err)
			}
			settle(t, s, c)
			packets := w.packets(t)
			if chain {
				var native [][]byte
				for _, step := range decodeRoundPackets(t, packets) {
					native = append(native, step.Packets...)
				}
				packets = native
			}
			found := false
			for _, p := range packets {
				if len(p) == 21 && p[0] == 50 && p[1] == 1 && p[6] == 227 && p[7] == 58 && p[15] == 26 && p[20] == 3 {
					found = true
				}
			}
			if !found || self.SP <= 1 {
				t.Fatal("mana result absent from transport", chain, packets)
			}
			want := uint32(self.SP)
			s.abandonBattle(c)
			stored, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || stored[0].SP != want {
				t.Fatal("resolved mana not saved on disconnect", stored, err, want)
			}
		})
	}
}
