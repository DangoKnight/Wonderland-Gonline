package app

import (
	"encoding/binary"
	"fmt"
	"image"
	"sort"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/minigame"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	battleIdleLeft          = 16 // FUN_00398700 / FUN_00430474: native combat stance.
	battleIdleRight         = 17
	battleDefeatedLeft      = 0x1a // FUN_0038ab70: the falling group, right half.
	battleDefeatedRight     = 0x1b // left half
	battleTextInk           = 0xffff
	battleBarWidth          = 48
	battleFloatDuration     = 1200 * time.Millisecond
	battleFallbackDuration  = 800 * time.Millisecond // Compatibility for absent MBTM records only.
	battleCommandWidth      = 160
	battleCommandHeight     = 160
	battleCommandOriginX    = 80
	battleCommandOriginY    = 115
	battleCommandButtonSize = 28
	battleSceneCameraX      = 432 // FUN_0036f448 -> FUN_0036ff4c.
	battleSceneCameraY      = 232
	battleCatchSkill        = 10008
	battleFleeSkill         = 60041
	battleDamageDigits      = "Num_YellowRed10_2" // FUN_0039e884.
	battleSPDigits          = "Num_Blue10_1"
	battleDigitStyle        = 4 // FUN_003b9c00: 20-pixel advance, centered.
	battleCommandAttack     = 0
	battleCommandSkills     = 1
	battleCommandDefend     = 2
	battleCommandCapture    = 3
	battleCommandFlee       = 4
	battleCommandPrevious   = 5
	battlePreviousX         = 65 // Native radial constructor 0x002974a0, button 8.
	battlePreviousY         = 83
	battleIconDefend        = 1
	battleIconCapture       = 2
	battleIconFlee          = 3
	battleIconAttack        = 6
	battleIconSkills        = 7
	battleIconItems         = 4 // Native radial button 4 at (90, 126): FUN_00351700, the item form.
	battleIconAuto          = 5 // Native radial button 5 at (42, 126): FUN_0039c238, automatic actions.
	battleCommandItems      = 6
	battleCommandAuto       = 7
	// Fighter overlay: FUN_00437f28 draws icon_rail2 then icon_HP2 cut to
	// HP*40/max at (x-18, feet-height), never above y 80 in battle; the
	// name (0x4147f8) is centred above it, cyan for monsters and yellow
	// for players and pets (0x4154d2).
	battleBarFrame       = "icon_rail2"
	battleBarFill        = "icon_HP2"
	battleBarFull        = 40
	battleBarLeft        = 18
	battleBarTopMin      = 0x50
	battleNameBase       = 20 // 0x415168/0x4154a7: two 10-pixel lifts above the bar.
	battleNameLiftLow    = 0x19
	battleNameLiftTall   = 10
	battleNameTallLimit  = 0x32
	battleNameHeight     = 0xc
	battleNameMonsterInk = 0x07ff
	battleNamePlayerInk  = 0xffe0
	battleNameStyle      = 2
	battleNameCharWidth  = 8
	// Compatibility: player names in Battle/Screenshot_20261008_091702.png
	// sit 20 pixels above the bar rather than the 30 the monster rule gives;
	// the player's extra overlay branches (0x415a6f..0x415c34) are not traced.
	battlePlayerNameLift = 0
	// FUN_0038ad1c: the hovered fighter's details, left of fighters in
	// columns 1 and 3 and right of those in columns 2 and 4.
	battleInfoRight = 0x1e
	battleInfoLeft  = 0x19
	battleInfoName  = 0x32
	battleInfoElem  = 0x23
	battleInfoLevel = 0x14
	battleInfoHP    = 10
	battleInfoInk   = 0xffff
	// FUN_003977a8 / FUN_0039800c / FUN_00395efc: the turn countdown.
	battleTimerDigits   = "Num_Blue10_2"
	battleTimerX        = 400 // DAT_00824e60/62: half the 800 x 600 screen.
	battleTimerY        = 300
	battleTimerLowerY   = 0x32 // kinds 2 and 7
	battleTimerWarnFrom = 10
	// The auto cancel buttons (+0x51c, constructor 0x297aea): Btn_Battle_5_1,
	// 28 x 28, 10 pixels from the right edge, at y 470 + 28*(n-1); button 3
	// (y 526) cancels both the player's and the pet's automatic actions.
	battleAutoCancelX = ScreenWidth - battleCommandButtonSize - 10
	battleAutoCancelY = 0x1d6 + 2*battleCommandButtonSize
	battleTimerSound  = `sound\wav0145.wav`
)

// battleElements are FUN_0038ad1c's element names (+0x1f82).
var battleElements = [...]string{"None", "Eart", "Watr", "Fire", "Wind", "Hart"}

type battlePreviousAction struct {
	id, owner uint32
	kind      byte
	skill     uint16
}

type battleBody struct {
	human *role.Human
	npc   *role.NPC
	name  []byte
	id    uint32
	owner uint32
	kind  byte
	// height is +0x20b2 (FUN_004265a4 for templates, 0x50 for players).
	height int
	// moveKind is the template's run style (Npc.dat +0x57); 0 for players.
	moveKind byte
	// shadow is the ground shadow kind (FUN_0030120c): the small cut for
	// players, the template's kind for others.
	shadow byte
}
type battlePlay struct {
	action clientbattle.Action
	at     time.Time
	hits   uint64
	sounds uint64
}
type battleFloat struct {
	cell   clientbattle.Cell
	text   []byte
	ink    uint16
	at     time.Time
	font   string
	amount uint32
}
type battleView struct {
	state          clientbattle.State
	stats          world.Stats
	assets         *clientbattle.Assets
	loadAttempted  bool
	bodies         map[clientbattle.Cell]*battleBody
	playing        []battlePlay
	floats         []battleFloat
	menu           *seui.Form
	selected       clientbattle.Cell
	binding        *hud.Binding
	previous       *seui.FixedButton
	lastActions    map[clientbattle.Cell]battlePreviousAction
	pendingActions map[clientbattle.Cell]battlePreviousAction
	// The turn countdown (+0xce3): started when a round opens for one of
	// our fighters, ticking each second, closing the choice at -1.
	timerOn    bool
	timerTurn  uint64
	timerStart time.Time
	timerLimit int
	timerShown int
	timedOut   bool
	auto       bool // The radial "auto" command (FUN_0039c238).
	autoTurn   uint64
	autoForm   *seui.Form // The cancel buttons (+0x51c), FUN_00298f7c.
	music      bool       // The battle scene's music has started.
	reactions  []battlePlay
	missing    map[string]bool // effect pictures absent from the exports
}

// Native receive handlers: FUN_003920a0 (action batches), FUN_00392d3c
// (absolute stats), FUN_003970e4 (turn actor).
func (c *Client) remoteBattlePacket(p []byte) bool {
	if c.World == nil || len(p) < 2 {
		return false
	}
	if p[0] == protocol.CommandBattlePet {
		switch p[1] {
		case protocol.BattlePetSelect:
			if len(p) != 6 {
				return false
			}
			c.remote.activePet = binary.LittleEndian.Uint32(p[2:])
			return true
		case protocol.BattlePetRest:
			if len(p) != 2 {
				return false
			}
			c.remote.activePet = 0
			return true
		}
		return false
	}
	view := &c.battle
	if view.state.Self != c.World.Player.ID {
		view.state = clientbattle.State{Self: c.World.Player.ID}
	}
	wasActive, turn := view.state.Active, view.state.Turn
	oldDeaths := map[clientbattle.Cell]bool{}
	for _, f := range c.remote.battle.fighters {
		oldDeaths[clientbattle.Cell{X: f.x, Y: f.y}] = f.countedDeath
	}
	queued := len(view.state.Actions)
	handled, valid := view.state.Apply(p)
	if !handled {
		return false
	}
	if !valid {
		return false
	}
	if view.state.Turn != turn {
		c.hotbar.submitted = nil
		c.hotbar.turn++
		c.closeHotbarTarget()
		view.binding = nil
		view.pendingActions = nil
	}
	if view.state.Active && !wasActive {
		c.World.StopWalk()
		c.groundHeld = false
		c.pendingNPC = nil
		c.joinTeamTarget = false
		view.loadAttempted = false
		view.bodies = nil
		view.playing = nil
		view.floats = nil
		view.selected = clientbattle.Cell{}
		view.lastActions = nil
	}
	if p[0] == protocol.CommandBattleEffect && p[1] == protocol.BattleEffectSubmitted {
		cell := clientbattle.Cell{X: p[2], Y: p[3]}
		if action, ok := view.pendingActions[cell]; ok {
			if f := view.state.At(cell); f != nil && f.ID == action.id && f.Owner == action.owner && f.Kind == action.kind {
				if view.lastActions == nil {
					view.lastActions = map[clientbattle.Cell]battlePreviousAction{}
				}
				view.lastActions[cell] = action
			}
			delete(view.pendingActions, cell)
		}
	}
	// Executed animations also cover Remote actions and authoritative skill
	// replacement. Keep the client stunt alias when it resolves to this skill.
	if p[0] == protocol.CommandBattleAction && p[1] == protocol.BattleActionAnimation {
		for _, action := range view.state.Actions[queued:] {
			f := view.state.At(action.Actor)
			if f == nil || !view.state.Owned(f) {
				continue
			}
			id := action.Skill
			old := view.lastActions[action.Actor]
			if old.id == f.ID && old.owner == f.Owner && old.kind == f.Kind && (old.skill == id || f.Kind == clientbattle.Player && c.bindingSkillID(hud.Binding{ID: old.skill}) == id) {
				id = old.skill
			}
			if view.lastActions == nil {
				view.lastActions = map[clientbattle.Cell]battlePreviousAction{}
			}
			view.lastActions[action.Actor] = battlePreviousAction{f.ID, f.Owner, f.Kind, id}
		}
	}
	if !view.state.Active && wasActive {
		c.closeBattleView()
		c.closeHotbarTarget()
		c.remote.nextWalk = c.Now().Add(remoteWalkPause)
	}
	b := remoteBattle{active: view.state.Active, ready: view.state.Ready, prompt: view.state.Prompt}
	for _, f := range view.state.Fighters {
		counted := oldDeaths[f.Cell]
		if p[0] == protocol.CommandBattleEffect && p[1] == protocol.BattleEffectDefeated && len(p) == 4 && f.X == p[2] && f.Y == p[3] && !counted {
			counted = true
			if f.Kind == clientbattle.Player && f.ID == c.World.Player.ID {
				c.remote.playerDeaths++
			}
			if f.Kind == clientbattle.Pet && f.Owner == c.World.Player.ID {
				c.remote.petDeaths++
			}
		}
		if f.HP > 0 {
			counted = false
		}
		b.fighters = append(b.fighters, remoteFighter{side: f.Side, kind: f.Kind, x: f.X, y: f.Y, id: f.ID, owner: f.Owner, hp: f.HP, maxHP: f.MaxHP, sp: f.SP, maxSP: f.MaxSP, countedDeath: counted})
	}
	c.remote.battle = b
	return true
}
func (c *Client) closeBattleView() {
	if c.MainStatus != nil {
		c.MainStatus.Stats = c.Stats
	}
	v := &c.battle
	if v.menu != nil {
		v.menu.Hide()
		c.UI.Remove(v.menu)
		v.menu = nil
	}
	if v.autoForm != nil {
		v.autoForm.Hide()
		c.UI.Remove(v.autoForm)
		v.autoForm = nil
	}
	v.bodies = nil
	v.playing = nil
	v.floats = nil
	v.binding = nil
	v.previous = nil
	v.lastActions = nil
	v.pendingActions = nil
	v.timerOn = false
	v.timedOut = false
	v.auto = false
	if v.music {
		v.music = false
		c.playMapMusic()
	}
}
func (c *Client) battleBody(f clientbattle.Fighter) *battleBody {
	v := &c.battle
	if v.bodies == nil {
		v.bodies = map[clientbattle.Cell]*battleBody{}
	}
	if b := v.bodies[f.Cell]; b != nil && b.id == f.ID && b.kind == f.Kind && b.owner == f.Owner {
		return b
	}
	b := &battleBody{id: f.ID, kind: f.Kind, owner: f.Owner}
	if f.Kind == clientbattle.Player {
		pl := c.World.Player
		if f.ID != pl.ID {
			if peer := c.World.Peers[f.ID]; peer != nil {
				pl = peer.Player
			} else if peer := c.teamAppearances[f.ID]; peer != nil {
				pl = peer.Player
			} else {
				v.bodies[f.Cell] = b
				return b
			}
		}
		b.human = role.NewHuman(c.lib, c.items)
		b.human.Now = func() time.Time { return c.Now() }
		ch := login.CharacterSlot{Body: pl.Body, Head: pl.Head, Color1: pl.Color1, Color2: pl.Color2}
		for _, id := range pl.Items {
			if it, ok := c.items[id]; ok && it.Definition.EquipSlot >= 1 && it.Definition.EquipSlot <= 6 {
				ch.Equipment[it.Definition.EquipSlot] = id
			}
		}
		b.human.SetCharacter(&ch)
		b.name = pl.Name
		b.height = world.DefaultBattleHeight
	} else {
		template, ok := c.npcTemplates[f.ID]
		if f.Kind == clientbattle.Pet && f.Owner == c.World.Player.ID {
			for _, p := range c.InventoryState.Pets {
				if game.BroadcastID(uint32(p.ID)) == f.ID {
					if !ok {
						template, ok = c.npcTemplates[uint32(p.ID)]
					}
					b.name = p.Name
					break
				}
			}
		}
		if f.Kind == clientbattle.Pet && len(b.name) == 0 {
			if pet := c.World.Companions[f.Owner]; pet != nil && pet.ID == f.ID {
				b.name = pet.Name
				if !ok && pet.Info.Look != 0 {
					template, ok = pet.Info, true
				}
			}
		}
		if ok {
			b.npc = role.NewNPC(c.lib, template.Look, template.Colors)
			b.npc.Now = func() time.Time { return c.Now() }
			b.height = world.DefaultBattleHeight
			b.moveKind = template.MoveKind
			b.shadow = template.Shadow
			if anchor, ok := b.npc.FirstAnchorY(); ok {
				b.height = world.BattleHeight(template, anchor)
			}
			if len(b.name) == 0 || f.Kind != clientbattle.Pet {
				b.name = clientassets.Big5Text(template.Name)
			}
		}
	}
	if len(b.name) == 0 {
		b.name = []byte(fmt.Sprint(f.ID))
	}
	v.bodies[f.Cell] = b
	return b
}
func (c *Client) battleAssets() {
	v := &c.battle
	if v.loadAttempted {
		return
	}
	v.loadAttempted = true
	if c.resources.battleAssets == nil {
		a, err := clientbattle.Load(c.Assets)
		if err != nil {
			c.Chat.Notice("Could not load battle assets: " + err.Error())
			return
		}
		c.resources.battleAssets = a
	}
	v.assets = c.resources.battleAssets
	bg := v.assets.Background(v.state.Background)
	for _, id := range bg.Pictures {
		if id != 0 {
			name := fmt.Sprint(id)
			if c.Pics.Find(name) < 0 {
				for _, archive := range []string{"images", "map"} {
					if compiled, err := c.Assets.CompiledPicture(archive, name); err == nil {
						if c.Pics.AddCompiled(name, compiled) == nil {
							break
						}
					}
					if m, err := c.Assets.LoadPicture(archive, name); err == nil {
						c.Pics.Add(name, m)
						break
					}
				}
			}
		}
	}
}

// battleFrame advances even for background workspace sessions. Drawing uses only
// surface operations, keeping fighter state and sprite clocks session-owned.
func (c *Client) battleFrame(draw bool) bool {
	v := &c.battle
	if !v.state.Active {
		return false
	}
	c.battleAssets()
	if !v.music {
		v.music = true
		c.playBattleMusic()
	}
	v.stats = *c.Stats
	for _, f := range v.state.Fighters {
		if f.Kind == clientbattle.Player && f.ID == v.state.Self {
			v.stats.HP, v.stats.MaxHP = f.HP, f.MaxHP
			v.stats.SP, v.stats.MaxSP = f.SP, f.MaxSP
			v.stats.Level, v.stats.Element = f.Level, f.Element
			c.MainStatus.Stats = &v.stats
			break
		}
	}
	now := c.Now()
	for _, a := range v.state.Actions {
		v.playing = append(v.playing, battlePlay{action: a, at: now})
	}
	v.state.Actions = nil
	active := v.playing[:0]
	for _, play := range v.playing {
		m := clientbattle.Motion{}
		if v.assets != nil {
			m = v.assets.Motions[play.action.Skill]
		}
		for i, sound := range m.Sounds {
			mask := uint64(1) << i
			if play.sounds&mask == 0 && now.Sub(play.at) >= m.EventAt(clientbattle.MotionEvent{Trigger: sound.Trigger}) {
				play.sounds |= mask
				if sound.ID%10000 != 0 && c.Env.Sound != nil {
					c.Env.Sound(fmt.Sprintf(`sound\SEB%04d.wav`, sound.ID%10000))
				}
			}
		}
		duration := m.Duration()
		if duration == 0 {
			duration = battleFallbackDuration
		}
		elapsed := now.Sub(play.at)
		cues := len(m.Events)
		if m.DamageMode == 0 || cues == 0 {
			cues = 1
		}
		for i := 0; i < cues; i++ {
			hitAt := duration / 2
			if len(m.Events) > 0 {
				hitAt = m.EventAt(m.Events[min(i, len(m.Events)-1)])
			}
			mask := uint64(1) << i
			if play.hits&mask == 0 && (elapsed >= hitAt || elapsed >= duration) {
				play.hits |= mask
				c.battleHit(play.action, now, play.at.Add(min(hitAt, duration)), m, i)
			}
		}
		if elapsed < duration {
			active = append(active, play)
		}
	}
	v.playing = append(active, v.reactions...)
	v.reactions = v.reactions[:0]
	// Background workspace sessions must expire presentation state too.
	floats := v.floats[:0]
	for _, f := range v.floats {
		if now.Sub(f.at) < battleFloatDuration {
			floats = append(floats, f)
		}
	}
	v.floats = floats
	c.battleTimerTick(now)
	c.battleAutoTick()
	c.battleMenu()
	if !draw {
		return true
	}
	c.Screen.Fill(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0)
	if v.assets != nil {
		bg := v.assets.Background(v.state.Background)
		// FUN_0036ff5c draws sky, distant scenery, then the foreground.
		for layer := len(bg.Pictures) - 1; layer >= 0; layer-- {
			if id := bg.Pictures[layer]; id != 0 {
				i := c.Pics.Find(fmt.Sprint(id))
				if i >= 0 {
					x, y := -battleSceneCameraX, -battleSceneCameraY
					if layer < 2 {
						y = int(bg.Heights[layer]) - battleSceneCameraY
					}
					c.Pics.Draw(c.Screen, i, x, y, layer != 2)
				}
			}
		}
	}
	fighters := append([]clientbattle.Fighter(nil), v.state.Fighters...)
	positions := make(map[clientbattle.Cell]image.Point, len(fighters))
	for _, f := range fighters {
		positions[f.Cell] = f.Position()
	}
	for _, play := range v.playing {
		if v.assets != nil {
			if motion, ok := v.assets.Motions[play.action.Skill]; ok {
				positions[play.action.Actor] = motion.PositionAt(play.action.Actor, battleTarget(play.action), now.Sub(play.at))
			}
		}
	}
	sort.SliceStable(fighters, func(i, j int) bool { return positions[fighters[i].Cell].Y < positions[fighters[j].Cell].Y })
	for _, f := range fighters {
		if f.Removed {
			continue
		}
		b := c.battleBody(f)
		at := positions[f.Cell]
		action := battleIdleLeft
		if f.X <= 2 {
			action = battleIdleRight
		}
		frame := -1
		if f.HP == 0 {
			action = battleDefeatedLeft
			if f.X <= 2 {
				action = battleDefeatedRight
			}
		}
		lift := 0
		for _, p := range v.playing {
			if p.action.Actor == f.Cell && v.assets != nil {
				m := v.assets.Motions[p.action.Skill]
				if len(m.Frames) > 0 {
					action, frame = m.PoseAt(f.Cell, battleTarget(p.action), now.Sub(p.at), b.moveKind)
					lift = m.LiftAt(f.Cell, battleTarget(p.action), now.Sub(p.at))
				}
			}
		}
		// The shadow stays on the ground while a leap lifts the fighter
		// (+0x2078 also lifts its name and bar).
		world.DrawShadow(c.Screen, c.Pics, b.shadow, at.X, at.Y)
		at.Y -= lift
		lit := image.Pt(c.Input.X, c.Input.Y).In(image.Rect(at.X-30, at.Y-90, at.X+30, at.Y+8)) && !c.uiHovered
		if b.human != nil {
			b.human.Lit = lit
			b.human.Hold(frame, false)
			b.human.DrawBody(c.Screen, at.X, at.Y, int32(action))
		}
		if b.npc != nil {
			// Compatibility: a monster sprite without a motion's action
			// (some lack 39 or the run actions) keeps its battle stance
			// instead of vanishing.
			if w, _ := b.npc.FrameSize(action); w == 0 && action != battleIdleLeft && action != battleIdleRight {
				action, frame = battleIdleLeft, -1
				if f.X <= 2 {
					action = battleIdleRight
				}
			}
			b.npc.SetLit(lit)
			b.npc.Hold(frame, false)
			b.npc.Draw(c.Screen, at.X, at.Y, action)
		}
		c.drawBattleFighterOverlay(f, b, at)
	}
	c.drawBattleEffects(now)
	c.drawBattleTimer(now)
	for _, f := range v.floats {
		elapsed := now.Sub(f.at)
		at := f.cell.Position()
		y := at.Y - 60 - int(elapsed/(30*time.Millisecond))
		if f.font != "" && c.Pics.Find(f.font) >= 0 {
			minigame.DrawNumber(c.Screen, c.Pics, at.X, y, int(f.amount), true, battleDigitStyle, f.font)
		} else {
			c.Env.Text.Draw(at.X-len(f.text)*4, y, 0, false, true, c.Screen, f.text, 18, 160, 0, f.ink, 2)
		}
	}
	return true
}
func (c *Client) battleHit(a clientbattle.Action, now, cueAt time.Time, motion clientbattle.Motion, cue int) {
	for _, target := range a.Targets {
		// FUN_003790d4: hit targets play the attack's reaction motion.
		if cue == 0 && motion.Reaction != 0 && target.Result != protocol.BattleHitMiss && target.Cell != a.Actor {
			// It starts at the cue's time, so a late frame does not delay it.
			if r, ok := c.battle.assets.Motions[motion.Reaction]; ok && now.Sub(cueAt) < r.Duration() {
				c.battle.reactions = append(c.battle.reactions, battlePlay{action: clientbattle.Action{Actor: target.Cell, Skill: motion.Reaction}, at: cueAt})
			}
		}
		if target.Result == protocol.BattleHitMiss {
			if cue != 0 {
				continue
			}
			c.battle.floats = append(c.battle.floats, battleFloat{cell: target.Cell, text: []byte("Miss"), ink: battleTextInk, at: now})
			continue
		}
		for _, result := range target.Stats {
			amount := motion.DamageParts(result.Amount)[cue]
			if amount == 0 {
				continue
			}
			ink := uint16(0xf800)
			font := battleDamageDigits
			if result.Mode == protocol.BattleStatCriticalDamage {
				ink = 0xffe0
			}
			if result.Mode == protocol.BattleStatRecovery || result.Mode == protocol.BattleStatCriticalRecovery {
				ink = 0x07e0
				// Recovery digit/tint selection remains unresolved. Retain the
				// clearly identifiable compatibility text until it is traced.
				font = ""
			}
			if result.Stat == game.StatCurrentSP {
				ink = 0x001f
				font = battleSPDigits
			}
			c.battle.floats = append(c.battle.floats, battleFloat{cell: target.Cell, text: []byte(fmt.Sprint(amount)), ink: ink, at: now, font: font, amount: amount})
		}
	}
}
func (c *Client) battleActor() (remoteFighter, bool) {
	v := &c.battle
	for _, f := range c.remote.battle.fighters {
		if v.state.CanChoose(clientbattle.Cell{X: f.x, Y: f.y}) && !c.hotbar.submitted[[2]byte{f.x, f.y}] && (v.selected == (clientbattle.Cell{}) || v.selected == (clientbattle.Cell{X: f.x, Y: f.y})) {
			v.selected = clientbattle.Cell{X: f.x, Y: f.y}
			return f, true
		}
	}
	v.selected = clientbattle.Cell{}
	for _, f := range c.remote.battle.fighters {
		if v.state.CanChoose(clientbattle.Cell{X: f.x, Y: f.y}) && !c.hotbar.submitted[[2]byte{f.x, f.y}] {
			v.selected = clientbattle.Cell{X: f.x, Y: f.y}
			return f, true
		}
	}
	return remoteFighter{}, false
}
func (c *Client) battleBinding(id uint16, actor remoteFighter) hud.Binding {
	b := hud.Binding{Kind: protocol.HotbarSkill, ID: id}
	if actor.kind == clientbattle.Pet {
		for i, p := range c.InventoryState.Pets {
			if p.ID != 0 && game.BroadcastID(uint32(p.ID)) == actor.id {
				b.Target = byte(i + 1)
				b.Pet = p.ID
				break
			}
		}
	}
	return b
}
func (c *Client) battleMenu() {
	v := &c.battle
	actor, ok := c.battleActor()
	show := ok && v.binding == nil && len(v.playing) == 0 && !v.timedOut && !v.auto && !(c.remote.active && c.remote.options.AutoFight)
	if !show {
		if v.menu != nil {
			v.menu.Visible = false
		}
		return
	}
	if v.menu == nil {
		// Native radial controls: constructor 0x002974a0 (missing from the
		// decompile; verified in aLogin's instructions), dispatch FUN_00297f84.
		// Transparent parent leaves the fighter available as a friendly target.
		f := seui.NewForm(c.Env)
		f.Dockable = false
		f.Draggable = false
		f.Init("", 0, 0, 0, 0, 0, false, battleCommandHeight, battleCommandWidth, 0)
		f.PixelHit = true
		v.menu = f
		commands := []struct {
			command, icon, x, y int
			label               string
		}{{battleCommandAttack, battleIconAttack, 12, 88, "Attack"}, {battleCommandSkills, battleIconSkills, 19, 41, "Skills"}, {battleCommandDefend, battleIconDefend, 64, 19, "Defend"}, {battleCommandCapture, battleIconCapture, 108, 41, "Capture"}, {battleCommandFlee, battleIconFlee, 119, 88, "Flee"}, {battleCommandItems, battleIconItems, 90, 126, "Items"}, {battleCommandAuto, battleIconAuto, 42, 126, "Auto"}}
		for _, command := range commands {
			choice := command.command
			b := seui.NewFixedButton(c.Env, f)
			b.Init(fmt.Sprintf("Btn_Battle_%d_1", command.icon), command.x, battleCommandButtonSize, battleCommandButtonSize, 0, 0, true, battleCommandButtonSize, battleCommandButtonSize, command.y)
			b.SetHint([]byte(command.label))
			b.OnClick = func() { c.chooseBattleCommand(choice) }
		}
		v.previous = seui.NewFixedButton(c.Env, f)
		v.previous.Init("", battlePreviousX, battleCommandButtonSize, battleCommandButtonSize, 0, 0, true, battleCommandButtonSize, battleCommandButtonSize, battlePreviousY)
		v.previous.Visible = false
		v.previous.OnClick = func() { c.chooseBattleCommand(battleCommandPrevious) }
		c.UI.Add(f)
	}
	last, known := v.lastActions[clientbattle.Cell{X: actor.x, Y: actor.y}]
	v.previous.Visible = known && last.skill != 0 && last.id == actor.id && last.owner == actor.owner && last.kind == actor.kind
	if v.previous.Visible {
		icon := battleIconSkills // Native previous command uses its radial command artwork.
		switch last.skill {
		case skills.BasicAttack:
			icon = battleIconAttack
		case skills.Defense:
			icon = battleIconDefend
		case battleCatchSkill:
			icon = battleIconCapture
		case battleFleeSkill:
			icon = battleIconFlee
		}
		v.previous.Image = c.Pics.Find(fmt.Sprintf("Btn_Battle_%d_1", icon))
		v.previous.SetHint([]byte("Repeat last action"))
	}
	at := (clientbattle.Cell{X: actor.x, Y: actor.y}).Position()
	v.menu.Left = at.X - battleCommandOriginX
	v.menu.Top = at.Y - battleCommandOriginY
	v.menu.Visible = true
}
func (c *Client) chooseBattleCommand(command int) {
	if c.battleAnimating() {
		return
	}
	actor, ok := c.battleActor()
	if !ok {
		return
	}
	switch command {
	case battleCommandAttack:
		b := c.battleBinding(skills.BasicAttack, actor)
		c.battle.binding = &b
	case battleCommandSkills:
		b := c.battleBinding(skills.BasicAttack, actor)
		c.Skills.Selected = b.Target
		c.Skills.Show()
	case battleCommandDefend:
		c.submitHotbar(c.battleBinding(skills.Defense, actor), actor)
	case battleCommandCapture:
		b := c.battleBinding(battleCatchSkill, actor)
		c.battle.binding = &b
	case battleCommandFlee:
		c.submitBattleSpecial(actor, actor, battleFleeSkill, protocol.BattleActionFleeRequest)
	case battleCommandItems:
		// FUN_00297f84 case 4 opens the item form (FUN_00351700). Battle
		// item use (AC50:2) is not executed by the server yet.
		if c.Inventory != nil {
			c.Inventory.Show()
		}
	case battleCommandAuto:
		c.battle.auto = true
		c.battleAutoTick()
	case battleCommandPrevious:
		last, ok := c.battle.lastActions[clientbattle.Cell{X: actor.x, Y: actor.y}]
		if !ok || last.id != actor.id || last.owner != actor.owner || last.kind != actor.kind {
			return
		}
		switch last.skill {
		case battleCatchSkill:
			b := c.battleBinding(last.skill, actor)
			c.battle.binding = &b
		case battleFleeSkill:
			c.submitBattleSpecial(actor, actor, last.skill, protocol.BattleActionFleeRequest)
		default:
			c.useBinding(c.battleBinding(last.skill, actor))
		}
	}
}

func (c *Client) rememberBattleSubmission(actor remoteFighter, skill uint16) {
	if c.battle.pendingActions == nil {
		c.battle.pendingActions = map[clientbattle.Cell]battlePreviousAction{}
	}
	c.battle.pendingActions[clientbattle.Cell{X: actor.x, Y: actor.y}] = battlePreviousAction{actor.id, actor.owner, actor.kind, skill}
}
func (c *Client) submitBattleSpecial(actor, target remoteFighter, skill uint16, sub byte) {
	if c.battleAnimating() || !c.battle.state.CanChoose(clientbattle.Cell{X: actor.x, Y: actor.y}) || c.hotbar.submitted[[2]byte{actor.x, actor.y}] || c.remote.active && c.remote.options.AutoFight {
		return
	}
	if skill == battleCatchSkill && (target.kind != clientbattle.Monster || target.side == actor.side || target.hp == 0) {
		return
	}
	p := protocol.Builder{protocol.CommandBattleAction, sub, actor.x, actor.y, target.x, target.y}.U16(skill)
	if err := c.Net.Send(p); err != nil {
		c.Chat.Notice(err.Error())
		return
	}
	if c.hotbar.submitted == nil {
		c.hotbar.submitted = map[[2]byte]bool{}
	}
	c.hotbar.submitted[[2]byte{actor.x, actor.y}] = true
	c.rememberBattleSubmission(actor, skill)
	c.battle.binding = nil
}

func (c *Client) battleAnimating() bool {
	return c.battle.state.Active && (len(c.battle.playing) > 0 || len(c.battle.state.Actions) > 0)
}

func (c *Client) battleClick(x, y int) bool {
	if !c.battle.state.Active {
		return false
	}
	if c.battleAnimating() {
		return true
	}
	actor, ok := c.battleActor()
	if !ok {
		return true
	}
	for _, target := range c.remote.battle.fighters {
		cell := clientbattle.Cell{X: target.x, Y: target.y}
		at := cell.Position()
		if !image.Pt(x, y).In(image.Rect(at.X-30, at.Y-90, at.X+30, at.Y+8)) {
			continue
		}
		if c.battle.state.CanChoose(cell) && c.battle.binding == nil {
			c.battle.selected = cell
			return true
		}
		binding := c.battleBinding(skills.BasicAttack, actor)
		if c.battle.binding != nil {
			binding = *c.battle.binding
		}
		if binding.ID == battleCatchSkill {
			c.submitBattleSpecial(actor, target, battleCatchSkill, protocol.BattleActionAttackRequest)
		} else if target.side != actor.side || binding.ID != skills.BasicAttack {
			c.submitHotbar(binding, target)
			c.battle.binding = nil
		}
		return true
	}
	return true
}

// drawBattleFighterOverlay is a fighter's health bar (FUN_00437f28, called
// for every shown fighter by FUN_0039bd48) and name (0x4147f8 through the
// fighter's overlay slot, FUN_0038ace4).
func (c *Client) drawBattleFighterOverlay(f clientbattle.Fighter, b *battleBody, at image.Point) {
	height := b.height
	if height == 0 {
		height = world.DefaultBattleHeight
	}
	barY := max(at.Y-height, battleBarTopMin)
	if frame := c.Pics.Find(battleBarFrame); frame >= 0 {
		c.Pics.Draw(c.Screen, frame, at.X-battleBarLeft, barY, true)
	}
	if fill := c.Pics.Find(battleBarFill); fill >= 0 && f.MaxHP > 0 {
		// Delphi Round(hp*40/max).
		w := int((uint64(f.HP)*battleBarFull*2 + uint64(f.MaxHP)) / (2 * uint64(f.MaxHP)))
		if w > 0 {
			_, h := c.Pics.Size(fill)
			c.Pics.DrawRect(c.Screen, fill, at.X-battleBarLeft, barY, image.Rect(0, 0, w, h), true)
		}
	}
	ink := uint16(battleNamePlayerInk)
	nameY := barY - battleNameBase - battleNameLiftTall
	if f.Kind == clientbattle.Monster {
		ink = battleNameMonsterInk
		if height < battleNameTallLimit {
			nameY = barY - battleNameBase - battleNameLiftLow
		}
	} else if f.Kind == clientbattle.Player {
		nameY = barY - battleNameBase + battlePlayerNameLift
	}
	width := len(b.name) * battleNameCharWidth
	c.Env.Text.Draw(at.X-width/2, nameY, 0, false, true, c.Screen, b.name, battleNameHeight, width+battleNameCharWidth, 0, ink, battleNameStyle)
}

// battleTimerTick runs the turn countdown: FUN_0039783c starts it at the
// battle kind's limit when a round opens with one of our fighters to
// command; FUN_0039800c counts it down each second, sounding wav0145 below
// ten; at -1 FUN_00398120 closes the command menus. An action batch
// (FUN_003920a0) stops it.
func (c *Client) battleTimerTick(now time.Time) {
	v := &c.battle
	if len(v.state.Actions) > 0 || len(v.playing) > 0 {
		v.timerOn = false
	}
	if v.state.Ready && v.timerTurn != v.state.Turn {
		v.timerTurn = v.state.Turn
		v.timedOut = false
		if _, ok := c.battleActor(); ok {
			v.timerOn = true
			v.timerStart = now
			v.timerLimit = v.state.TurnLimit()
			v.timerShown = v.timerLimit
		}
	}
	if !v.timerOn {
		return
	}
	left := v.timerLimit - int(now.Sub(v.timerStart)/time.Second)
	for v.timerShown > left && v.timerShown >= 0 {
		v.timerShown--
		if v.timerShown >= 0 && v.timerShown < battleTimerWarnFrom && c.Env.Sound != nil {
			c.Env.Sound(battleTimerSound)
		}
	}
	if v.timerShown < 0 {
		v.timerOn = false
		v.timedOut = true
		v.binding = nil
		if c.Skills != nil {
			c.Skills.Hide()
		}
	}
}

// drawBattleTimer is FUN_003977a8: the seconds left, centred on the screen
// (50 lower in kinds 2 and 7).
func (c *Client) drawBattleTimer(now time.Time) {
	v := &c.battle
	if !v.timerOn || v.timerShown < 0 {
		return
	}
	y := battleTimerY
	if v.state.Kind == 2 || v.state.Kind == 7 {
		y += battleTimerLowerY
	}
	if c.Pics.Find(battleTimerDigits) >= 0 {
		minigame.DrawNumber(c.Screen, c.Pics, battleTimerX, y, v.timerShown, true, battleDigitStyle, battleTimerDigits)
	}
}

// battleOverlay is FUN_0038f460, drawn after the forms: the hovered
// fighter's details (FUN_0038ad1c) while commands are being chosen.
func (c *Client) battleOverlay() {
	v := &c.battle
	if !v.state.Active || !v.timerOn || c.uiHovered {
		return
	}
	for _, f := range v.state.Fighters {
		if f.Removed {
			continue
		}
		at := f.Position()
		if !image.Pt(c.Input.X, c.Input.Y).In(image.Rect(at.X-30, at.Y-90, at.X+30, at.Y+8)) {
			continue
		}
		b := c.battleBody(f)
		element := "None"
		if int(f.Element) < len(battleElements) {
			element = battleElements[f.Element]
		}
		lines := []struct {
			text []byte
			lift int
		}{{b.name, battleInfoName}, {[]byte(element), battleInfoElem}, {[]byte(fmt.Sprintf("LV:%d", f.Level)), battleInfoLevel}, {[]byte(fmt.Sprintf("HP:%d/%d", f.HP, f.MaxHP)), battleInfoHP}}
		for _, line := range lines {
			width := len(line.text) * battleNameCharWidth
			x := at.X + battleInfoRight
			if f.X == 1 || f.X == 3 {
				x = at.X - battleInfoLeft - width
			}
			c.Env.Text.Draw(x, at.Y-line.lift, 0, false, true, c.Screen, line.text, battleNameHeight, width+battleNameCharWidth, 0, battleInfoInk, battleNameStyle)
		}
		return
	}
}

// battleAutoTick is the radial "auto" command (FUN_00297f84 case 5,
// FUN_0039c238): once per round each of our fighters attacks (skill 10001)
// the first living fighter of the other half, scanning columns 1-2 from the
// right half and 4-3 from the left, rows 1-4 (FUN_0039a5b8). The auto-play
// form's configured skills (PTR_DAT_004ca6b0) are not ported; the native
// fallback attack is used.
func (c *Client) battleAutoTick() {
	v := &c.battle
	c.battleAutoCancel()
	if !v.auto || !v.state.Ready || v.autoTurn == v.state.Turn || c.battleAnimating() {
		return
	}
	v.autoTurn = v.state.Turn
	for _, actor := range c.remote.battle.fighters {
		if !v.state.CanChoose(clientbattle.Cell{X: actor.x, Y: actor.y}) || c.hotbar.submitted[[2]byte{actor.x, actor.y}] {
			continue
		}
		columns := []byte{4, 3}
		if actor.x >= 3 {
			columns = []byte{1, 2}
		}
		target, found := remoteFighter{}, false
		for _, x := range columns {
			for y := byte(1); y <= clientbattle.GridRows && !found; y++ {
				for _, f := range c.remote.battle.fighters {
					if f.x == x && f.y == y && f.hp > 0 {
						target, found = f, true
						break
					}
				}
			}
			if found {
				break
			}
		}
		if !found {
			return
		}
		c.submitHotbar(c.battleBinding(skills.BasicAttack, actor), target)
	}
}

// battleAutoCancel shows the auto cancel button while automatic actions run.
func (c *Client) battleAutoCancel() {
	v := &c.battle
	if v.autoForm == nil {
		if !v.auto {
			return
		}
		f := seui.NewForm(c.Env)
		f.Dockable, f.Draggable = false, false
		f.Init("", 0, 0, 0, 0, 0, false, battleCommandButtonSize, battleCommandButtonSize, 0)
		b := seui.NewFixedButton(c.Env, f)
		b.Init(fmt.Sprintf("Btn_Battle_%d_1", battleIconAuto), battleAutoCancelX, battleCommandButtonSize, battleCommandButtonSize, 0, 0, true, battleCommandButtonSize, battleCommandButtonSize, battleAutoCancelY)
		b.OnClick = func() { c.battle.auto = false }
		v.autoForm = f
		c.UI.Add(f)
	}
	v.autoForm.Visible = v.auto
}

// battleTarget is an action's first target, which approach paths aim at.
func battleTarget(a clientbattle.Action) clientbattle.Cell {
	if len(a.Targets) == 0 {
		return clientbattle.Cell{}
	}
	return a.Targets[0].Cell
}

// playBattleMusic plays the battle scene record's music (battle scenes
// such as 59081 name BGM0014), as map scenes name theirs.
func (c *Client) playBattleMusic() {
	if c.Music == nil {
		return
	}
	if c.sceneMusic == nil {
		c.sceneMusic, _ = world.SceneMusic(c.Assets)
	}
	if track := c.sceneMusic[c.battle.assets.Scene(c.battle.state.Background)]; track != "" {
		c.Music.Play(musicDir + track + musicExt)
	}
}

// Battle effects (FUN_0037273c): a frame of the effect's picture strip
// centred on its point, added with the light blit at level 30 when the
// picture's first pixel is the light marker 1 (FUN_00370c68), else keyed.
const (
	battleEffectLightMarker = 1
	battleEffectLightLevel  = 0x1e
)

func (c *Client) drawBattleEffects(now time.Time) {
	v := &c.battle
	if v.assets == nil {
		return
	}
	for _, play := range v.playing {
		m, ok := v.assets.Motions[play.action.Skill]
		if !ok || len(m.Effects) == 0 {
			continue
		}
		for _, e := range m.EffectsAt(play.action.Actor, battleTarget(play.action), now.Sub(play.at)) {
			i := c.Pics.Find(e.Picture)
			if i < 0 && !v.missing[e.Picture] {
				c.loadPictures(e.Picture)
				if i = c.Pics.Find(e.Picture); i < 0 {
					if v.missing == nil {
						v.missing = map[string]bool{}
					}
					v.missing[e.Picture] = true
				}
			}
			if i < 0 {
				continue
			}
			w, h := c.Pics.Size(i)
			fh := h / max(e.Frames, 1)
			r := image.Rect(0, (e.Frame-1)*fh, w, e.Frame*fh)
			x, y := e.At.X-w/2, e.At.Y-fh/2
			if c.Pics.Pixel(i, 0, 0) == battleEffectLightMarker {
				c.Pics.DrawLight(c.Screen, i, x, y, r, battleEffectLightLevel)
			} else {
				c.Pics.DrawRect(c.Screen, i, x, y, r, true)
			}
		}
	}
}
