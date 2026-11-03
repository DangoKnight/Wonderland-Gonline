package app

import (
	"image"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestBattleAdditionalSoundChannels(t *testing.T) {
	c, now, _ := enterBattle(t)
	c.battleFrame(false)
	m := clientbattle.Motion{Frames: []clientbattle.MotionFrame{{Action: 16, Hold: 600 * time.Millisecond}}, Sounds: []clientbattle.SoundCue{{ID: 10040, Trigger: 300}}}
	body := c.World.Player.Body
	m.WeaponSounds[0][1], m.WeaponSoundTriggers[1] = 10018, 0
	m.BodySounds[body][1], m.BodySoundTriggers[1] = 10164, 150
	c.battle.assets = &clientbattle.Assets{Motions: map[uint16]clientbattle.Motion{50000: m}}
	var played []string
	c.Env.Sound = func(path string) { played = append(played, path) }
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 80, 195, 0, 1, 2, 2, 1, 0, 1, 25, 25, 0, 0, 0, 1})
	start := *now
	for _, elapsed := range []time.Duration{0, 149 * time.Millisecond, 150 * time.Millisecond, 300 * time.Millisecond, 300 * time.Millisecond} {
		*now = start.Add(elapsed)
		c.battleFrame(false)
	}
	if want := []string{`sound\SEB0018.wav`, `sound\SEB0164.wav`, `sound\SEB0040.wav`}; !reflect.DeepEqual(played, want) {
		t.Fatalf("audio %v, want %v", played, want)
	}
}

func TestBattleReactionUsesAttackingWeapon(t *testing.T) {
	c, now, _ := enterBattle(t)
	c.battleFrame(false)
	c.items[65000] = assets.NativeItem{Definition: game.ItemDefinition{ID: 65000, Type: 1, EquipSlot: 3}}
	c.World.Player.Items = []uint16{65000}
	attack := clientbattle.Motion{Reaction: 60011, Events: []clientbattle.MotionEvent{{Trigger: 150, Percent: 100}}}
	reaction := clientbattle.Motion{Frames: []clientbattle.MotionFrame{{Action: 26, Hold: 300 * time.Millisecond}}}
	reaction.WeaponSounds[1][1] = 10018
	c.battle.assets = &clientbattle.Assets{Motions: map[uint16]clientbattle.Motion{10001: attack, 60011: reaction}}
	var played []string
	c.Env.Sound = func(path string) {
		if strings.HasPrefix(path, `sound\SEB`) {
			played = append(played, path)
		}
	}
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 25, 0, 0, 0, 1})
	c.battleFrame(false)
	*now = now.Add(150 * time.Millisecond)
	c.battleFrame(false)
	*now = now.Add(time.Millisecond)
	c.battleFrame(false)
	c.battleFrame(false)
	if !reflect.DeepEqual(played, []string{`sound\SEB0018.wav`}) {
		t.Fatalf("weapon impact %v", played)
	}
}

func TestBattleVoiceRouletteAndRecordings(t *testing.T) {
	basic := skills.Definition{Skill: assets.Skill{ID: 10001}}
	for _, tc := range []struct {
		roll int
		want string
	}{{1, "_bt43"}, {10, "_bt43"}, {11, "_bt42"}, {30, "_bt42"}, {31, "_bt41"}, {60, "_bt41"}, {61, ""}, {100, ""}} {
		if got := battleVoiceSuffix(basic, tc.roll, false, false, false); got != tc.want {
			t.Fatalf("roll %d: %s", tc.roll, got)
		}
	}
	group7 := skills.Definition{Skill: assets.Skill{ID: 11011, Type: 7}}
	if battleVoiceSuffix(group7, 100, false, false, false) != "_bt71" || battleVoiceSuffix(group7, 100, false, false, true) != "_bt61" {
		t.Fatal("player/companion voice groups conflated")
	}
	c, _, _ := enterBattle(t)
	if bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE"); bundle != "" {
		useCompiledPetAssets(t, c, bundle)
	}
	c.World.Player.Body, c.World.Player.Head = 2, 0
	a := clientbattle.Action{Actor: clientbattle.Cell{X: 4, Y: 2}, Skill: 11011, Targets: []clientbattle.Target{{Cell: clientbattle.Cell{X: 2, Y: 2}}}}
	path := c.battleVoice(a, "10", 2, 2)
	if !strings.HasPrefix(path, `..\audio\odd\1002_bt6`) {
		t.Fatalf("player battle voice %q", path)
	}
	sounds := &Sounds{Root: c.Assets.Media}
	for _, path := range []string{path, `sound\SEB0040.wav`, `sound\SEB0164.wav`} {
		if len(sounds.PCM(path)) == 0 {
			t.Fatalf("undecodable battle sound %s", path)
		}
	}
	// An existing recording suppresses the native body track and only
	// fires once even when the same clock instant is drawn repeatedly.
	c.battleFrame(false)
	p := c.newBattlePlay(a, c.Now(), a.Actor)
	var played []string
	c.Env.Sound = func(path string) { played = append(played, path) }
	c.battleAudio(&p, c.battle.assets.Motions[a.Skill], c.Now())
	c.battleAudio(&p, c.battle.assets.Motions[a.Skill], c.Now())
	voices := 0
	for _, path := range played {
		if strings.HasSuffix(path, ".ogg") {
			voices++
		}
	}
	if voices != 1 {
		t.Fatalf("voice fired %d times", voices)
	}
}

func TestBattleCameraPath(t *testing.T) {
	m := clientbattle.Motion{CameraMode: 2, Camera: []clientbattle.CameraPoint{
		{At: image.Pt(685, 192), Speed: .5, Trigger: 500},
		{At: image.Pt(400, 192), Speed: .5, Trigger: 500},
	}}
	for _, tc := range []struct {
		ms   int
		want image.Point
	}{{0, image.Pt(432, 232)}, {200, image.Pt(332, 232)}, {470, image.Pt(197, 232)}, {600, image.Pt(197, 232)}, {1000, image.Pt(212, 232)}, {2040, image.Pt(482, 232)}} {
		if got := motionCamera(m, clientbattle.Cell{X: 4, Y: 2}, clientbattle.Cell{X: 2, Y: 2}, time.Duration(tc.ms)*time.Millisecond); got != tc.want {
			t.Fatalf("camera at %d: %v, want %v", tc.ms, got, tc.want)
		}
	}
	if got := clampBattleCamera(image.Pt(-200, 900)); got != image.Pt(0, 232) {
		t.Fatal("field clamp", got)
	}
}

func TestBattleZoomAndSetting(t *testing.T) {
	c, now, _ := enterBattle(t)
	c.battleFrame(false)
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 25, 0, 0, 0, 1})
	c.battleFrame(false)
	m := c.battle.assets.Motions[10001]
	*now = now.Add(m.EventAt(m.Events[0]))
	c.Frame()
	if c.battle.zoom.Min.X <= 0 || c.battle.zoom.Dx() >= ScreenWidth || c.battle.camera == battleCameraOrigin() || c.battle.zoomCanvas == nil {
		t.Fatal("attack lacks camera movement/zoom", c.battle.camera, c.battle.zoom)
	}
	if path := os.Getenv("BATTLE_ZOOM_SNAPSHOT"); path != "" {
		c.Sprites.WaitNative()
		c.Frame()
		savePNG(t, path, c)
	}
	c.Settings.Zoom.OnClick()
	c.battleFrame(true)
	if c.SettingsState.Local.Zoom || c.battle.zoom != image.Rect(0, 0, 800, 600) {
		t.Fatal("Zoom Mode did not disable battle zoom")
	}
	c.Settings.Zoom.OnClick()
	*now = now.Add(time.Minute)
	c.battleFrame(false)
	if c.battle.zoom != image.Rect(0, 0, 800, 600) || c.battle.camera != battleCameraOrigin() {
		t.Fatal("action retained camera/zoom")
	}
	c.dispatch([]byte{11, 0, 17, 39, 0, 0, 0, 0})
	if c.battle.zoomCanvas != nil {
		t.Fatal("battle exit retained zoom target")
	}
}

func TestBattleRevealQueuesActionsAndAudio(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.dispatch(remoteRecord(250, 2, 4, 2, 10001, 0))
	enemy := remoteRecord(5, 7, 2, 2, 20001, 0)
	enemy[2] = 2
	c.dispatch(enemy)
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 3, 43, 0, 1, 2, 2, 1, 0, 1, 25, 25, 0, 0, 0, 1})
	var played []string
	c.Env.Sound = func(path string) { played = append(played, path) }
	c.Frame()
	*now = now.Add(sceneFadeSteps * sceneFadeFrame)
	c.Frame()
	if c.sceneFade.mode != sceneFadeRevealBattle || len(c.battle.playing) != 0 || len(c.battle.state.Actions) != 1 || len(played) != 0 {
		t.Fatal("action/audio ran behind entry fade", played)
	}
	*now = now.Add(sceneFadeSteps * sceneFadeFrame)
	c.Frame()
	if c.sceneFade.mode != sceneFadeNone || len(c.battle.playing) == 0 || len(c.battle.state.Actions) != 0 {
		t.Fatal("reveal did not release action")
	}
}
