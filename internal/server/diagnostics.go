package server

import (
	"encoding/hex"
	"wonderland-gonline/internal/protocol"
)

const (
	luckyDrawTraceBytes  = 64
	nativePoseTraceBytes = 7 // AC32 subcommand, character ID and expression/pose.
)

// General packet diagnostics contain no payload bytes: authentication,
// creation and deletion requests carry credentials in their bodies.
func packetLogAttrs(p []byte) []any {
	attrs := []any{"bytes", len(p)}
	if len(p) > 0 {
		attrs = append(attrs, "action", p[0])
	}
	if len(p) > 1 {
		attrs = append(attrs, "code", p[1])
	}
	return attrs
}

// Lucky Draw and received fishing controls expose decoded bytes. Other commands retain metadata;
// AC35 in particular also carries deletion credentials. Bound malformed traces.
func packetTraceAttrs(p []byte, sent bool) []any {
	attrs := packetLogAttrs(p)
	if len(p) == 0 {
		return attrs
	}
	if p[0] == protocol.CommandLuckyDraw {
		attrs = append(attrs, "payload_hex", hex.EncodeToString(p[:min(len(p), luckyDrawTraceBytes)]), "payload_truncated", len(p) > luckyDrawTraceBytes)
		if !sent && len(p) >= 2 {
			intent := "unknown"
			if p[1] == protocol.LuckyDrawSpin {
				intent = "spin"
			}
			attrs = append(attrs, "lucky_draw_intent", intent, "expected_request_bytes", protocol.LuckyDrawRequestBytes)
		}
	}
	if len(p) >= 2 && p[0] == protocol.CommandInventory && (p[1] == protocol.InventoryPotentialPill || p[1] == protocol.InventoryPotentialPillResult) {
		limit := protocol.PotentialPillRequestBytes
		if sent {
			limit = protocol.PotentialPillResultBytes
		}
		attrs = append(attrs, "payload_hex", hex.EncodeToString(p[:min(len(p), limit)]), "payload_truncated", len(p) > limit)
	}
	if !sent && len(p) >= 2 && p[0] == protocol.CommandInventory && (p[1] == protocol.InventoryFishingStart || p[1] == protocol.InventoryFishingStop) {
		attrs = append(attrs, "payload_hex", hex.EncodeToString(p[:min(len(p), nativeFishingRequestBytes)]), "payload_truncated", len(p) > nativeFishingRequestBytes)
	}
	if len(p) >= 2 && p[0] == protocol.CommandMovement && (p[1] == protocol.MovementMove || p[1] == protocol.MovementStop) {
		attrs = append(attrs, "payload_hex", hex.EncodeToString(p[:min(len(p), protocol.MovementNativeRequestBytes)]), "payload_truncated", len(p) > protocol.MovementNativeRequestBytes)
	}
	if len(p) >= 2 && p[0] == protocol.CommandPose && (p[1] == protocol.PoseEmote || p[1] == protocol.PoseBroadcast || p[1] == protocol.PoseStop) {
		attrs = append(attrs, "payload_hex", hex.EncodeToString(p[:min(len(p), nativePoseTraceBytes)]), "payload_truncated", len(p) > nativePoseTraceBytes)
	}
	if sent && len(p) >= 5 && p[0] == protocol.CommandLuckyDraw && p[1] == protocol.LuckyDrawMode {
		switch p[2] {
		case protocol.LuckyDrawCatalog:
			attrs = append(attrs, "lucky_draw_used", p[3], "reward_count", p[4])
		case protocol.LuckyDrawResult:
			attrs = append(attrs, "result_slot", p[3], "lucky_draw_used", p[4])
		}
	}

	return attrs
}

// Read world state under its lock because another session can move this player.
func (s *Server) sessionLogAttrs(c *Session) []any {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	attrs := []any{"session", c.info.ID, "service", c.info.Service, "account", c.account.ID, "map_ready", c.ready}
	if c.character != nil {
		attrs = append(attrs, "character", c.character.ID, "map", c.character.Map)
	}
	return attrs
}
