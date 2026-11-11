package app

import "wonderland-gonline/internal/localization"

import (
	"encoding/binary"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/internal/protocol"
)

type combatRuntime struct {
	offered      bool
	enabled      bool
	lastSequence uint32
	choiceToken  uint32
	choices      [][]byte
	assembly     protocol.CombatAssembly
	round        *combatPlayback
}

type combatPlayback struct {
	steps         []protocol.CombatStep
	index         int
	until         time.Time
	deferred      [][]byte
	deferredBytes int
}

// The native dispatcher is also the interpreter for packets inside a chain.
// Subsequent readiness/results/warps wait for local playback; chat and explicit
// disconnect reasons still arrive immediately. No playback acknowledgments exist.
func (c *Client) dispatch(p []byte) {
	if len(p) == 0 {
		return
	}
	if c.afkPacket(p) {
		return
	}
	// Apply the ordered peer refresh before its end fence, even during combat.
	if c.afk.resync && !c.afk.sceneReceived {
		c.dispatchNative(p)
		return
	}
	if c.afkIgnore(p) {
		return
	}
	if c.connectionCapabilitiesPacket(p) {
		return
	}
	if protocol.IsCollectionTimingCapability(p) {
		if !c.connection.offered && !c.connection.enabled {
			c.gonlineCollectionTiming = true
		}
		return
	}
	if !(len(p) > 1 && p[0] == protocol.CommandHandshake && p[1] == protocol.HandshakeBackdropLocation) && c.backdropPacket(p) {
		return
	}
	if c.combatCapabilitiesPacket(p) {
		return
	}
	if r := c.combat.round; r != nil && p[0] != protocol.CommandChat && p[0] != protocol.CommandDiscovery {
		if r.deferredBytes+len(p) > protocol.CombatMaxRoundBytes || len(r.deferred) >= protocol.CombatMaxRoundPackets {
			c.invalidCombat()
			return
		}
		r.deferred = append(r.deferred, append([]byte(nil), p...))
		r.deferredBytes += len(p)
		return
	}
	if p[0] == protocol.CommandGonlineCombat {
		c.combatPacket(p)
		return
	}
	if c.backdropPacket(p) {
		return
	}
	c.dispatchNative(p)
}

func (c *Client) combatCapabilitiesPacket(p []byte) bool {
	if len(p) < 2 || p[0] != protocol.CommandHandshake {
		return false
	}
	switch p[1] {
	case protocol.HandshakeCombatOffer:
		if c.connection.offered || c.connection.enabled {
			return true
		}
		flags, valid := protocol.ParseCombatCapabilities(p, protocol.HandshakeCombatOffer)
		if valid && flags&protocol.CombatCapabilityRoundChain != 0 && !c.combat.offered && !c.combat.enabled {
			if c.Net.Send(protocol.CombatCapabilities(protocol.HandshakeCombatAccept, protocol.CombatCapabilityRoundChain)) == nil {
				c.combat.offered = true
			}
		}
		return true
	case protocol.HandshakeCombatEnabled:
		if c.connection.offered || c.connection.enabled {
			return true
		}
		flags, valid := protocol.ParseCombatCapabilities(p, protocol.HandshakeCombatEnabled)
		if valid && c.combat.offered && flags == protocol.CombatCapabilityRoundChain {
			c.combat.enabled = true
		}
		return true
	}
	return false
}

func (c *Client) combatPacket(p []byte) {
	if !c.combat.enabled {
		return
	} // Legacy servers cannot activate the extension by a generic echo.
	if len(p) < 2 || !c.battle.state.Active {
		c.invalidCombat()
		return
	}
	switch p[1] {
	case protocol.CombatRoundPrompt:
		if len(p) != protocol.CombatPromptBytes {
			c.invalidCombat()
			return
		}
		token := binary.LittleEndian.Uint32(p[2:])
		if token <= c.combat.lastSequence {
			return
		}
		cell := clientbattle.Cell{X: p[6], Y: p[7]}
		if !cell.Valid() || !c.battle.state.Owned(c.battle.state.At(cell)) {
			c.invalidCombat()
			return
		}
		c.combat.lastSequence, c.combat.choiceToken = token, token
		c.combat.choices = nil
		c.dispatchNative([]byte{protocol.CommandBattleAction, protocol.BattleActionTurn, cell.X, cell.Y, 0})
		c.battle.state.Prompt = true // A living pet may be the owner's only remaining actor.
		c.dispatchNative([]byte{protocol.CommandBattleReady, protocol.BattleReadyReady})
	case protocol.CombatRoundChunk:
		if len(p) <= protocol.CombatChunkHeaderBytes {
			c.invalidCombat()
			return
		}
		if binary.LittleEndian.Uint32(p[2:]) <= c.combat.lastSequence {
			return
		}
		done, err := c.combat.assembly.Add(p)
		if err != nil {
			c.invalidCombat()
			return
		}
		if !done {
			return
		}
		steps, err := protocol.DecodeCombatRound(c.combat.assembly.Data)
		if err != nil {
			c.invalidCombat()
			return
		}
		// Validate every known native battle record before publishing the first
		// step. The copy also follows reinforcement replacements in order.
		staged := c.battle.state
		staged.Fighters = append([]clientbattle.Fighter(nil), staged.Fighters...)
		staged.Actions = nil
		for _, step := range steps {
			for _, packet := range step.Packets {
				if handled, valid := staged.Apply(packet); handled && !valid {
					c.invalidCombat()
					return
				}
			}
		}
		c.combat.lastSequence = c.combat.assembly.ID
		c.combat.assembly = protocol.CombatAssembly{}
		c.combat.round = &combatPlayback{steps: steps}
		c.combat.choices, c.combat.choiceToken = nil, 0
		c.battle.state.Ready, c.remote.battle.ready, c.battle.timerOn = false, false, false
	default:
		c.invalidCombat()
	}
}

func (c *Client) invalidCombat() {
	c.disconnectText = []byte(localization.UI("ui.app.invalid_combat_communication_8a4215fb"))
	c.Net.Close()
	c.disconnected()
}

// Called even for background sessions. A step waits for its minimum interval
// and actual animation/contact/removal completion, rather than network traffic.
func (c *Client) combatRoundTick(now time.Time) {
	for r := c.combat.round; r != nil; r = c.combat.round {
		v := &c.battle
		replacing := r.index < len(r.steps) && c.petReplacementStep(r.steps[r.index].Packets)
		if now.Before(r.until) || len(v.state.Actions) > 0 || len(v.playing) > 0 || len(v.departures) > 0 && !replacing || len(v.petDepartures) > 0 || len(v.arrivals) > 0 || len(v.transitionPackets) > 0 || len(v.overkills) > 0 {
			return
		}
		if r.index == len(r.steps) {
			c.combat.round = nil
			for _, packet := range r.deferred {
				c.dispatch(packet)
			}
			return
		}
		step := r.steps[r.index]
		r.index++
		r.until = now.Add(step.Delay)
		for _, packet := range step.Packets {
			c.dispatchNative(packet)
		}
	}
}

func (c *Client) sendCombatChoice(p []byte) error {
	if !c.combat.enabled {
		return c.Net.Send(p)
	}
	if c.combat.choiceToken == 0 || !protocol.ValidCombatChoice(p) {
		return protocol.ErrMalformed
	}
	cell := clientbattle.Cell{X: p[2], Y: p[3]}
	f := c.battle.state.At(cell)
	if !c.battle.state.Owned(f) || f.HP == 0 || f.Removed || f.Waiting {
		return protocol.ErrMalformed
	}
	for _, choice := range c.combat.choices {
		if choice[2] == cell.X && choice[3] == cell.Y {
			return protocol.ErrMalformed
		}
	}
	choices := append(append([][]byte(nil), c.combat.choices...), append([]byte(nil), p...))
	want := 0
	for _, actor := range c.battle.state.Fighters {
		if c.battle.state.Owned(&actor) && actor.HP > 0 && !actor.Removed && !actor.Waiting {
			want++
		}
	}
	if len(choices) < want {
		c.combat.choices = choices
		return nil
	}
	packet, err := protocol.EncodeCombatChoices(c.combat.choiceToken, choices)
	if err != nil {
		return err
	}
	if err = c.Net.Send(packet); err != nil {
		return err
	}
	c.combat.choices, c.combat.choiceToken = nil, 0
	return nil
}

// Preserve a partial selection using ordinary AC50 commands. The complete-batch
// contract stays unchanged; the server's timeout leaves missing actors idle.
func (c *Client) flushCombatChoices() {
	if !c.combat.enabled || c.combat.choiceToken == 0 {
		return
	}
	for _, choice := range c.combat.choices {
		if err := c.Net.Send(choice); err != nil {
			c.Chat.Notice(err.Error())
			return
		}
	}
	c.combat.choices, c.combat.choiceToken = nil, 0
}
