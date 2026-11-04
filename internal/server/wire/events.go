// Package wire builds server-side protocol payloads from supplied definitions
// and snapshots. It owns no sessions, sockets, transactions or gameplay decisions.
package wire

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// EventTimerUpdate is the captured AC66:2 remaining-time notification.
func EventTimerUpdate(timer uint16, remaining time.Duration) []byte {
	p := protocol.Builder{protocol.CommandEventTimers, protocol.EventTimersUpdate, 1}.U16(timer)
	return binary.LittleEndian.AppendUint64(p, math.Float64bits(max(0, remaining.Seconds())))
}

// EventFrame is BuildEventFrame, the native AC20:1 layout: type, subject, actor, mode,
// value, text or question, branch.
func EventFrame(kind, subject byte, actor uint16, mode byte, value uint32, text uint16, step, branch byte) []byte {
	if kind == 1 {
		branch = 0
	} // Speech ends with a 24-bit talk ID, not a branch byte.
	return protocol.Builder{protocol.CommandEvent, protocol.EventFrame, 0, 0, 0, step, kind, subject}.U16(actor).U8(mode).U32(value).U16(text).U8(branch)
}

func SpeechFrame(subject byte, actor uint16, talk uint32, step byte) []byte {
	p := EventFrame(1, subject, actor, 1, 0, uint16(talk), step, 0)
	p[len(p)-1] = byte(talk >> 16)
	return p
}

// DialogueFrame is the legacy single-line dialogue packet with a 24-bit talk ID.
func DialogueFrame(step, portrait, speaker byte, talk uint32) []byte {
	return []byte{protocol.CommandEvent, protocol.EventFrame, 0, 0, 0, step, 1, portrait, speaker, 0, 1, 0, 0, 0, 0, byte(talk), byte(talk >> 8), byte(talk >> 16)}
}

func RecordPointStatus(char game.Character) []byte {
	if char.RecordPoint != nil && char.RecordPoint.Map != 0 {
		return []byte{protocol.CommandCharacterState, protocol.CharacterStateWireCode21, 1}
	}
	return []byte{protocol.CommandCharacterState, protocol.CharacterStateWireCode21, 2}
}

func ServiceQuestion(click uint16, question, branch byte) []byte {
	return []byte{protocol.CommandEvent, protocol.EventFrame, 0, 0, 0, 1, 6, 3, byte(click), 0, 0, 0, 0, 0, 0, question, 0, branch}
}

func StoryCuePacket(char game.Character, cue assets.StoryCue) ([]byte, error) {
	var p []byte
	d := cue.Data
	switch cue.Kind {
	case "territory_empty":
		p = protocol.Builder{protocol.CommandTerritory, protocol.TerritoryWireCode1, byte(d[0]), 0}.U32(0)
	case "movie":
		p = EventFrame(5, 0, 0, byte(d[0]), d[1], 0, byte(d[2]), byte(d[3]))
	case "pose":

		p = protocol.Builder{protocol.CommandPose, protocol.PoseBroadcast}.U32(char.ID).U8(byte(d[0]))
	case "character_state":
		p = protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateWireCode30, byte(d[0])}.U32(char.ID).U8(byte(d[1]))
	case "movie_handshake":
		p = []byte{protocol.CommandMovie, protocol.MovieWireCode12}
		for _, v := range d {
			p = append(p, byte(v))
		}
	case "actor_movement":
		p = protocol.Builder{protocol.CommandScene, protocol.SceneActorMovement}.U16(uint16(d[0])).U16(uint16(d[1]))
	case "actor_action":
		p = []byte{protocol.CommandScene, protocol.SceneActorAction}
		for _, v := range d {
			p = append(p, byte(v))
		}
	case "quest_state":
		p = protocol.Builder{protocol.CommandQuest, protocol.QuestMarkState}.U16(uint16(d[0])).U8(byte(d[1]))
	case "quest_flag":
		p = protocol.Builder{protocol.CommandQuest, protocol.QuestFlag}.U16(uint16(d[0])).U8(byte(d[1]))
	case "movement_lock":
		p = []byte{protocol.CommandMovement, protocol.MovementMovementLock, byte(d[0])}
	case "event_resume":
		p = []byte{protocol.CommandEvent, protocol.EventResume}
	case "event_close":
		p = []byte{protocol.CommandEvent, protocol.EventClose}
	case "step_complete":
		p = []byte{protocol.CommandEvent, protocol.EventStepComplete}
	case "character_refresh":
		p = []byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh}
	default:
		return nil, fmt.Errorf("unknown cinematic cue %q", cue.Kind)
	}

	return p, nil
}
