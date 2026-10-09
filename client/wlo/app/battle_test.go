package app

import (
	"bytes"
	"os"
	"testing"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/cursor"
	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
)

func enterBattle(t *testing.T) (*Client, *time.Time, func() [][]byte) {
	t.Helper()
	c, now, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	enemy := remoteRecord(5, 7, 2, 2, 20001, 0)
	enemy[2] = 2
	for _, p := range [][]byte{remoteRecord(250, 2, 4, 2, 10001, 0), enemy, {50, 6, 4, 2, 0}, {52, 1}} {
		c.dispatch(p)
	}
	return c, now, sent
}
func TestBattleSceneCommandsAndExit(t *testing.T) {
	c, now, sent := enterBattle(t)
	if !c.battleFrame(true) || c.battle.menu == nil || !c.battle.menu.Visible {
		t.Fatal("battle commands unavailable")
	}
	if path := os.Getenv("BATTLE_SNAPSHOT"); path != "" {
		c.Frame()
		savePNG(t, path, c)
	}
	c.WalkKeys(true, false, false, false)
	if c.World.Walking() {
		t.Fatal("battle allowed world movement")
	}
	c.chooseBattleCommand(0)
	c.GroundClick(230, 370)
	if p := sent(); len(p) != 1 || !bytes.Equal(p[0], []byte{50, 1, 4, 2, 2, 2, 0x11, 0x27}) {
		t.Fatalf("attack %x", p)
	}
	c.GroundClick(230, 370)
	if len(sent()) != 1 {
		t.Fatal("duplicate action")
	}
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	c.chooseBattleCommand(4)
	if p := sent(); len(p) != 2 || !bytes.Equal(p[1], []byte{50, 5, 4, 2, 4, 2, 0x89, 0xea}) {
		t.Fatalf("flee %x", p)
	}
	*now = now.Add(time.Second)
	c.dispatch([]byte{11, 0, 0x11, 0x27, 0, 0, 0, 0})
	if c.battleFrame(true) || c.battle.menu != nil || c.remote.battle.active {
		t.Fatal("battle exit retained UI")
	}
}
func TestBattleActionsBeforeDrawingApplyOnce(t *testing.T) {
	c, _, _ := enterBattle(t)
	p := []byte{50, 1, 17, 0, 4, 2, 0x11, 0x27, 0, 1, 2, 2, 1, 0, 1, 0x19, 25, 0, 0, 0, 1}
	c.dispatch(p)
	c.dispatch(p)
	if hp := c.battle.state.At(clientbattle.Cell{X: 2, Y: 2}).HP; hp != 50 {
		t.Fatalf("queued actions double applied: %d", hp)
	}
	c.battleFrame(false)
	if len(c.battle.state.Actions) != 0 || len(c.battle.playing) != 2 {
		t.Fatal("background workspace did not advance")
	}
}
func TestBattleCaptureSelection(t *testing.T) {
	c, _, sent := enterBattle(t)
	c.chooseBattleCommand(3)
	c.GroundClick(660, 370)
	if len(sent()) != 0 {
		t.Fatal("capture targeted ally")
	}
	c.GroundClick(230, 370)
	if p := sent(); len(p) != 1 || !bytes.Equal(p[0], []byte{50, 1, 4, 2, 2, 2, 0x18, 0x27}) {
		t.Fatalf("capture %x", p)
	}
}

func TestBattlePreviousActionUsesAcknowledgedHistory(t *testing.T) {
	c, now, sent := enterBattle(t)
	c.battleFrame(false)
	if c.battle.previous == nil || c.battle.previous.Visible {
		t.Fatal("new battle invented a previous action")
	}
	c.chooseBattleCommand(battleCommandAttack)
	c.battleFrame(false)
	if c.battle.previous.Visible {
		t.Fatal("selecting a command changed performed history")
	}
	c.HotbarKey(27, 0)
	c.chooseBattleCommand(battleCommandDefend)
	if c.battle.lastActions[clientbattle.Cell{X: 4, Y: 2}].skill != 0 {
		t.Fatal("unacknowledged action changed performed history")
	}
	c.dispatch([]byte{53, 5, 4, 2})
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	c.battleFrame(false)
	if !c.battle.previous.Visible || c.battle.previous.Image != c.Pics.Find("Btn_Battle_1_1") || c.battle.previous.Left != 65 || c.battle.previous.Top != 83 {
		t.Fatal("previous defense icon does not match native radial control")
	}
	if path := os.Getenv("BATTLE_SNAPSHOT"); path != "" {
		c.Frame()
		savePNG(t, path, c)
	}
	c.battle.previous.OnClick()
	if p := sent(); len(p) != 2 || !bytes.Equal(p[1], []byte{50, 4, 4, 2, 4, 2, 0x75, 0xea}) {
		t.Fatalf("repeat defense %x", p)
	}
	// Authoritative basic-attack animation replaces the defense history.
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 0x11, 0x27, 0, 1, 2, 2, 1, 0, 1, 25, 1, 0, 0, 0, 1})
	c.battleFrame(false)
	*now = now.Add(time.Minute)
	c.battleFrame(false)
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	c.battleFrame(false)
	if c.battle.previous.Image != c.Pics.Find("Btn_Battle_6_1") {
		t.Fatal("executed action did not replace history")
	}
	c.chooseBattleCommand(battleCommandPrevious)
	if c.battle.binding == nil || c.battle.binding.ID != 10001 || len(sent()) != 2 {
		t.Fatal("repeating attack must request a fresh target")
	}
	c.HotbarKey(27, 0)
	c.InventoryState.Pets[0].ID = 17162
	c.dispatch(remoteRecord(5, 4, 3, 2, 17162, 10001))
	c.GroundClick(590, 370)
	c.battleFrame(false)
	if c.battle.previous.Visible {
		t.Fatal("player history leaked into pet controls")
	}
	c.chooseBattleCommand(battleCommandDefend)
	c.dispatch([]byte{53, 5, 3, 2})
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	c.battleFrame(false)
	if !c.battle.previous.Visible || c.battle.previous.Image != c.Pics.Find("Btn_Battle_1_1") {
		t.Fatal("pet did not retain its own previous action")
	}
	c.GroundClick(660, 370)
	c.battleFrame(false)
	if c.battle.previous.Image != c.Pics.Find("Btn_Battle_6_1") {
		t.Fatal("pet history replaced player history")
	}
	c.dispatch([]byte{11, 0, 0x11, 0x27, 0, 0, 0, 0})
	if c.battle.previous != nil || len(c.battle.lastActions) != 0 || len(c.battle.pendingActions) != 0 {
		t.Fatal("battle exit retained history controls")
	}
}

func TestRoamingPacketDispatchAndBackgroundProgression(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.World.NPCs[1] = &world.NPC{ClickID: 1, X: 100, Y: 100, Action: 8, Shown: true}
	var unhandled [][]byte
	c.Unhandled = func(p []byte) { unhandled = append(unhandled, p) }
	c.dispatch([]byte{22, 2, 1, 0, 200, 0, 100, 0, 2})
	*now = now.Add(time.Second)
	c.frame(false)
	if n := c.World.NPCs[1]; n.X != 140 || n.Y != 100 || n.Action != world.FaceRight || len(unhandled) != 0 {
		t.Fatal("background session did not play roaming packet", n, unhandled)
	}
	c.dispatch([]byte{22, 2, 1, 0, 200})
	if len(unhandled) != 1 {
		t.Fatal("malformed roaming packet was silently accepted")
	}
}

func TestBattleNativeTargetCursor(t *testing.T) {
	c, _, _ := enterBattle(t)
	for _, test := range []struct {
		id    uint16
		shape cursor.Shape
	}{{10001, cursor.ShapeSwordAlt}, {10008, cursor.ShapeGrab}, {11016, cursor.ShapeMagic}, {60021, cursor.ShapeNormal}, {60041, cursor.ShapeNormal}} {
		c.battle.binding = &hud.Binding{ID: test.id}
		c.Input.Hovered = nil
		c.updateCursor(c.Now())
		if c.Cursors.Current != test.shape {
			t.Fatalf("skill %d: cursor %d, want %d", test.id, c.Cursors.Current, test.shape)
		}
	}
	c.battle.binding = nil
	c.updateCursor(c.Now())
	if c.Cursors.Current != cursor.ShapeNormal {
		t.Fatal("cancelled target cursor retained")
	}
}

func TestBattleNativeSkillTargets(t *testing.T) {
	c, _, _ := enterBattle(t)
	actor, _ := c.battleActor()
	ally := actor
	ally.x = 3
	enemy := c.remote.battle.fighters[1]
	for _, test := range []struct {
		id      uint16
		target  remoteFighter
		allowed bool
	}{{15190, actor, true}, {15190, ally, false}, {15075, actor, false}, {15075, ally, true}, {15075, enemy, false}, {11053, ally, false}} {
		if got := c.hotbarTargetAllowed(hud.Binding{ID: test.id}, actor, test.target); got != test.allowed {
			t.Fatalf("skill %d: target %d/%d allowed=%v", test.id, test.target.x, test.target.y, got)
		}
	}
	ally.hp = 0
	if !c.hotbarTargetAllowed(hud.Binding{ID: 11053}, actor, ally) {
		t.Fatal("revival rejected defeated ally")
	}
	c.World.Player.Body, c.World.Player.Head = 4, 6 // Cure 2 Players stunt (11077).
	if !c.hotbarTargetAllowed(hud.Binding{ID: game.StarterStuntClientID}, actor, actor) || c.hotbarTargetAllowed(hud.Binding{ID: game.StarterStuntClientID}, actor, enemy) {
		t.Fatal("starter stunt used placeholder attack target policy")
	}
}

func TestBattleRevivalRechecksCurrentTarget(t *testing.T) {
	c, _, sent := enterBattle(t)
	c.SkillState.Learned[11053] = skills.Progress{Grade: 1}
	actor, _ := c.battleActor()
	actor.sp = 65535
	c.remote.battle.fighters[0] = actor
	ally := remoteFighter{side: actor.side, kind: clientbattle.Player, id: 10002, x: 4, y: 3, hp: 0}
	c.remote.battle.fighters = append(c.remote.battle.fighters, ally)
	c.battle.state.Fighters = append(c.battle.state.Fighters, clientbattle.Fighter{Cell: clientbattle.Cell{X: 4, Y: 3}, Kind: clientbattle.Player, ID: ally.id, Side: ally.side})
	binding := c.battleBinding(11053, actor)
	stale := ally
	stale.hp = 100 // Selection snapshot predates this fighter's defeat.
	c.submitHotbar(binding, stale)
	if len(sent()) != 1 {
		t.Fatal("revival ignored current defeated state")
	}
	c.hotbar.submitted = nil
	c.battle.state.At(clientbattle.Cell{X: 4, Y: 3}).Removed = true
	c.submitHotbar(binding, ally)
	if len(sent()) != 1 {
		t.Fatal("revival targeted departed fighter")
	}
}

func TestBattlePetNicknames(t *testing.T) {
	c, _, _ := enterBattle(t)
	c.InventoryState.Pets[0].ID = 17162
	c.InventoryState.Pets[0].Name = []byte("My pet")
	f := clientbattle.Fighter{Cell: clientbattle.Cell{X: 3, Y: 2}, Kind: clientbattle.Pet, ID: 17162, Owner: c.World.Player.ID}
	if name := string(c.battleBody(f).name); name != "My pet" {
		t.Fatalf("owned pet name %q", name)
	}
	f.Owner = 10002
	c.World.Companions = map[uint32]*world.Companion{f.Owner: {ID: f.ID, Name: []byte("Their pet")}}
	if name := string(c.battleBody(f).name); name != "Their pet" {
		t.Fatalf("peer pet name %q", name)
	}
}

func TestBattleNewTurnWaitsForPresentation(t *testing.T) {
	c, now, sent := enterBattle(t)
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 0x11, 0x27, 0, 1, 2, 2, 1, 0, 1, 25, 25, 0, 0, 0, 1})
	// Simulate a new prompt arriving while the previous action is still
	// queued or playing. Both radial and hotbar commands must wait.
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	for _, draw := range []bool{false, true} {
		c.chooseBattleCommand(2)
		actor, ok := c.battleActor()
		if !ok {
			t.Fatal("owned fighter not ready")
		}
		c.submitHotbar(c.battleBinding(60021, actor), actor)
		if len(sent()) != 0 {
			t.Fatal("action submitted before previous presentation finished")
		}
		c.battleFrame(draw)
	}
	*now = now.Add(time.Minute)
	c.battleFrame(false)
	c.chooseBattleCommand(2)
	if len(sent()) != 1 {
		t.Fatal("finished presentation did not release next turn")
	}
	*now = now.Add(time.Minute)
	c.battleFrame(false)
	if len(c.battle.floats) != 0 {
		t.Fatal("background session retained expired damage")
	}
}

func TestBattleSoundsPlayOnce(t *testing.T) {
	c, now, _ := enterBattle(t)
	var sounds []string
	c.Env.Sound = func(path string) { sounds = append(sounds, path) }
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 3, 0x2b, 0, 1, 2, 2, 1, 0, 1, 25, 20, 0, 0, 0, 1})
	c.battleFrame(false)
	*now = now.Add(time.Minute)
	c.battleFrame(false)
	c.battleFrame(false)
	if len(sounds) != 5 || sounds[0] != `sound\SEB0040.wav` || sounds[3] != `sound\SEB0031.wav` {
		t.Fatalf("sound cues %v", sounds)
	}
}

func TestBattleHUDVitalsStayTransient(t *testing.T) {
	c, _, _ := enterBattle(t)
	c.Stats.HP = 88
	c.dispatch([]byte{51, 1, 4, 2, 25, 45, 0, 0, 0})
	c.battleFrame(false)
	if c.MainStatus.Stats.HP != 45 || c.Stats.HP != 88 {
		t.Fatal("battle HUD did not preserve world snapshot")
	}
	c.dispatch([]byte{11, 0, 0x11, 0x27, 0, 0, 0, 0})
	if c.MainStatus.Stats != c.Stats {
		t.Fatal("battle HUD survived exit")
	}
}

// TestBattleTurnCountdown: a round with a command to choose starts the
// native countdown at the kind's limit (20 s for kind 1); wav0145 sounds
// each second below ten, and at -1 the command menu closes.
func TestBattleTurnCountdown(t *testing.T) {
	c, now, _ := enterBattle(t)
	c.dispatch([]byte{11, 10, 1})
	c.dispatch([]byte{52, 1})
	var played []string
	c.Env.Sound = func(p string) { played = append(played, p) }
	c.battleFrame(false)
	if !c.battle.timerOn || c.battle.timerShown != clientbattle.TurnLimitShort {
		t.Fatalf("countdown %v %d", c.battle.timerOn, c.battle.timerShown)
	}
	for range clientbattle.TurnLimitShort - 9 {
		*now = now.Add(time.Second)
		c.battleFrame(false)
	}
	if c.battle.timerShown != 9 || len(played) != 1 || played[0] != battleTimerSound {
		t.Fatalf("at nine seconds shown %d sounds %q", c.battle.timerShown, played)
	}
	for range 10 {
		*now = now.Add(time.Second)
		c.battleFrame(false)
	}
	if !c.battle.timedOut || c.battle.timerOn || c.battle.menu.Visible || len(played) != 10 {
		t.Fatalf("timeout: out %v on %v menu %v sounds %d", c.battle.timedOut, c.battle.timerOn, c.battle.menu.Visible, len(played))
	}
	// A battle of kind 2 counts from 30.
	c.dispatch([]byte{11, 10, 2})
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	c.battleFrame(false)
	if c.battle.timerShown != clientbattle.TurnLimitLong || c.battle.timedOut {
		t.Fatalf("kind 2 countdown %d", c.battle.timerShown)
	}
}

// TestBattleAutoCommand: the radial auto command attacks the first living
// fighter of the other half each round until its cancel button is pressed.
func TestBattleAutoCommand(t *testing.T) {
	c, _, sent := enterBattle(t)
	c.battleFrame(false)
	c.chooseBattleCommand(battleCommandAuto)
	if p := sent(); len(p) != 1 || !bytes.Equal(p[0], []byte{50, 1, 4, 2, 2, 2, 0x11, 0x27}) {
		t.Fatalf("auto attack %x", p)
	}
	c.battleFrame(false)
	if c.battle.menu.Visible || c.battle.autoForm == nil || !c.battle.autoForm.Visible {
		t.Fatal("auto left the radial open or hid its cancel button")
	}
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	c.battleFrame(false)
	if p := sent(); len(p) != 2 {
		t.Fatalf("auto did not repeat: %x", p)
	}
	c.battle.autoForm.Children[0].(*seui.FixedButton).OnClick()
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	c.battleFrame(false)
	if p := sent(); len(p) != 2 || c.battle.autoForm.Visible || !c.battle.menu.Visible {
		t.Fatalf("cancel left auto running: %d packets", len(p))
	}
}

// TestBattleHoverDetails: hovering a fighter while commands are chosen
// draws FUN_0038ad1c's details beside it; BATTLE_HOVER_SNAPSHOT saves it.
func TestBattleHoverDetails(t *testing.T) {
	c, _, _ := enterBattle(t)
	c.Frame()
	at := (clientbattle.Cell{X: 2, Y: 2}).Position()
	c.Input.X, c.Input.Y = at.X, at.Y-20
	before := append([]uint16(nil), c.Screen.Pix...)
	c.Frame()
	if !c.battle.timerOn {
		t.Fatal("no countdown while choosing")
	}
	changed := 0
	for y := at.Y - battleInfoName; y < at.Y; y++ {
		for x := at.X + battleInfoRight; x < at.X+battleInfoRight+80; x++ {
			if c.Screen.Pix[y*c.Screen.W+x] != before[y*c.Screen.W+x] {
				changed++
			}
		}
	}
	if changed == 0 {
		t.Fatal("no details right of a column-2 fighter")
	}
	if path := os.Getenv("BATTLE_HOVER_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
}

// TestBattleHitReaction: a hit target plays the attack's reaction motion
// (60011 for the basic attack) from the damage cue.
func TestBattleHitReaction(t *testing.T) {
	c, now, _ := enterBattle(t)
	c.Frame()
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 0x11, 0x27, 0, 1, 2, 2, 1, 0, 1, 0x19, 25, 0, 0, 0, 1})
	for range 40 {
		*now = now.Add(50 * time.Millisecond)
		c.battleFrame(false)
		for _, p := range c.battle.playing {
			if p.action.Actor == (clientbattle.Cell{X: 2, Y: 2}) && p.action.Skill == 60011 {
				return
			}
		}
	}
	t.Fatal("the target never played its reaction")
}

// TestBattleMusic: a battle plays its scene's music (BGM0014 for the
// fallback scene 59081) and the map's music returns afterwards.
func TestBattleMusic(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	c.Music = &Music{}
	c.playMapMusic()
	mapTrack := c.Music.current
	enemy := remoteRecord(5, 7, 2, 2, 20001, 0)
	enemy[2] = 2
	for _, p := range [][]byte{remoteRecord(250, 2, 4, 2, 10001, 0), enemy, {50, 6, 4, 2, 0}, {52, 1}} {
		c.dispatch(p)
	}
	c.battleFrame(false)
	if c.Music.current != `Sound\BGM0014.wav` {
		t.Fatalf("battle music %q", c.Music.current)
	}
	*now = now.Add(time.Second)
	c.dispatch([]byte{11, 0, 0x11, 0x27, 0, 0, 0, 0})
	if c.Music.current != mapTrack || mapTrack == "" {
		t.Fatalf("after the battle %q, map %q", c.Music.current, mapTrack)
	}
}
