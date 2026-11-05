package world

import (
	"encoding/binary"
	"time"
	"wonderland-gonline/internal/protocol"
)

const (
	npcExpressionBytes             = 3
	npcAnimationBytes              = 3 // actor UInt16, action UInt8 (captured Legacy layout).
	npcAnimationCompatibilityBytes = 4
	npcExpressionBase              = 100
)

// ApplyNPCPresentation mirrors FUN_004437d0/00443868. The final animation
// compatibility byte sent by Gonline is optional and unused by that native
// handler. The captured Legacy honey hand-in sends only actor and action.
func (w *World) ApplyNPCPresentation(sub byte, p []byte, now time.Time) bool {
	if sub == protocol.SceneActorExpression && len(p) != npcExpressionBytes || sub == protocol.SceneActorAnimation && len(p) != npcAnimationBytes && len(p) != npcAnimationCompatibilityBytes {
		return false
	}
	if sub != protocol.SceneActorExpression && sub != protocol.SceneActorAnimation {
		return false
	}
	n := w.NPCs[binary.LittleEndian.Uint16(p)]
	if n == nil {
		return true
	}
	if sub == protocol.SceneActorExpression {
		code := int(p[2]) + npcExpressionBase
		if code < expressionFirst || code >= expressionLimit {
			return false
		}
		if w.NPCExpressions == nil {
			w.NPCExpressions = map[uint16]Expression{}
		}
		w.NPCExpressions[n.ClickID] = Expression{Code: byte(code), Started: now}
		return true
	}
	action := int(p[2])
	if painter, ok := n.Painter.(interface{ FrameSize(int) (int, int) }); ok {
		found := false
		for candidate := action; candidate < action+actionFamily(action); candidate++ {
			if width, height := painter.FrameSize(candidate); width > 0 && height > 0 {
				action = candidate
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	n.route, n.walker = npcRoute{}, Walker{}
	n.Action, n.Fixed, n.Wrap, n.Frame = action, false, false, 0
	// FUN_00443868 -> FUN_0030a344: scripted animation supersedes the
	// facing saved by dialogue, including a previously held lying pose.
	n.Conversation = false
	if painter, ok := n.Painter.(interface{ RestartAnimation() }); ok {
		painter.RestartAnimation()
	}
	return true
}
