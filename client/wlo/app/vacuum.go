package app

import (
	"time"
	"wonderland-gonline/internal/protocol"
)

const (
	vacuumCleanerItemID       uint16 = 34077
	bigVacuumCleanerItemID    uint16 = 34129
	oneDayVacuumCleanerItemID uint16 = 34134
	vacuumEquipmentIndex             = 5
	vacuumMaxActorID                 = 255               // AC23:73 carries one byte, unlike AC20:1.
	vacuumLegacyInterval             = 190 * time.Second // FUN_00456824: 190000 ms, independent of AC66 timers.
	vacuumRespawnGrace               = 10 * time.Second
	// FUN_00455154's exact native template list, independent of NPC kind.
	vacuumWoodTemplate        uint32 = 19052
	vacuumCrystalOreTemplate  uint32 = 19058
	vacuumCrudeOilTemplate    uint32 = 19059
	vacuumSulfurOreTemplate   uint32 = 19060
	vacuumPureOreTemplate     uint32 = 19061
	vacuumWaterSourceTemplate uint32 = 19069
	vacuumLeafTemplate        uint32 = 19070
	vacuumAlloyOreTemplate    uint32 = 19071
)

func vacuumCollectionTemplate(id uint32) bool {
	switch id {
	case vacuumWoodTemplate, vacuumCrystalOreTemplate, vacuumCrudeOilTemplate,
		vacuumSulfurOreTemplate, vacuumPureOreTemplate, vacuumWaterSourceTemplate,
		vacuumLeafTemplate, vacuumAlloyOreTemplate:
		return true
	}
	return false
}

type vacuumRuntime struct {
	actor                    uint16
	mapID                    uint16
	x, y                     int
	started, waiting, reward bool
	timer                    uint16
	cooldown                 time.Duration
	next                     time.Time
}

func (c *Client) vacuumEquipped() bool {
	if c.InventoryState == nil {
		return false
	}
	item := c.InventoryState.Equipment[vacuumEquipmentIndex]
	if item.Empty() {
		return false
	}
	return item.ID == vacuumCleanerItemID || item.ID == bigVacuumCleanerItemID || item.ID == oneDayVacuumCleanerItemID
}

func (c *Client) prepareVacuum(actor uint16) {
	if actor == 0 || actor > vacuumMaxActorID || !c.vacuumEquipped() {
		return
	}
	n := c.World.NPCs[actor]
	if n == nil || !vacuumCollectionTemplate(n.Template) {
		return
	}
	p := c.World.Player
	c.vacuum = vacuumRuntime{actor: actor, mapID: p.Map, x: p.X, y: p.Y, waiting: true}
}

func (c *Client) beginVacuum() {
	r := &c.vacuum
	if r.actor == 0 || r.started {
		return
	}
	r.started = true
	// Until the first successful receipt supplies this source's actual timer,
	// use the native cadence, including when starting on an exhausted spot.
	r.next = c.Now().Add(vacuumLegacyInterval)
	r.cooldown = vacuumLegacyInterval - vacuumRespawnGrace
	if c.Net.Send([]byte{protocol.CommandInventory, protocol.InventoryVacuumCollection, byte(r.actor)}) != nil {
		c.vacuum = vacuumRuntime{}
	}
}

// Update timing only after this actor's item receipt and a single validated timer.
// Other actors, malformed batches and unrelated timers cannot reschedule a run.
func (c *Client) vacuumTimer(updates map[uint16]time.Time) {
	r := &c.vacuum
	if r.actor == 0 || !r.waiting || !r.reward || c.acquisitionActor != r.actor || len(updates) != 1 {
		return
	}
	now := c.Now()
	for id, expiry := range updates {
		if !expiry.After(now) || r.timer != 0 && r.timer != id {
			return
		}
		r.timer, r.cooldown = id, expiry.Sub(now)
		if c.gonlineCollectionTiming {
			r.next = expiry.Add(vacuumRespawnGrace)
		}
	}
	r.waiting = false
}

func (c *Client) stopVacuum() {
	started := c.vacuum.started
	c.vacuum = vacuumRuntime{}
	if started && c.Net != nil {
		c.Net.Send([]byte{protocol.CommandInventory, protocol.InventoryVacuumCollection, 0})
	}
}

func (c *Client) vacuumTick() {
	r := &c.vacuum
	if r.actor == 0 {
		return
	}
	if c.World == nil || !c.vacuumEquipped() || c.battle.state.Active {
		c.stopVacuum()
		return
	}
	p := c.World.Player
	if p.Map != r.mapID || p.X != r.x || p.Y != r.y || c.World.Walking() {
		c.stopVacuum()
		return
	}
	if !r.started {
		if !c.event.active {
			c.vacuum = vacuumRuntime{}
		}
		return
	}
	now := c.Now()
	if now.Before(r.next) || !c.mapReady || c.held || c.event.active || c.movie != nil || c.sport != nil || c.settingsPromptForm != nil || c.sceneFrozen() {
		return
	}
	n := c.World.NPCs[r.actor]
	if n == nil || !n.Shown || abs(p.X-n.X) > npcReach || abs(p.Y-n.Y) > npcReach {
		c.stopVacuum()
		return
	}
	r.waiting, r.reward = true, false
	r.next = now.Add(vacuumLegacyInterval)
	if c.gonlineCollectionTiming {
		r.next = now.Add(r.cooldown + vacuumRespawnGrace)
	}
	c.sendNPCClick(r.actor)
}
