package app

import (
	"bytes"
	"encoding/binary"
	"image"
	"net"
	"sync"
	"testing"
	"time"

	"wonderland-go/client/wlo/login"
	"wonderland-go/client/wlo/world"
	"wonderland-go/internal/protocol"
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
