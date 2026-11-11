package app

import (
	"image"
	"testing"
	"time"

	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/settings"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/internal/assets"
)

const (
	cameraTestPlainSkill   uint16 = 30028 // Double Kick, the captured no-track attack.
	cameraTestTrackedSkill uint16 = 10001 // Hand Attack, the captured camera owner.
)

func TestBattleCameraSelectsActiveAttackTrack(t *testing.T) {
	start := time.Unix(0, 0)
	actor, target := clientbattle.Cell{X: 4, Y: 2}, clientbattle.Cell{X: 2, Y: 2}
	track := clientbattle.Motion{Frames: []clientbattle.MotionFrame{{Offset: image.Pt(200, 0), Hold: time.Second}}, CameraMode: battleCameraPath, Camera: []clientbattle.CameraPoint{{At: image.Pt(650, 150), Speed: 1, Trigger: 100}}}
	plain := battlePlay{at: start, action: clientbattle.Action{Actor: actor, Skill: cameraTestPlainSkill, Targets: []clientbattle.Target{{Cell: target}}}}
	tracked := battlePlay{at: start, action: clientbattle.Action{Actor: actor, Skill: cameraTestTrackedSkill, Targets: []clientbattle.Target{{Cell: target}}}}
	c := &Client{}
	c.battle.assets = &clientbattle.Assets{Motions: map[uint16]clientbattle.Motion{cameraTestTrackedSkill: track}}
	c.battle.playing = []battlePlay{plain, tracked}
	now := start.Add(200 * time.Millisecond)
	c.battleCamera(now)
	if want := motionCamera(track, actor, target, now.Sub(start)); c.battle.camera != want || want == battleCameraOrigin() {
		t.Fatal("plain action stole the authored track", c.battle.camera, want)
	}
	// A delayed attack and a targetless hit reaction cannot own the camera.
	delayed, reaction := tracked, tracked
	delayed.at = now.Add(time.Second)
	reaction.action.Targets = nil
	c.battle.playing = []battlePlay{delayed, reaction, plain}
	c.battleCamera(now)
	if c.battle.camera != battleCameraOrigin() {
		t.Fatal("inactive/reaction track stole camera ownership")
	}
	c.battle.playing = nil
	c.battleCamera(now)
	if c.battle.camera != battleCameraOrigin() || c.battle.zoom != image.Rect(0, 0, ScreenWidth, ScreenHeight) {
		t.Fatal("idle scene retained the previous camera")
	}
}

func TestUntrackedBattleZoomFramesTargetAndReturnsSmoothly(t *testing.T) {
	for _, actor := range []clientbattle.Cell{{X: 4, Y: 2}, {X: 2, Y: 2}} {
		target := clientbattle.Cell{X: 2, Y: 2}
		if actor.X == 2 {
			target.X = 4
		}
		motion := clientbattle.Motion{Frames: []clientbattle.MotionFrame{{Offset: image.Pt(300, 0), Hold: 150 * time.Millisecond}}}
		start := time.Unix(0, 0)
		c := &Client{SkillState: skills.NewState(&skills.Catalog{Definitions: map[uint16]skills.Definition{cameraTestPlainSkill: {Skill: assets.Skill{Attack: battleZoomAttackCategory}}}})}
		c.battle.assets = &clientbattle.Assets{Motions: map[uint16]clientbattle.Motion{cameraTestPlainSkill: motion}}
		c.battle.playing = []battlePlay{{at: start, action: clientbattle.Action{Actor: actor, Skill: cameraTestPlainSkill, Targets: []clientbattle.Target{{Cell: target}}}}}
		previous := battleCameraOrigin()
		zoomed := false
		for elapsed := time.Duration(0); elapsed <= motion.Duration()+time.Millisecond; elapsed += time.Millisecond {
			c.battleCamera(start.Add(elapsed))
			if c.battle.camera != clampBattleCamera(c.battle.camera) {
				t.Fatal("camera escaped battlefield")
			}
			if abs(c.battle.camera.X-previous.X) > 2 || abs(c.battle.camera.Y-previous.Y) > 2 {
				t.Fatal("fallback camera snapped", previous, c.battle.camera)
			}
			previous = c.battle.camera
			if c.battle.zoom.Dx() < ScreenWidth {
				zoomed = true
				projected := target.Position().Add(battleCameraOrigin()).Sub(c.battle.camera)
				if !projected.In(c.battle.zoom) {
					t.Fatal("zoom cropped the target", projected, c.battle.zoom)
				}
			}
		}
		if !zoomed || c.battle.camera != battleCameraOrigin() || c.battle.zoom != image.Rect(0, 0, ScreenWidth, ScreenHeight) {
			t.Fatal("fallback zoom did not run and settle")
		}
		c.SettingsState = settings.NewState()
		c.SettingsState.Local.Zoom = false
		c.battleCamera(start.Add(motion.ReturnStart() - time.Millisecond))
		if c.battle.camera != battleCameraOrigin() || c.battle.zoom != image.Rect(0, 0, ScreenWidth, ScreenHeight) {
			t.Fatal("disabled zoom still moved the fallback camera")
		}
	}
}
