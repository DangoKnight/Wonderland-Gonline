package app

import (
	"image"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

// FUN_0039108c -> FUN_0037896c state 38 -> FUN_00378d80 starts an entry
// controller before ordinary actions. FUN_00392728 uses distance 80 behind the
// formation. Gonline presents the incoming pet walking concurrently with the
// outgoing pet; its native Waiting flag still governs commands and selection.
const battleArrivalDistancePixels = 240 // Cosmetic ground entry from near the screen edge.

type battlePetDeparture struct {
	fighter clientbattle.Fighter
	body    *battleBody
	at      time.Time
}

func (v *battleView) petPresented(f clientbattle.Fighter) bool {
	return f.Kind == clientbattle.Pet && f.Generation != 0 && v.presentedPets[f.Cell] == f.Generation
}

func (c *Client) petReplacementPacket(p []byte) bool {
	if len(p) != 2+clientbattle.FighterRecordBytes || p[0] != protocol.CommandBattleState || p[1] != protocol.BattleStateParticipant || p[3] != clientbattle.Pet {
		return false
	}
	_, leaving := c.battle.departures[clientbattle.Cell{X: p[14], Y: p[15]}]
	return leaving && len(c.battle.playing) == 0 && len(c.battle.state.Actions) == 0
}

func (c *Client) petReplacementStep(packets [][]byte) bool {
	replacing := false
	for _, p := range packets {
		if c.petReplacementPacket(p) {
			replacing = true
			continue
		}
		if len(p) < 2 || !(p[0] == protocol.CommandBattlePet || p[0] == protocol.CommandBattleStat || p[0] == protocol.CommandBattleEffect && p[1] == protocol.BattleEffectSubmitted) {
			return false
		}
	}
	return replacing
}

func (c *Client) retainDepartingPet(old clientbattle.Fighter, began time.Time) {
	v := &c.battle
	old = v.displayed(old)
	old.Removed, old.Waiting = false, false
	if v.petDepartures == nil {
		v.petDepartures = map[clientbattle.Cell]battlePetDeparture{}
	}
	v.petDepartures[old.Cell] = battlePetDeparture{fighter: old, body: c.battleBody(old), at: began}
}

func (c *Client) departingPetsTick(now time.Time) {
	for cell, pet := range c.battle.petDepartures {
		if pet.fighter.HP == 0 || !pet.at.IsZero() && now.Sub(pet.at) >= c.battleDepartureMotion().Duration() {
			delete(c.battle.petDepartures, cell)
		} else if pet.at.IsZero() {
			pet.at = now
			c.battle.petDepartures[cell] = pet
		}
	}
}

func (c *Client) drawDepartingPets(now time.Time, offset image.Point) {
	motion := c.battleDepartureMotion()
	for _, f := range c.battle.state.Fighters {
		pet, exists := c.battle.petDepartures[f.Cell]
		if !exists {
			continue
		}
		elapsed := time.Duration(0)
		if !pet.at.IsZero() {
			elapsed = now.Sub(pet.at)
		}
		at := motion.PositionAt(pet.fighter.Cell, clientbattle.Cell{}, elapsed).Add(offset)
		action, frame := motion.PoseAt(pet.fighter.Cell, clientbattle.Cell{}, elapsed, pet.body.moveKind)
		world.DrawShadow(c.Screen, c.Pics, pet.body.shadow, at.X, at.Y)
		c.drawBattleBody(pet.fighter, pet.body, at, action, frame, false)
		c.drawBattleFighterOverlay(pet.fighter, pet.body, at)
	}
}

func battleArrivalMotion() clientbattle.Motion {
	return clientbattle.Motion{Return: clientbattle.MotionTravelWalk, Frames: []clientbattle.MotionFrame{
		{Offset: image.Pt(-battleArrivalDistancePixels, 0)},
	}}
}

func (c *Client) battleArrivals(now time.Time) {
	v := &c.battle
	if len(v.playing) > 0 || len(v.state.Actions) > 0 {
		return
	}
	motion := battleArrivalMotion()
	duration := motion.Duration() - motion.ReturnStart()
	for cell, began := range v.arrivals {
		if began.IsZero() {
			v.arrivals[cell] = now
		} else if now.Sub(began) >= duration {
			delete(v.arrivals, cell)
		}
	}
}

// A replacement can reuse its cell while a detached copy of the old pet exits.
// Attacks, readiness and settlement wait until both visual tracks complete.
func (c *Client) deferBattleTransitionPacket(p []byte) bool {
	v := &c.battle
	if !v.state.Active || !v.petTransition || v.replayingTransition || len(p) == 0 {
		return false
	}
	if p[0] == protocol.CommandChat || p[0] == protocol.CommandDiscovery {
		return false
	}
	if p[0] == protocol.CommandBattlePet || c.petReplacementPacket(p) {
		return false
	}
	if (len(p) == 4 || len(p) == 5) && p[0] == protocol.CommandBattleState && p[1] == protocol.BattleStateExit {
		return false // One native departure batch can withdraw owner and pet together.
	}
	if len(p) > 1 && p[0] == protocol.CommandBattleState && p[1] == protocol.BattleStateFormation {
		return false // A new battle resets the previous presentation completely.
	}
	if len(v.departures) == 0 && len(v.petDepartures) == 0 && len(v.arrivals) == 0 && len(v.transitionPackets) == 0 {
		return false
	}
	if len(v.transitionPackets) >= protocol.CombatMaxRoundPackets || v.transitionBytes+len(p) > protocol.CombatMaxRoundBytes {
		c.invalidCombat()
		return true
	}
	v.transitionPackets = append(v.transitionPackets, append([]byte(nil), p...))
	v.transitionBytes += len(p)
	return true
}

func (c *Client) drainBattleTransitionPackets() {
	v := &c.battle
	if !v.petTransition || len(v.petDepartures) > 0 || len(v.playing) > 0 || len(v.state.Actions) > 0 {
		return
	}
	for len(v.transitionPackets) > 0 && len(v.petDepartures) == 0 && len(v.arrivals) == 0 && len(v.state.Actions) == 0 {
		p := v.transitionPackets[0]
		if len(v.departures) > 0 && !(len(p) > 0 && p[0] == protocol.CommandBattlePet || c.petReplacementPacket(p)) {
			break
		}
		v.transitionPackets = v.transitionPackets[1:]
		remaining := v.transitionPackets
		v.transitionBytes -= len(p)
		v.replayingTransition = true
		c.dispatchNative(p)
		v.replayingTransition = false
		if !v.state.Active {
			// Closing the scene clears its transient fields, but settlement
			// packets that followed the close still belong to this connection.
			if c.World != nil {
				for _, packet := range remaining {
					c.dispatch(packet)
				}
			}
			return
		}
	}
	if len(v.transitionPackets) == 0 && len(v.petDepartures) == 0 && len(v.departures) == 0 && len(v.arrivals) == 0 {
		v.petTransition = false
		v.transitionPackets = nil
	}
}
