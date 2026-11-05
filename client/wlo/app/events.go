package app

import (
	"encoding/binary"
	"fmt"
	"log"
	"time"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/movie"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/protocol"
)

// Events. Clicking a map NPC (FUN_00303b54) sends 20/1 with its click ID;
// the server answers with event frames (20/1..6, the receive case at
// 0x2e3dc0), which FUN_00304fd0 interprets by kind. A step the client has
// finished (+0x7108 set, nothing awaited) is acknowledged with 20/6 by the
// frame loop (FUN_00307300); 20/7 ends the event the same way, and 20/8
// (resume) clears it. While the server holds the player (6/2), clicks do
// not walk.
//
// Ported kinds: 1, a line of dialogue (FUN_00304fd0 case 1) spoken by a
// map NPC (subject 3) or the player (subject 7), shown in the talk window
// until clicked; 4, a map NPC walking one of its routes (world.WalkRoute),
// acknowledged once every route walker has arrived; 5, movies; 6,
// questions (the map's question table at +0x5d9c); 11 and 14, an NPC or
// the player turning; and 15, the event's music. Kinds 7-10, 12 and 13
// jump to the interpreter's end (table at 0x30502e) without finishing the
// step, but this repository's server waits for 20/6 after kinds 9 and 13,
// so they and every other kind are acknowledged at once, as kind 2 is.
const (
	// Frame layout after the command byte (0x2e3dc0).
	eventFrameKind    = 5
	eventFrameSubject = 6
	eventFrameActor   = 7 // u16
	eventFrameMode    = 9
	eventFrameText    = 14
	eventFrameBytes   = 17
	// Subcommands 1..6 carry frames.
	eventLastFrame = 6
	// Kinds (+0x7113) and speakers (+0x7114).
	eventKindTalk     = 1
	eventKindRoute    = 4
	eventKindMovie    = 5
	eventKindQuestion = 6
	eventKindNPCTurn  = 11
	eventKindTurn     = 14
	eventKindMusic    = 15
	// Kind 11 modes: toward the player, another NPC, or a direction.
	npcTurnPlayer    = 1
	npcTurnNPC       = 2
	npcTurnDirection = 3
	// Kind 14 subjects: toward an NPC, or a direction.
	turnTowardNPC = 1
	turnDirection = 2
	// Kind 15 subjects: play a track, or return to the map's.
	musicPlay    = 1
	musicRestore = 2
	// Speakers, and the talk mode that turns the NPC.
	npcTalkTurn      = 1
	eventSpeakerNPC  = 3
	eventSpeakerSelf = 7
	// eventChoiceCancel closes a question (the server's cancel value).
	eventChoiceCancel = 40
	// talkBodyAction is the body-mode pose: standing, facing down-left.
	talkBodyAction = 0xb
	// npcReach is FUN_00303b54's distance check, per axis.
	interactionNoticeDuration  = 2 * time.Second
	npcReach                   = 0xa9
	conversationStandingAction = 8
	conversationFacingCount    = 8
)

// eventState is the interpreter's step flags.
type eventState struct {
	active bool // a step is in progress (+0x70e8)
	done   bool // the step is finished and awaits 20/6 (+0x7108)
	// A question awaiting its answer (+0x710a): the reply value of each
	// option row, and whether the step also ends with 20/6 (mode 0).
	answers    []byte
	ackAnswer  bool
	npcFacings map[*world.NPC]int
}

// clickNPC is a left click on a map NPC: within reach it sends 20/1,
// otherwise it reports the range failure.
func (c *Client) clickNPC(n *world.NPC) {
	c.stopVacuum()
	p := c.World.Player
	if abs(p.X-n.X) <= npcReach && abs(p.Y-n.Y) <= npcReach {
		c.World.StopWalk()
		c.prepareVacuum(n.ClickID)
		c.sendNPCClick(n.ClickID)
		c.beginVacuum()
		return
	}
	c.pendingNPC = nil
	c.Notices.Show([]byte("Too far"), interactionNoticeDuration, c.Now())
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// sendNPCClick is 20/1 with the NPC's click ID.
func (c *Client) sendNPCClick(id uint16) {
	c.pendingNPC = nil
	c.acquisitionActor = id
	c.event = eventState{active: true}
	c.Net.Send(binary.LittleEndian.AppendUint16([]byte{protocol.CommandEvent, protocol.EventActorClick}, id))
}

// reachNPC sends a walk's pending click once the player stops near it.
func (c *Client) reachNPC() {
	n := c.pendingNPC
	if n == nil || c.World == nil || c.World.Walking() {
		return
	}
	c.pendingNPC = nil
	p := c.World.Player
	if abs(p.X-n.X) <= npcReach && abs(p.Y-n.Y) <= npcReach {
		c.sendNPCClick(n.ClickID)
	}
}

// eventFrame is 20/1..6: s is the packet after its command byte.
func (c *Client) eventFrame(s []byte) {
	if len(s) < eventFrameBytes || c.World == nil {
		return
	}
	// Server steps supersede local pet notices. A hidden/replaced notice must
	// not cause the next NPC line's click to swallow its AC20:6 acknowledgement.
	if c.petAnnouncement {
		c.Talk.Hide()
		c.petAnnouncement = false
	}
	c.event.active = true
	kind, subject := s[eventFrameKind], s[eventFrameSubject]
	actor := binary.LittleEndian.Uint16(s[eventFrameActor:])
	talk := binary.LittleEndian.Uint16(s[eventFrameText:])
	switch kind {
	case eventKindTalk:
		c.faceConversation(subject, actor, s[eventFrameMode] == npcTalkTurn)
		c.event.done = false
		c.say(subject, actor, uint32(talk)|uint32(s[eventFrameText+2])<<16)
	case eventKindQuestion:
		c.faceConversation(subject, actor, true)
		c.ask(subject, actor, talk, s[eventFrameMode])
	case eventKindMovie:
		c.startMovie(s)
	case eventKindNPCTurn:
		c.event.done = true
		c.turnNPC(uint16(subject), s[eventFrameMode], int(binary.LittleEndian.Uint32(s[eventFrameValue:])))
	case eventKindTurn:
		c.event.done = true
		c.turnPlayer(subject, actor)
	case eventKindMusic:
		c.event.done = true
		c.eventTrack(subject, binary.LittleEndian.Uint32(s[eventFrameValue:]))
	case eventKindRoute:
		// Only map NPCs (subject 3) walk; the value is the route slot.
		c.event.done = true
		if subject == eventSpeakerNPC {
			c.World.WalkRoute(actor, byte(binary.LittleEndian.Uint32(s[eventFrameValue:])), s[eventFrameMode], c.Now())
		}
	default:
		c.event.done = true
	}
}

// turnNPC is kind 11 (case 0xb): the NPC whose click ID is the subject
// byte turns toward the player, another NPC, or a direction. The NPC's
// +0x1efc check and FUN_0030a344's release are not ported.
func (c *Client) turnNPC(click uint16, mode byte, value int) {
	n := c.World.NPCs[click]
	if n == nil {
		return
	}
	var x, y int
	switch mode {
	case npcTurnPlayer:
		x, y = c.World.Player.X, c.World.Player.Y
	case npcTurnNPC:
		o := c.World.NPCs[uint16(value)]
		if o == nil {
			return
		}
		x, y = o.X, o.Y
	case npcTurnDirection:
		var ok bool
		if x, y, ok = world.DirectionPoint(n.X, n.Y, value); !ok {
			return
		}
	default:
		return
	}
	n.Action = world.TurnAction(n.Action, n.X, n.Y, x, y)
}

// turnPlayer is kind 14 (case 0xe): the player turns toward a map NPC or
// a direction (the actor field).
func (c *Client) turnPlayer(subject byte, actor uint16) {
	p := &c.World.Player
	var x, y int
	switch subject {
	case turnTowardNPC:
		n := c.World.NPCs[actor]
		if n == nil {
			return
		}
		x, y = n.X, n.Y
	case turnDirection:
		var ok bool
		if x, y, ok = world.DirectionPoint(p.X, p.Y, int(actor)); !ok {
			return
		}
	default:
		return
	}
	p.Direction = int32(world.TurnAction(int(p.Direction), p.X, p.Y, x, y))
}

// eventTrack is kind 15 (case 0xf): subject 1 makes BGM plus the value
// (at least four digits) the event's track (+0x732c), which the music
// function (FUN_00307eb8) prefers to the map's; subject 2 clears it.
func (c *Client) eventTrack(subject byte, value uint32) {
	switch subject {
	case musicPlay:
		c.eventMusic = fmt.Sprintf("BGM%04d", value)
	case musicRestore:
		c.eventMusic = ""
	default:
		return
	}
	c.playMapMusic()
}

// say is kind 1: the line talk spoken by an NPC or the player.
func (c *Client) say(subject byte, actor uint16, talk uint32) {
	c.loadTalks()
	// FUN_00304fd0 passes the UInt16 at +0x711c to both the text and voice
	// lookups. The following event byte can be a flag, not part of the ID.
	// Retain extended exported references only when no native ID exists.
	nativeTalk := uint32(uint16(talk))
	if nativeTalk != 0 {
		if _, ok := c.talks[nativeTalk]; ok {
			talk = nativeTalk
		}
	}
	text := clientassets.Big5Text(c.talks[talk])
	c.Talk.Say(text, c.speaker(subject, actor), c.World.Player.Name)
	if subject == eventSpeakerSelf {
		c.playDialogueVoice(talk, movie.PlayerTemplate)
	} else if n := c.World.NPCs[actor]; subject == eventSpeakerNPC && n != nil {
		c.playDialogueVoice(talk, n.Template)
	}
}

// loadTalks reads Talk.dat once.
func (c *Client) loadTalks() {
	if c.talks == nil {
		var err error
		if c.resources.talks == nil {
			c.resources.talks, err = world.Talks(c.Assets)
		}
		c.talks = c.resources.talks
		if err != nil {
			log.Printf("talk: %v", err)
			c.talks = map[uint32]string{}
		}
	}
}

// speaker is FUN_0034c814's subject: a map NPC (3) or the player (7).
func (c *Client) speaker(subject byte, actor uint16) hud.Speaker {
	var who hud.Speaker
	switch subject {
	case eventSpeakerNPC:
		if n := c.World.NPCs[actor]; n != nil {
			who = npcSpeaker(n)
		}
	case eventSpeakerSelf:
		who = hud.Speaker{Name: c.World.Player.Name, Left: true}
		if f, ok := c.World.Body.(interface {
			DrawFace(*surface.Surface, int, int, int, bool)
			FaceWidth(int) int
			BodyWidth(int) int
		}); ok {
			// The player speaks in the wide form with the large art
			// (+0x2a8); the original also checks the worn look against
			// a table (PTR_DAT_004ca774), which is not ported. Without
			// the art the face-mode portrait stands at the left.
			if f.FaceWidth(role.TalkArtAction) > 0 {
				// FUN_0034d990 places it by the body sprite's width in
				// the same action (FUN_00437cd4).
				who.ArtW = f.BodyWidth(role.TalkArtAction)
				who.Art = func(dst *surface.Surface, x, y int, blinking bool) {
					f.DrawFace(dst, x, y, role.TalkArtAction, blinking)
				}
			} else {
				who.Face = func(dst *surface.Surface, x, y int, blinking bool) {
					f.DrawFace(dst, x, y, role.PortraitAction, blinking)
				}
			}
		}
	}
	return who
}

// npcSpeaker is FUN_0034c814 for a map NPC: face mode with its face
// sprite, body mode (standing facing down-left, action 0xb) without one.
func npcSpeaker(n *world.NPC) hud.Speaker {
	who := hud.Speaker{Name: clientassets.Big5Text(n.Info.Name), NameLow: n.Info.HeightPreset == 1, BodyLow: n.Info.TalkLow == 1}
	f, ok := n.Painter.(*role.NPC)
	if !ok {
		return who
	}
	if id := n.Info.Face; id != 0 {
		who.Face = func(dst *surface.Surface, x, y int, blinking bool) { f.DrawFace(dst, x, y, id, blinking) }
		return who
	}
	who.BodyW, who.BodyH = f.FrameSize(talkBodyAction)
	// The body goes through the map's sprite placement (FUN_002fe8e8), so
	// tall-name templates such as Burke the tiger drop as they do there.
	drop := n.Info.SpriteDrop()
	who.Body = func(dst *surface.Surface, x, y int) { f.Draw(dst, x, y+drop, talkBodyAction) }
	return who
}

// Question replies (20/9): an option row r sends 30 + r ("3" + the row,
// FUN_00307ca4), Yes 20 and No 21 (FUN_00307c4c, FUN_00307c78), Close 40.
const (
	answerOption = 30
	answerYes    = 20
	answerNo     = 21
	yesNoEntries = 2
)

// ask is kind 6 (FUN_00304fd0 case 6): the question's prompt and options
// in the talk window, spoken by an NPC or the player. Mode 0 marks the
// step done as well, so the answer is followed by 20/6; mode 1 sends only
// the answer. Item-picker questions (+0x42 1 or 3) open the native shop form;
// unsupported non-item pickers are closed explicitly.
func (c *Client) ask(subject byte, actor, id uint16, mode byte) {
	q, ok := c.World.Questions[id]
	if ok && q.Mode != 0 && c.shopQuestion(q) {
		return
	}
	if !ok || q.Mode != 0 {
		log.Printf("event: question %d (form %d) not ported, closing", id, q.Mode)
		c.Net.Send([]byte{protocol.CommandEvent, protocol.EventChoice, eventChoiceCancel})
		return
	}
	var prompt, options [][]byte
	var answers []byte
	yesNo := 0
	for _, e := range q.Entries {
		text := c.questionText(e)
		switch e.Type {
		case world.QuestionPrompt:
			prompt = append(prompt, text)
		case world.QuestionYesNo:
			yesNo++
		case world.QuestionOption:
			answers = append(answers, answerOption+byte(len(options)))
			options = append(options, text)
		}
	}
	buttons := len(options) == 0 && yesNo == yesNoEntries
	if buttons {
		// FUN_0034d150: the OK and Cancel buttons answer Yes and No.
		options, answers = [][]byte{nil, nil}, []byte{answerYes, answerNo}
	}
	c.event.done = false
	c.event.answers, c.event.ackAnswer = answers, mode == 0
	c.Talk.Ask(prompt, c.speaker(subject, actor), c.World.Player.Name, options, buttons)
}

// questionText is an entry's text: a talk line, or an item's name.
func (c *Client) questionText(e world.QuestionEntry) []byte {
	switch e.Kind {
	case world.QuestionText:
		c.loadTalks()
		return clientassets.Big5Text(c.talks[uint32(e.Value)])
	case world.QuestionItem:
		if it, ok := c.items[e.Value]; ok {
			return clientassets.Big5Text(it.Definition.Name)
		}
	}
	return nil
}

// answer sends a question's reply and closes the window.
func (c *Client) answer(v byte) {
	c.Talk.Hide()
	c.event.answers = nil
	c.Net.Send([]byte{protocol.CommandEvent, protocol.EventChoice, v})
	if v != eventChoiceCancel && c.event.ackAnswer {
		// Send the question's ACK with its answer, before a new server frame
		// can replace the step and erase or misattribute the pending ACK.
		c.event.done = false
		c.Net.Send([]byte{protocol.CommandEvent, protocol.EventAcknowledge})
	}
}

// pickAnswer is a click while a question shows.
func (c *Client) pickAnswer(x, y int) {
	row, cancel := c.Talk.Pick(x, y)
	switch {
	case cancel:
		c.answer(eventChoiceCancel)
	case row >= 0 && row < len(c.event.answers):
		c.answer(c.event.answers[row])
	}
}

// eventClose is 20/7: the step ends and is acknowledged.
func (c *Client) eventClose() { c.event.done = true }

// Receive cases 10, 0xb and 0xd..0x11 of 20 (0x2e3dc0) all set +0x7108
// and clear +0x7109; only 10 is named by the server. The others keep their
// numbers until their senders are traced.
const (
	eventStepDoneWireCode11 = 11
	eventStepDoneWireCode13 = 13
	eventStepDoneWireCode17 = 17
)

// stepDone reports a 20 subcommand that finishes the step.
func stepDone(sub byte) bool {
	return sub == protocol.EventStepComplete || sub == eventStepDoneWireCode11 ||
		(sub >= eventStepDoneWireCode13 && sub <= eventStepDoneWireCode17)
}

// eventResume is 20/8 (receive case 8): the event is over and the server
// no longer holds the player (+0x2392 = 0), so a door event's 6/2 hold
// ends with the teleport's closing 20/8.
func (c *Client) eventResume() {
	c.stopConstellation()
	for npc, action := range c.event.npcFacings {
		npc.Action = action
		npc.Conversation = false
	}
	c.event = eventState{}
	c.held = false
	c.Talk.Hide()
	c.petAnnouncement = false
}

// advanceTalk is a click while a line shows: the step is done.
func (c *Client) advanceTalk() {
	c.Talk.Hide()
	if c.petAnnouncement {
		c.petAnnouncement = false
		return
	}
	c.event.done = true
}

// eventTick is FUN_00307300's acknowledgement.
func (c *Client) eventTick() {
	// Native +0x732a holds AC20:6 until the entire constellation sequence ends.
	if c.constellation != nil {
		return
	}
	if c.minigameResultAckPending {
		c.minigameResultAckPending = false
		c.Net.Send([]byte{protocol.CommandEvent, protocol.EventAcknowledge})
	}
	if !c.event.active && !c.held && c.movie == nil && !c.Talk.Shown() && len(c.petAnnouncements) > 0 {
		c.World.StopWalk()
		c.Talk.Say(c.petAnnouncements[0], hud.Speaker{}, c.World.Player.Name)
		c.petAnnouncements = c.petAnnouncements[1:]
		c.petAnnouncement = true
	}
	// FUN_00307300 holds a finished step while route walkers are queued.
	if !c.clinicUntil.IsZero() {
		return
	}
	if c.event.done && c.World != nil && !c.World.RoutesDone() {
		return
	}
	if c.event.done {
		c.event.done = false
		c.Net.Send([]byte{protocol.CommandEvent, protocol.EventAcknowledge})
	}
}

// FUN_00304fd0 saves turning actors for FUN_00307bc4 to restore at event end.
// Talk mode 1 permits turning; mode 2 preserves the NPC's facing. Question
// prompts turn eligible NPCs regardless of their answer-acknowledgment mode.
func (c *Client) faceConversation(subject byte, actor uint16, turn bool) {
	if c.World == nil || subject != eventSpeakerNPC {
		return
	}
	n := c.World.NPCs[actor]
	if n == nil {
		return
	}
	p := &c.World.Player
	if c.event.npcFacings == nil {
		c.event.npcFacings = map[*world.NPC]int{}
	}
	n.BeginConversation(c.Now())
	if _, saved := c.event.npcFacings[n]; !saved {
		c.event.npcFacings[n] = n.Action
	}
	if turn {
		n.FaceConversation(p.X, p.Y)
	}
	// The player faces the speaker even when the speaker is exempt from turning.
	c.World.StopWalk()
	p.Direction = int32(world.Facing(p.X, p.Y, n.X, n.Y, int(p.Direction)%conversationFacingCount) + conversationStandingAction)
}
