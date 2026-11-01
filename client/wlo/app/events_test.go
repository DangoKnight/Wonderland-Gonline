package app

import (
	"bytes"
	"encoding/binary"
	"image"
	"net"
	"sync"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

// wire connects the client to a pipe and returns what it has sent so far.
func wire(t *testing.T, c *Client) func() [][]byte {
	t.Helper()
	near, far := net.Pipe()
	t.Cleanup(func() { near.Close(); far.Close() })
	c.Net.Dial = func(string, string) (net.Conn, error) { return near, nil }
	var mu sync.Mutex
	var got [][]byte
	go func() {
		for {
			p, err := protocol.Read(far)
			if err != nil {
				return
			}
			mu.Lock()
			got = append(got, p)
			mu.Unlock()
		}
	}()
	c.Net.Connect("127.0.0.1")
	for deadline := time.Now().Add(2 * time.Second); ; {
		connected := false
		for _, e := range c.Net.Poll() {
			connected = connected || e.Kind == login.Connected
		}
		if connected {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not connected")
		}
		time.Sleep(time.Millisecond)
	}
	return func() [][]byte {
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		return append([][]byte(nil), got...)
	}
}

// talkFrame is the server's kind-1 frame: an NPC (subject 3) says talk.
func talkFrame(actor, talk uint16) []byte {
	p := []byte{protocol.CommandEvent, 1, 0, 0, 0, 1, eventKindTalk, eventSpeakerNPC}
	p = binary.LittleEndian.AppendUint16(p, actor)
	p = append(p, 1, 0, 0, 0, 0)
	p = binary.LittleEndian.AppendUint16(p, talk)
	return append(p, 0)
}

// nearNPC is a shown NPC within reach of the player and its screen box.
func nearNPC(t *testing.T, c *Client) (*world.NPC, image.Rectangle) {
	cx, cy := c.World.Camera()
	p := c.World.Player
	for _, n := range c.World.NPCs {
		if !n.Shown || n.Painter == nil || abs(n.X-p.X) > npcReach || abs(n.Y-p.Y) > npcReach {
			continue
		}
		if r := n.Painter.Bounds(n.X-cx, n.Y-cy, n.Action); !r.Empty() && c.World.NPCAt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2) == n {
			return n, r
		}
	}
	t.Fatal("no NPC within reach")
	return nil, image.Rectangle{}
}

// TestNPCTalk: clicking an NPC sends 20/1 with its click ID; a talk frame
// shows the line in the talk window; a click closes it and the frame loop
// acknowledges with 20/6; 20/8 ends the event and clicks walk again.
func TestNPCTalk(t *testing.T) {
	c, _, legs := enteredClient(t)
	sent := wire(t, c)
	n, r := nearNPC(t, c)
	mid := r.Min.Add(image.Pt(r.Dx()/2, r.Dy()/2))
	c.GroundClick(mid.X, mid.Y)
	click := binary.LittleEndian.AppendUint16([]byte{protocol.CommandEvent, protocol.EventActorClick}, n.ClickID)
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], click) {
		t.Fatalf("sent %x, want %x", got, click)
	}
	if len(*legs) != 0 {
		t.Fatal("clicking an NPC in reach walked")
	}
	c.dispatch(talkFrame(n.ClickID, 10002))
	if !c.Talk.Shown() || len(c.Talk.Lines()) != 1 || string(c.Talk.Lines()[0]) != "No." {
		t.Fatalf("talk window %v %q", c.Talk.Shown(), c.Talk.Lines())
	}
	c.GroundClick(10, 300) // closes the line; does not walk
	c.Frame()
	if c.Talk.Shown() || len(*legs) != 0 {
		t.Fatal("click did not close the line, or walked")
	}
	if got := sent(); len(got) != 2 || !bytes.Equal(got[1], []byte{protocol.CommandEvent, protocol.EventAcknowledge}) {
		t.Fatalf("sent %x, want 20/6 after the click", got)
	}
	c.GroundClick(10, 300) // still in the event
	if len(*legs) != 0 {
		t.Fatal("walked during the event")
	}
	c.dispatch([]byte{protocol.CommandEvent, protocol.EventResume})
	c.GroundClick(300, 350)
	if len(*legs) == 0 {
		t.Fatal("no walk after the event")
	}
}

// TestTalkWrap: lines wrap at word boundaries within 40 characters, as
// the reference's "When the sea wind is blowing, it feels great." does.
func TestTalkWrap(t *testing.T) {
	c, _, _ := enteredClient(t)
	n, _ := nearNPC(t, c)
	c.dispatch(talkFrame(n.ClickID, 30126))
	got := c.Talk.Lines()
	if len(got) != 2 || string(got[0]) != "When the sea wind is blowing, it feels " || string(got[1]) != "great." {
		t.Fatalf("lines %q", got)
	}
}

// TestCatTalk: the Persian Cat (deck click 8, template 11000) has no face
// sprite, so the window shows its body; its "#swav1541/#sMeow~" plays the
// effect and shows "Meow~".
func TestCatTalk(t *testing.T) {
	c, _, _ := enteredClient(t)
	var played []string
	c.Env.Sound = func(p string) { played = append(played, p) }
	cat := c.World.NPCs[8]
	if cat == nil || cat.Template != 11000 || cat.Info.Face != 0 {
		t.Fatalf("deck click 8 is %+v", cat)
	}
	c.dispatch(talkFrame(8, 30137))
	if got := c.Talk.Lines(); len(got) != 1 || string(got[0]) != "Meow~" {
		t.Fatalf("lines %q", got)
	}
	if len(played) != 1 || played[0] != `sound\wav1541.wav` {
		t.Fatalf("played %q", played)
	}
	if who := npcSpeaker(cat); who.Body == nil || who.Face != nil || who.BodyW == 0 || who.BodyH == 0 {
		t.Fatalf("speaker %+v", who)
	}
}

// questionFrame is the server's kind-6 frame: question id asked by NPC
// actor, mode 0 (answered, then 20/6).
func questionFrame(actor, id uint16) []byte {
	p := []byte{protocol.CommandEvent, 1, 0, 0, 0, 1, eventKindQuestion, eventSpeakerNPC}
	p = binary.LittleEndian.AppendUint16(p, actor)
	p = append(p, 0, 0, 0, 0, 0)
	p = binary.LittleEndian.AppendUint16(p, id)
	return append(p, 0)
}

// TestQuestion: map 10005's question 1 offers five options; clicking the
// third sends 20/9 with 32, then the step ends with 20/6. A ground click
// while choosing does nothing.
func TestQuestion(t *testing.T) {
	c, now, legs := enteredClient(t)
	sent := wire(t, c)
	rec, err := world.MapRecord(c.Assets, 10005)
	if err != nil {
		t.Fatal(err)
	}
	c.World.Questions = world.MapQuestions(rec)
	n, _ := nearNPC(t, c)
	c.dispatch(questionFrame(n.ClickID, 1))
	if !c.Talk.Choosing() || len(c.Talk.Options()) != 5 {
		t.Fatalf("options %q", c.Talk.Options())
	}
	for i := 0; i < 6; i++ {
		*now = now.Add(16 * time.Millisecond)
		c.Frame()
	}
	c.GroundClick(10, 590)
	if !c.Talk.Choosing() || len(*legs) != 0 || len(sent()) != 0 {
		t.Fatal("a click outside the options answered or walked")
	}
	p := c.Talk.OptionPoint(2)
	c.GroundClick(p.X, p.Y)
	c.Frame()
	got := sent()
	if len(got) != 2 || !bytes.Equal(got[0], []byte{protocol.CommandEvent, protocol.EventChoice, 32}) ||
		!bytes.Equal(got[1], []byte{protocol.CommandEvent, protocol.EventAcknowledge}) {
		t.Fatalf("sent %x", got)
	}
	if c.Talk.Shown() {
		t.Fatal("the question is still open")
	}
}

// TestStepCompleteAcknowledged: 20/10 (and its twins) finish the step, so
// the next frame answers 20/6 at once, as the beach rescue's server steps
// expect.
func TestStepCompleteAcknowledged(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	ack := []byte{protocol.CommandEvent, protocol.EventAcknowledge}
	for i, sub := range []byte{protocol.EventStepComplete, eventStepDoneWireCode11, eventStepDoneWireCode13, eventStepDoneWireCode17} {
		c.dispatch([]byte{protocol.CommandEvent, sub})
		c.Frame()
		if got := sent(); len(got) != i+1 || !bytes.Equal(got[i], ack) {
			t.Fatalf("after 20/%d sent %x", sub, got)
		}
	}
	c.dispatch([]byte{protocol.CommandEvent, 12})
	c.Frame()
	if got := sent(); len(got) != 4 {
		t.Fatalf("20/12 was acknowledged: %x", got)
	}
}

// TestRouteEvent: a kind-4 frame walks the NPC along its route, and the
// frame loop sends one 20/6 only after it arrives.
func TestRouteEvent(t *testing.T) {
	c, now, _ := enteredClient(t)
	sent := wire(t, c)
	n := &world.NPC{ClickID: 40, X: 1000, Y: 1000, Action: 8, Shown: true, Routes: map[byte][]image.Point{3: {{1100, 1000}}}}
	c.World.NPCs[40] = n
	p := []byte{protocol.CommandEvent, 1, 0, 0, 0, 1, eventKindRoute, eventSpeakerNPC}
	p = binary.LittleEndian.AppendUint16(p, 40)
	p = append(p, world.RouteWalk, 3, 0, 0, 0, 0, 0, 0)
	c.dispatch(p)
	c.eventTick()
	if got := sent(); len(got) != 0 {
		t.Fatalf("acknowledged before arrival: %x", got)
	}
	*now = now.Add(time.Second)
	c.World.Step(*now)
	c.eventTick()
	c.eventTick()
	ack := []byte{protocol.CommandEvent, protocol.EventAcknowledge}
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], ack) || n.X != 1100 {
		t.Fatalf("sent %x, NPC at %d", got, n.X)
	}
}

// eventFrameOf builds a frame of any kind (the server's BuildEventFrame).
func eventFrameOf(kind, subject byte, actor uint16, mode byte, value uint32) []byte {
	p := []byte{protocol.CommandEvent, 1, 0, 0, 0, 1, kind, subject}
	p = binary.LittleEndian.AppendUint16(p, actor)
	p = append(p, mode)
	p = binary.LittleEndian.AppendUint32(p, value)
	return append(p, 0, 0, 0)
}

// TestEventTurns: kind 14 turns the player toward an NPC or a direction,
// and kind 11 turns an NPC toward the player, another NPC or a direction;
// each is acknowledged at once.
func TestEventTurns(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	p := &c.World.Player
	a := &world.NPC{ClickID: 41, X: p.X - 100, Y: p.Y, Action: 8, Shown: true}
	b := &world.NPC{ClickID: 42, X: p.X - 100, Y: p.Y + 100, Action: 8, Shown: true}
	c.World.NPCs[41], c.World.NPCs[42] = a, b
	steps := []struct {
		frame []byte
		check func() bool
	}{
		{eventFrameOf(eventKindTurn, turnTowardNPC, 41, 0, 0), func() bool { return p.Direction == 8+world.FaceLeft }},
		{eventFrameOf(eventKindTurn, turnDirection, 5, 0, 0), func() bool { return p.Direction == 8+world.FaceDown }},
		{eventFrameOf(eventKindNPCTurn, 41, 10, npcTurnPlayer, 0), func() bool { return a.Action == 8+world.FaceRight }},
		{eventFrameOf(eventKindNPCTurn, 41, 10, npcTurnNPC, 42), func() bool { return a.Action == 8+world.FaceDown }},
		{eventFrameOf(eventKindNPCTurn, 41, 10, npcTurnDirection, 1), func() bool { return a.Action == 8+world.FaceUp }},
	}
	for i, s := range steps {
		c.dispatch(s.frame)
		c.eventTick()
		if !s.check() {
			t.Errorf("step %d: player %d, NPC %d", i, p.Direction, a.Action)
		}
	}
	if got := sent(); len(got) != len(steps) {
		t.Fatalf("%d acknowledgements, want %d", len(got), len(steps))
	}
}

// TestEventMusic: kind 15 subject 1 sets the event's track, which the map
// music prefers, and subject 2 clears it.
func TestEventMusic(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.dispatch(eventFrameOf(eventKindMusic, musicPlay, 0, 0, 18))
	if c.eventMusic != "BGM0018" {
		t.Fatalf("track %q", c.eventMusic)
	}
	c.dispatch(eventFrameOf(eventKindMusic, musicRestore, 0, 0, 0))
	if c.eventMusic != "" {
		t.Fatalf("track %q after restore", c.eventMusic)
	}
}
