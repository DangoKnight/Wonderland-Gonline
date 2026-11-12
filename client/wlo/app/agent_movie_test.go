package app

import (
	"bytes"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/internal/protocol"
)

func TestAgentMovieDialogueWaitsForMovieCompletion(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	c.event = eventState{active: true}
	c.movie = &moviePlay{}
	c.Talk.Say([]byte("A cinematic line"), hud.Speaker{}, nil)
	if _, err := c.AgentSubmit([]byte(`{"command":"dialogue.advance"}`)); err != nil {
		t.Fatal(err)
	}
	c.agentTick()
	c.eventTick()
	if c.Talk.Shown() || c.event.done || len(sent()) != 0 {
		t.Fatal("API dismissed a movie line by acknowledging the entire event", sent())
	}
	// A server completion cue must not bypass a running cinematic either.
	c.dispatch([]byte{protocol.CommandEvent, protocol.EventStepComplete})
	c.eventTick()
	if len(sent()) != 0 {
		t.Fatal("completion cue bypassed movie")
	}
	c.endMovie()
	c.eventTick()
	c.eventTick()
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{protocol.CommandEvent, protocol.EventAcknowledge}) {
		t.Fatal("movie did not acknowledge exactly once", got)
	}
	c.dispatch([]byte{protocol.CommandEvent, protocol.EventResume})
	if c.event.active || c.held || c.MoviePlaying() {
		t.Fatal("completed event retained input lock")
	}
}

func TestEventResumeRetiresMovieWithoutLateAcknowledgment(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	c.held = true
	c.event = eventState{active: true, done: true}
	c.movie = &moviePlay{}
	c.Talk.Lowered = true
	c.Talk.Say([]byte("A canceled movie line"), hud.Speaker{}, nil)
	c.dispatch([]byte{protocol.CommandEvent, protocol.EventResume})
	c.eventTick()
	if c.MoviePlaying() || c.Talk.Shown() || c.Talk.Lowered || c.event.active || c.held || len(sent()) != 0 {
		t.Fatal("event resume retained movie state or sent stale acknowledgment", sent())
	}
}

func TestAgentBeachRescueCompletesAndReleasesMovement(t *testing.T) {
	c, now, _ := enteredClient(t)
	sent := wire(t, c)
	c.AgentEnableSessionHost()
	c.dispatch([]byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh})
	c.dispatch([]byte{protocol.CommandMovement, protocol.MovementMovementLock, 1})
	c.dispatch(movieFrameFor(12008, 1))
	if !c.MoviePlaying() {
		t.Fatal("rescue fixture did not load")
	}
	lines := 0
	for frame := 0; frame < 30*30 && c.MoviePlaying(); frame++ {
		if c.Talk.Shown() {
			lines++
			if _, err := c.AgentSubmit([]byte(`{"command":"dialogue.advance"}`)); err != nil {
				t.Fatal(err)
			}
		}
		*now = now.Add(time.Second / 30)
		c.frame(false)
		if c.MoviePlaying() && len(sent()) != 0 {
			t.Fatal("rescue advanced the server before movie completion", sent())
		}
	}
	if c.MoviePlaying() || lines != 12 {
		t.Fatal("agent failed to finish rescue", lines)
	}
	c.frame(false)
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{protocol.CommandEvent, protocol.EventAcknowledge}) {
		t.Fatal("incorrect rescue completion", got)
	}
	c.dispatch([]byte{protocol.CommandEvent, protocol.EventResume})
	c.dispatch([]byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh})
	if !c.AgentWalkWorld(c.World.Player.X+40, c.World.Player.Y) {
		t.Fatal("agent movement remained locked after completed cinematic")
	}
}
