package app

import (
	"bytes"
	"testing"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/internal/protocol"
)

func deliverCombatRound(t *testing.T, c *Client, id uint32, steps []protocol.CombatStep) {
	t.Helper()
	data, err := protocol.EncodeCombatRound(steps)
	if err != nil {
		t.Fatal(err)
	}
	packets, err := protocol.CombatRoundChunks(id, data)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range packets {
		c.dispatch(p)
	}
}

func TestGonlineCombatNegotiationAndConnectionReset(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	c.dispatch([]byte{1, 242, 71, 76, 66, 82, 1, 0, 1, 0, 0, 0})
	if c.combat.enabled {
		t.Fatal("unsolicited confirmation enabled transport")
	}
	c.dispatch([]byte{1, 240, 1}) // An old server's generic echo is insufficient.
	if len(sent()) != 0 {
		t.Fatal("legacy server received extension request")
	}
	c.dispatch([]byte{1, 240, 71, 76, 66, 82, 2, 0, 1, 0, 0, 0})
	if len(sent()) != 0 {
		t.Fatal("unsupported version accepted")
	}
	c.dispatch([]byte{1, 240, 71, 76, 66, 82, 1, 0, 1, 0, 0, 0})
	if p := sent(); len(p) != 1 || !bytes.Equal(p[0], []byte{1, 241, 71, 76, 66, 82, 1, 0, 1, 0, 0, 0}) {
		t.Fatal("acceptance", p)
	}
	if c.combat.enabled {
		t.Fatal("enabled before confirmation")
	}
	c.dispatch([]byte{1, 242, 71, 76, 66, 82, 1, 0, 1, 0, 0, 0})
	if !c.combat.enabled {
		t.Fatal("confirmation ignored")
	}
	c.Servers.OnConnecting()
	if c.combat.enabled || c.combat.offered || c.combat.lastSequence != 0 {
		t.Fatal("capabilities leaked to next connection")
	}
}

func TestGonlineCombatOneRequestForPlayerAndPet(t *testing.T) {
	c, _, sent := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
	c.combat.enabled = true
	c.dispatch([]byte{254, 3, 9, 0, 0, 0, 4, 2})
	self := []byte{50, 1, 4, 2, 2, 2, 17, 39}
	pet := []byte{50, 4, 3, 2, 3, 2, 117, 234}
	if err := c.sendCombatChoice(self); err != nil {
		t.Fatal(err)
	}
	if len(sent()) != 0 {
		t.Fatal("sent before pet choice")
	}
	if err := c.sendCombatChoice(pet); err != nil {
		t.Fatal(err)
	}
	want := []byte{254, 2, 9, 0, 0, 0, 2, 50, 1, 4, 2, 2, 2, 17, 39, 50, 4, 3, 2, 3, 2, 117, 234}
	if p := sent(); len(p) != 1 || !bytes.Equal(p[0], want) {
		t.Fatal("round submission", p)
	}
	if err := c.sendCombatChoice(self); err == nil || len(sent()) != 1 {
		t.Fatal("duplicate submission")
	}
}

func TestGonlineCombatTimeoutPreservesPetChoice(t *testing.T) {
	c, _, sent := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
	c.combat.enabled = true
	c.dispatch([]byte{254, 3, 9, 0, 0, 0, 4, 2})
	pet := []byte{50, 1, 3, 2, 2, 2, 17, 39}
	if err := c.sendCombatChoice(pet); err != nil {
		t.Fatal(err)
	}
	c.flushCombatChoices()
	p := sent()
	if len(p) != 1 {
		t.Fatal("timeout batch count", p)
	}
	if !bytes.Equal(p[0], pet) || c.combat.choiceToken != 0 || len(c.combat.choices) != 0 {
		t.Fatal("timeout discarded choice or added an action", p)
	}
}

func TestGonlineCombatSequentialContactAndDeferredReadiness(t *testing.T) {
	c, now := overkillClient(t)
	sent := wire(t, c)
	c.combat.enabled = true
	attack := []byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 25, 0, 0, 0, 1}
	steps := []protocol.CombatStep{{Packets: [][]byte{attack}, Delay: time.Second}, {Packets: [][]byte{{51, 1, 4, 2, 26, 7, 0, 0, 0}}, Delay: time.Second}}
	deliverCombatRound(t, c, 2, steps)
	c.dispatch([]byte{254, 3, 3, 0, 0, 0, 4, 2}) // Server is ahead of local playback.
	if c.battle.state.At(clientbattle.Cell{X: 2, Y: 2}).HP != 100 || !c.battleAnimating() || c.remote.battle.ready {
		t.Fatal("chain applied at receipt")
	}
	c.battleFrame(false)
	if c.battle.state.At(clientbattle.Cell{X: 2, Y: 2}).HP != 75 || c.battle.shown[clientbattle.Cell{X: 2, Y: 2}].HP != 100 || c.combat.round.index != 1 {
		t.Fatal("first action contact timing")
	}
	*now = now.Add(150 * time.Millisecond)
	c.battleFrame(false)
	if c.battle.shown[clientbattle.Cell{X: 2, Y: 2}].HP != 75 || c.combat.round.index != 1 {
		t.Fatal("damage cue or ordering")
	}
	for i := 0; i < 60 && c.combat.round != nil; i++ {
		*now = now.Add(100 * time.Millisecond)
		c.battleFrame(false)
	}
	if c.combat.round != nil || !c.battle.state.Ready || c.combat.choiceToken != 3 || c.battle.state.At(clientbattle.Cell{X: 4, Y: 2}).SP != 7 {
		t.Fatal("deferred next round not released")
	}
	if len(sent()) != 0 {
		t.Fatal("client sent playback requests", sent())
	}
	deliverCombatRound(t, c, 2, steps)
	if c.combat.round != nil || c.battle.state.At(clientbattle.Cell{X: 2, Y: 2}).HP != 75 {
		t.Fatal("duplicate chain replayed")
	}
}

func TestGonlineCombatOverkillBeforeDeferredWarp(t *testing.T) {
	c, now := overkillClient(t)
	c.combat.enabled = true
	attack := []byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 151, 0, 0, 0, 1}
	deliverCombatRound(t, c, 2, []protocol.CombatStep{{Packets: [][]byte{attack}, Delay: time.Second}, {Packets: [][]byte{{53, 3, 2, 2}}, Delay: 300 * time.Millisecond}})
	c.dispatch([]byte{11, 0, 17, 39, 0, 0, 0, 0})
	c.dispatch(protocol.Builder{12}.U32(10001).U16(10017).U16(1111).U16(222).U16(0).U8(0))
	if c.World.Player.X == 1111 || !c.battle.state.Active {
		t.Fatal("ending interrupted queued round")
	}
	sawFlight := false
	for i := 0; i < 100 && c.battle.state.Active; i++ {
		c.battleFrame(false)
		if at, ok := c.battle.overkills[clientbattle.Cell{X: 2, Y: 2}]; ok && !at.IsZero() {
			sawFlight = true
		}
		*now = now.Add(100 * time.Millisecond)
	}
	if !sawFlight || c.battle.state.Active || c.World.Player.X != 1111 || c.combat.round != nil {
		t.Fatal("overkill/close/warp order", sawFlight, c.World.Player.X)
	}
}

func TestGonlineCombatComboOverkillAndReinforcement(t *testing.T) {
	c, now := overkillClient(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
	c.combat.enabled = true
	combo := []byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 80, 0, 0, 0, 1,
		17, 0, 3, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 80, 0, 0, 0, 1}
	replacement := remoteRecord(5, 7, 2, 2, 20001, 0)
	replacement[2] = 2
	deliverCombatRound(t, c, 2, []protocol.CombatStep{
		{Packets: [][]byte{combo}, Delay: time.Second},
		{Packets: [][]byte{{53, 3, 2, 2}}, Delay: 300 * time.Millisecond},
		{Packets: [][]byte{replacement}},
	})
	c.battleFrame(false)
	if len(c.battle.playing) != 2 {
		t.Fatal("combo was not simultaneous")
	}
	cell := clientbattle.Cell{X: 2, Y: 2}
	*now = now.Add(150 * time.Millisecond)
	c.battleFrame(false)
	if c.battle.shown[cell].HP != 0 {
		t.Fatal("combo damage cues")
	}
	sawFlight := false
	for i := 0; i < 70 && c.combat.round != nil; i++ {
		*now = now.Add(100 * time.Millisecond)
		c.battleFrame(false)
		if at, ok := c.battle.overkills[cell]; ok && !at.IsZero() {
			sawFlight = true
		}
	}
	f := c.battle.shown[cell]
	if !sawFlight || c.combat.round != nil || f.HP != 100 || f.Removed || f.Overkilled {
		t.Fatal("reinforcement inherited old damage/removal", f)
	}
}

func TestGonlineCombatInvalidLaterStepDoesNotApplyEarlierDamage(t *testing.T) {
	c, _ := overkillClient(t)
	c.combat.enabled = true
	attack := []byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 25, 0, 0, 0, 1}
	deliverCombatRound(t, c, 2, []protocol.CombatStep{{Packets: [][]byte{attack}}, {Packets: [][]byte{{51, 1, 2}}}})
	if c.combat.round != nil || c.battle.state.At(clientbattle.Cell{X: 2, Y: 2}).HP != 100 || len(c.battle.state.Actions) != 0 {
		t.Fatal("malformed round partially applied")
	}
}
