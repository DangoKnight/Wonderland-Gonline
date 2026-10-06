package app

import (
	"encoding/binary"
	"log"

	"wonderland-gonline/client/wlo/hud"
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
// until clicked. Every other kind is acknowledged at once, as kind 2 is;
// questions (kind 6, the map's question table at +0x5d9c) are cancelled
// with 20/9 choice 40.
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
	eventKindQuestion = 6
	eventKindMovie    = 5
	eventSpeakerNPC   = 3
	eventSpeakerSelf  = 7
	// eventChoiceCancel closes a question (the server's cancel value).
	eventChoiceCancel = 40
	// talkBodyAction is the body-mode pose: standing, facing down-left.
	talkBodyAction = 0xb
	// npcReach is FUN_00303b54's distance check, per axis.
	npcReach = 0xa9
)

// eventState is the interpreter's step flags.
type eventState struct {
	active bool // a step is in progress (+0x70e8)
	done   bool // the step is finished and awaits 20/6 (+0x7108)
	// A question awaiting its answer (+0x710a): the reply value of each
	// option row, and whether the step also ends with 20/6 (mode 0).
	answers   []byte
	ackAnswer bool
}

// clickNPC is a left click on a map NPC: within reach it sends 20/1,
// otherwise the player walks toward it first.
func (c *Client) clickNPC(n *world.NPC) {
	p := c.World.Player
	if abs(p.X-n.X) <= npcReach && abs(p.Y-n.Y) <= npcReach {
		c.World.StopWalk()
		c.sendNPCClick(n.ClickID)
		return
	}
	if c.World.WalkTo(n.X, n.Y, c.Now()) {
		c.pendingNPC = n
	}
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
	c.event.active = true
	kind, subject := s[eventFrameKind], s[eventFrameSubject]
	actor := binary.LittleEndian.Uint16(s[eventFrameActor:])
	talk := binary.LittleEndian.Uint16(s[eventFrameText:])
	switch kind {
	case eventKindTalk:
		c.event.done = false
		c.say(subject, actor, talk)
	case eventKindQuestion:
		c.ask(subject, actor, talk, s[eventFrameMode])
	case eventKindMovie:
		c.startMovie(s)
	default:
		c.event.done = true
	}
}

// say is kind 1: the line talk spoken by an NPC or the player.
func (c *Client) say(subject byte, actor, talk uint16) {
	c.loadTalks()
	text := clientassets.Big5Text(c.talks[talk])
	c.Talk.Say(text, c.speaker(subject, actor), c.World.Player.Name)
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
			c.talks = map[uint16]string{}
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
// the answer. Questions for other forms (+0x42 1 or 3: the item and
// trade pickers) are not ported and are closed.
func (c *Client) ask(subject byte, actor, id uint16, mode byte) {
	q, ok := c.World.Questions[id]
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
		return clientassets.Big5Text(c.talks[e.Value])
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
		c.event.done = true
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
	c.event = eventState{}
	c.held = false
	c.Talk.Hide()
}

// advanceTalk is a click while a line shows: the step is done.
func (c *Client) advanceTalk() {
	c.Talk.Hide()
	c.event.done = true
}

// eventTick is FUN_00307300's acknowledgement.
func (c *Client) eventTick() {
	if c.event.done {
		c.event.done = false
		c.Net.Send([]byte{protocol.CommandEvent, protocol.EventAcknowledge})
	}
}
