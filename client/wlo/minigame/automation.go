package minigame

import (
	"encoding/json"
	"fmt"
	"image"
	"math"
	"time"
	"wonderland-gonline/client/wlo/seui"
)

// Automation exposes game-domain actions and only the information available in
// the current presentation. No OS/browser input or screen coordinates are used.
type Automation interface {
	AutomationState() seui.AutomationInfo
	AutomationAction(string, json.RawMessage, time.Time) error
}

func badAction() error { return fmt.Errorf("minigame action unavailable") }

func (g *Sheep) AutomationState() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "dream", Actions: []string{"strike"}, Value: map[string]any{"score": g.Score, "lives": g.Lives, "cue": g.cue, "answered": g.answered, "remaining": g.remaining}}
}
func (g *Sheep) AutomationAction(action string, _ json.RawMessage, now time.Time) error {
	if action != "strike" || g.answered || !g.ready(now) || g.hammerFrame(now) != sheepStandingHammer {
		return badAction()
	}
	g.Strike(now)
	return nil
}

func (m *Memory) AutomationState() seui.AutomationInfo {
	cards := []map[string]any{}
	for i, c := range m.cards {
		row := map[string]any{"id": i, "taken": c.Taken}
		if m.phase != memoryPlaying || c.Taken {
			row["number"] = c.Value
		}
		cards = append(cards, row)
	}
	return seui.AutomationInfo{Kind: "memory", Actions: []string{"choose"}, Value: map[string]any{"score": m.Score, "lives": m.Lives, "phase": m.phase, "cards": cards, "deadline": m.deadline}}
}
func (m *Memory) AutomationAction(action string, raw json.RawMessage, now time.Time) error {
	var index int
	if action != "choose" || seui.DecodeAutomation(raw, &index) != nil || index < 0 || index >= len(m.cards) || m.cards[index].Taken || m.phase != memoryPlaying || !m.ready(now) {
		return badAction()
	}
	m.ChooseCard(index, now)
	return nil
}

func (g *Arcade) AutomationState() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "arcade", Actions: []string{"requestPlay", "confirmPlay"}, Value: map[string]any{"kind": g.Kind, "pending": g.pending, "confirming": g.confirming, "animating": g.animating, "status": g.status, "symbols": g.symbols, "prize": g.prize, "quantity": g.quantity}}
}
func (g *Arcade) AutomationAction(action string, _ json.RawMessage, now time.Time) error {
	if !g.Started || g.Done() || g.pending || g.animating {
		return badAction()
	}
	switch action {
	case "requestPlay":
		g.Play(now, false)
	case "confirmPlay":
		if !g.confirming {
			return badAction()
		}
		g.Play(now, true)
	default:
		return badAction()
	}
	return nil
}

func (g *Mario) AutomationState() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "fruitMachine", Actions: []string{"play", "leave"}, Value: map[string]any{"kind": g.Kind, "armed": g.Armed(), "spinning": g.Spinning(), "pending": g.pending, "points": g.Points, "message": g.message, "symbols": g.shown}}
}
func (g *Mario) AutomationAction(action string, _ json.RawMessage, _ time.Time) error {
	if !g.Started || g.Done() {
		return badAction()
	}
	switch action {
	case "play":
		if g.pending || g.spinning {
			return badAction()
		}
		g.Press()
	case "leave":
		g.Exit()
	default:
		return badAction()
	}
	return nil
}

func (g *Lucky) AutomationState() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "catch", Actions: []string{"move"}, Value: map[string]any{"score": g.Score, "lives": g.Lives, "position": g.x, "target": g.aim, "drops": g.drops, "remaining": g.seconds, "minimum": luckyLeft, "maximum": luckyRight}}
}
func (g *Lucky) AutomationAction(action string, raw json.RawMessage, now time.Time) error {
	var position int
	if action != "move" || seui.DecodeAutomation(raw, &position) != nil || position < luckyLeft || position > luckyRight || !g.ready(now) {
		return badAction()
	}
	g.Aim(position, 0)
	return nil
}

func (m *Mole) AutomationState() seui.AutomationInfo {
	targets := []map[string]any{}
	for i := 1; i <= moleHoles; i++ {
		h := m.holes[i]
		if h.state >= holeRising && h.state <= holeUp && h.frame > 0 && h.frame <= moleFrames {
			targets = append(targets, map[string]any{"id": i, "kind": h.kind, "frame": h.frame})
		}
	}
	return seui.AutomationInfo{Kind: "mole", Actions: []string{"strike"}, Value: map[string]any{"state": m.state, "score": m.score, "remaining": m.seconds, "targets": targets, "hammer": m.hammer}}
}
func (m *Mole) AutomationAction(action string, raw json.RawMessage, now time.Time) error {
	var index int
	if action != "strike" || seui.DecodeAutomation(raw, &index) != nil || index < 1 || index > moleHoles || m.state != moleStateRunning || m.hammer != hammerReady {
		return badAction()
	}
	h := &m.holes[index]
	if h.state < holeRising || h.state > holeUp || h.frame == 0 || h.frame > moleFrames {
		return badAction()
	}
	m.hamAt, m.hammer = now, hammerDown
	if h.kind == moleKindBomb {
		h.state, h.frame, h.at = holeEmpty, 0, now
		m.effect.Play(MoleExplosion, 100*time.Millisecond, LevelKeyed, h.x+0x3c, h.y+0x32, 4, 4, 1, now)
		m.score = max(m.score-moleBombCost, 0)
		m.sound(soundBomb)
		return nil
	}
	h.state = holeHit
	h.hitFrame = moleFrames + 1 - h.frame
	h.frame = 0
	m.effect.Play(MoleSparkle, 70*time.Millisecond, sparkleLevel, h.x+0x3c, h.y+0x3c, 9, 9, 4, now)
	m.score++
	m.sound(soundHit)
	return nil
}

func (h *Hunter) AutomationState() seui.AutomationInfo {
	targets := []map[string]any{}
	for i, m := range h.monsters {
		if m != nil && m.hp > 0 {
			targets = append(targets, map[string]any{"id": i, "monster": m.id, "hp": m.hp, "x": m.x, "y": m.y})
		}
	}
	return seui.AutomationInfo{Kind: "hunter", Actions: []string{"shoot"}, Value: map[string]any{"targets": targets, "lives": h.lives, "score": h.score, "remaining": h.seconds, "kills": h.kills}}
}
func (h *Hunter) AutomationAction(action string, raw json.RawMessage, _ time.Time) error {
	var index int
	if action != "shoot" || seui.DecodeAutomation(raw, &index) != nil || index < 0 || index >= len(h.monsters) || h.monsters[index] == nil || h.monsters[index].hp <= 0 || h.ended || !h.Started || h.done || h.ammo < 1 || h.shot != 0 {
		return badAction()
	}
	m := h.monsters[index]
	h.sound(soundShot)
	h.sightX, h.sightY = m.x, m.y
	h.shotX, h.shotY = m.x, m.y
	h.shot = 1
	m.hp -= hunterShotDamage
	return nil
}

func (p *Pumpkin) AutomationState() seui.AutomationInfo {
	targets := []map[string]any{}
	for i, g := range p.ghosts {
		if g.active {
			targets = append(targets, map[string]any{"id": i, "x": g.x, "y": g.y, "hit": g.hit})
		}
	}
	return seui.AutomationInfo{Kind: "pumpkin", Actions: []string{"shoot"}, Value: map[string]any{"targets": targets, "score": p.Score, "lives": p.Lives, "busy": p.shot.active, "deadline": p.deadline}}
}
func (p *Pumpkin) AutomationAction(action string, raw json.RawMessage, now time.Time) error {
	var index int
	if action != "shoot" || seui.DecodeAutomation(raw, &index) != nil || index < 0 || index >= len(p.ghosts) || !p.ghosts[index].active || p.ghosts[index].hit || !p.ready(now) || now.Before(p.startedAt.Add(arcadeCountdown)) || p.shot.active {
		return badAction()
	}
	g := p.ghosts[index]
	dx, dy := float64(g.x-pumpkinOriginX), float64(g.y-pumpkinOriginY)
	length := math.Hypot(dx, dy)
	if length == 0 {
		return badAction()
	}
	p.shot = pumpkinShot{x: pumpkinOriginX, y: pumpkinOriginY, dx: dx / length * pumpkinProjectileStep, dy: dy / length * pumpkinProjectileStep, active: true}
	return nil
}

func (s *Scotd) AutomationState() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "fighter", Actions: []string{"left", "right", "duck", "jump", "attack"}, Value: map[string]any{"x": s.X, "y": s.Y, "lives": s.Lives, "kills": s.Kills, "direction": s.direction}}
}
func (s *Scotd) AutomationAction(action string, raw json.RawMessage, now time.Time) error {
	var down bool
	if seui.DecodeAutomation(raw, &down) != nil || !s.ready(now) {
		return badAction()
	}
	key := map[string]int{"left": KeyLeft, "right": KeyRight, "duck": KeyDown, "jump": KeyUp, "attack": KeySpace}[action]
	if key == 0 {
		return badAction()
	}
	s.Key(key, down, now)
	return nil
}

func (g *Throw) AutomationState() seui.AutomationInfo {
	actors := []map[string]any{}
	for i, p := range g.npc {
		actors = append(actors, map[string]any{"id": i, "x": p.x, "y": p.y, "lives": p.lives, "state": p.state})
	}
	var player any
	if p := g.me(); p != nil {
		player = map[string]any{"x": p.x, "y": p.y, "state": p.state, "lives": p.lives}
	}
	return seui.AutomationInfo{Kind: "dodgeball", Actions: []string{"move", "charge", "throw"}, Value: map[string]any{"targets": actors, "player": player, "power": g.power, "running": g.running}}
}
func (g *Throw) AutomationAction(action string, raw json.RawMessage, now time.Time) error {
	p := g.me()
	if !g.running || p == nil {
		return badAction()
	}
	switch action {
	case "move":
		var point struct{ X, Y int }
		if seui.DecodeAutomation(raw, &point) != nil || !image.Pt(point.X, point.Y).In(throwOwnCourt) || p.state != throwIdle {
			return badAction()
		}
		p.state = throwWantRun
		g.down, g.up, g.target = true, false, image.Pt(point.X, point.Y)
	case "charge":
		if p.state != throwIdle {
			return badAction()
		}
		p.state, g.powerAt, g.power = throwCharging, now, 0
		g.down, g.up = true, false
	case "throw":
		// The native game aims by court position and charge power, not by a
		// selected opponent. Releasing must not pretend to retarget the ball.
		if p.state != throwCharging {
			return badAction()
		}
		g.down, g.up = false, true
	default:
		return badAction()
	}
	return nil
}
