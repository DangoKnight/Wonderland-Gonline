package world

import (
	"image"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
)

type cloudTestBody struct {
	login.RoleView
	draws int
}

func (b *cloudTestBody) DrawBody(_ *surface.Surface, _, _ int, _ int32) { b.draws++ }

func TestBattlePresenceNativeBatchAndCleanup(t *testing.T) {
	now := time.Unix(100, 0)
	w := &World{Player: Player{ID: 1}, Peers: map[uint32]*Peer{2: {Player: Player{ID: 2}}}}
	w.SetCompanion(2, 123, []byte("Pet"), nil)
	// Independent native wire records: player, ID, reserved word, battle status.
	p := []byte{11, 4, 2, 1, 0, 0, 0, 0, 0, 1, 2, 2, 0, 0, 0, 0, 0, 1}
	if !w.ApplyBattlePresence(p, now) || !w.fighting(1) || !w.fighting(2) {
		t.Fatal("battle batch not applied")
	}
	if _, _, _, visible := w.companionPosition(2); visible || w.Companions[2].ID != 123 {
		t.Fatal("battle pet must hide without being removed")
	}
	if w.ApplyBattlePresence(append(p, 2), now) || !w.fighting(2) {
		t.Fatal("truncated batch mutated state")
	}
	w.ApplyBattlePresence(p, now.Add(time.Second))
	if w.battlePresence[2].started != now {
		t.Fatal("duplicate restarted animation")
	}
	w.ApplyBattlePresence([]byte{11, 4, 2, 2, 0, 0, 0, 0, 0, 4}, now)
	if w.fighting(2) {
		t.Fatal("spectator must retain normal body")
	}
	w.ApplyBattlePresence([]byte{11, 4, 2, 2, 0, 0, 0, 0, 0, 0}, now)
	if _, _, _, visible := w.companionPosition(2); !visible {
		t.Fatal("pet not restored")
	}
	w.ApplyBattlePresence([]byte{11, 0, 1, 0, 0, 0, 1, 0}, now)
	if !w.fighting(1) {
		t.Fatal("nonzero close result cleared presence")
	}
	if w.ApplyBattlePresence([]byte{11, 0, 1, 0, 0, 0, 0, 0}, now) || w.fighting(1) {
		t.Fatal("close must clear presence and reach combat handler")
	}
	w.ApplyBattlePresence(p, now)
	w.RemovePeer(2)
	if w.fighting(2) {
		t.Fatal("departed peer retained cloud")
	}
}

func TestBattleCloudReplacesBodyAtSortedFeet(t *testing.T) {
	now := time.Unix(100, 0)
	body := &cloudTestBody{}
	w := &World{Env: &seui.Env{Screen: surface.New(800, 600), Pics: picdb.New()}, Scene: &Scene{Width: 800, Height: 600}, Player: Player{ID: 1},
		Peers: map[uint32]*Peer{2: {Player: Player{ID: 2, X: 100, Y: 200}, Role: body}}, HideNames: true, Cinematic: true, CameraAt: &image.Point{}, Now: func() time.Time { return now }}
	draws := 0
	w.BattleCloud = func(dst *surface.Surface, x, y int, elapsed time.Duration) bool {
		if x != 100 || y != 200 || elapsed != time.Second {
			t.Fatal("cloud origin or clock incorrect", x, y, elapsed)
		}
		draws++
		dst.Fill(image.Rect(x, y-1, x+1, y), 7)
		return true
	}
	w.ApplyBattlePresence([]byte{11, 4, 2, 2, 0, 0, 0, 0, 0, 1}, now.Add(-time.Second))
	w.Draw()
	if draws != 1 || body.draws != 0 || w.Env.Screen.Pix[199*800+100] != 7 {
		t.Fatal("cloud failed to replace body")
	}
	w.ApplyBattlePresence([]byte{11, 4, 2, 2, 0, 0, 0, 0, 0, 0}, now)
	w.Draw()
	if draws != 1 || body.draws != 1 {
		t.Fatal("body not restored after combat")
	}
}

func TestBattleCloudNeverDrawsForLocalPlayer(t *testing.T) {
	now := time.Unix(100, 0)
	body := &cloudTestBody{}
	w := &World{Env: &seui.Env{Screen: surface.New(800, 600), Pics: picdb.New()},
		Scene: &Scene{Width: 800, Height: 600}, Player: Player{ID: 1, X: 100, Y: 200}, Body: body,
		HideNames: true, Cinematic: true, CameraAt: &image.Point{}, Now: func() time.Time { return now }}
	w.BattleCloud = func(*surface.Surface, int, int, time.Duration) bool {
		t.Fatal("local player's cloud rendered")
		return true
	}
	w.ApplyBattlePresence([]byte{11, 4, 2, 1, 0, 0, 0, 0, 0, 1}, now)
	w.Draw() // Participation may arrive before the battle roster.
	if body.draws != 1 {
		t.Fatal("local body hidden before battlefield initialization")
	}
}
