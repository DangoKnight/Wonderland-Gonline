package role

import (
	"time"

	"wonderland-gonline/client/wlo/surface"
)

const (
	battleCloudFamily    = "009"
	battleCloudSprite    = 9000 // FUN_00412c50 replaces fighting map roles with this sprite.
	battleCloudAction    = 0
	battleCloudFrameTime = 100 * time.Millisecond
)

// DrawBattleCloud uses the native sprite's frame origins, keeping the dust at
// the character's feet. Elapsed time makes animation independent of draw rate.
func (l *Library) DrawBattleCloud(dst *surface.Surface, x, y int, elapsed time.Duration) bool {
	if l == nil {
		return false
	}
	arc, key := l.lookup(battleCloudFamily, battleCloudSprite)
	if arc == nil {
		return false
	}
	s := arc.sprite(key)
	if s == nil || s.frameCount(battleCloudAction) == 0 {
		return false
	}
	frame := int(max(elapsed, 0)/battleCloudFrameTime) % s.frameCount(battleCloudAction)
	f := s.frame(battleCloudAction, frame)
	if f == nil {
		return false
	}
	s.draw(dst, f, x, y)
	return true
}
