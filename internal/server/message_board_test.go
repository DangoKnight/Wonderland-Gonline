package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

func TestMessageBoardServiceOpensAfterResume(t *testing.T) {
	s, players, wires := worldFixture(t)
	c, w := players[0], wires[0]
	ev := &assets.Event{ClickID: 65000, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{evOp(1, 7, 15, 0, 0, 0)}}}}
	if err := s.startEvent(context.Background(), c, 36, ev, 0, false); err != nil {
		t.Fatal(err)
	}
	resume, opened := -1, -1
	for i, p := range w.packets(t) {
		if bytes.Equal(p, []byte{20, 8}) {
			resume = i
		}
		if bytes.Equal(p, []byte{82, 9}) {
			opened = i
		}
	}
	if opened <= resume || resume < 0 || c.event != nil {
		t.Fatal("board did not open after dialogue resumed", w.packets(t))
	}
	w.Reset()
	// Exact request captured from Legacy's Notice window on 2026-11-07.
	if err := s.messageBoardCommand(context.Background(), c, []byte{82, 2, 1, 1}); err != nil {
		t.Fatal(err)
	}
	got := w.packets(t)
	if len(got) != 2 || !bytes.Equal(got[0], []byte{82, 4, 0, 0, 0, 0, 1}) || !bytes.Equal(got[1], []byte{82, 1}) {
		t.Fatal(got)
	}
	w.Reset()
	if err := s.messageBoardCommand(context.Background(), c, []byte{82, 3, 99, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if got := w.packets(t); len(got) != 1 || !bytes.Equal(got[0], []byte{82, 7}) {
		t.Fatal(got)
	}
	for _, p := range [][]byte{{82}, {82, 2, 0}, {82, 2, 1, 0}, {82, 3}, {82, 3, 1, 0, 0, 0, 0}} {
		if err := s.messageBoardCommand(context.Background(), c, p); err != protocol.ErrMalformed {
			t.Fatal(p, err)
		}
	}
	// Both definitions retain independent legacy numeric values.
	if world.ServiceMessageBoard != 15 || protocol.CommandMessageBoard != 82 {
		t.Fatal("legacy codes changed")
	}
}

func TestMessageBoardPostReadDelete(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	c, w := players[0], wires[0]
	post := []byte{82, 1, 1, 0, 0, 0, 'T', 2, 0, 0, 0, 'H', 'i', 1}
	if err := s.messageBoardCommand(ctx, c, post); err != nil {
		t.Fatal(err)
	}
	got := w.packets(t)
	if len(got) != 3 || !bytes.Equal(got[0], []byte{82, 8, 1}) || !bytes.Equal(got[1], []byte{82, 4, 1, 0, 0, 0, 1}) {
		t.Fatal("native posting acknowledgement/page sequence", got)
	}
	posts, count, err := s.Store.MessageBoardPage(ctx, 0, 10)
	if err != nil || count != 1 || len(posts) != 1 || posts[0].AuthorID != c.character.ID {
		t.Fatal(posts, count, err)
	}
	id := posts[0].ID
	// Reproduce native selection from the actual wire row, rather than bypassing
	// the list with a SQL ID (which previously hid the swapped identifiers).
	row := got[2]
	if binary.LittleEndian.Uint32(row[len(row)-8:]) != c.character.ID || binary.LittleEndian.Uint32(row[len(row)-4:]) != id {
		t.Fatal("list must carry author ID before message ID", row)
	}
	w.Reset()
	request := append([]byte{82, 3}, row[len(row)-4:]...)
	request = append(request, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1)
	if err := s.messageBoardCommand(ctx, c, request); err != nil {
		t.Fatal(err)
	}
	if got := w.packets(t); len(got) != 1 || got[0][1] != 2 {
		t.Fatal("selecting advertised message failed", got)
	}
	if err := s.messageBoardCommand(ctx, c, post); err != nil {
		t.Fatal(err)
	}
	_, count, _ = s.Store.MessageBoardPage(ctx, 0, 10)
	if count != 1 {
		t.Fatal("replayed post duplicated")
	}
	w.Reset()
	if err := s.messageBoardCommand(ctx, c, protocol.Builder{82, 3}.U32(id).U32(1).F64(0).U8(1)); err != nil {
		t.Fatal(err)
	}
	if got := w.packets(t); len(got) != 1 || got[0][0] != 82 || got[0][1] != 2 {
		t.Fatal(got)
	}
	if len(players) < 2 {
		t.Fatal("need a second character")
	}
	if err := s.messageBoardCommand(ctx, players[1], protocol.Builder{82, 4}.U32(id).U32(1).F64(0).U8(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.MessageBoardEntry(ctx, id); err != nil {
		t.Fatal("another character deleted notice", err)
	}
	if err := s.messageBoardCommand(ctx, c, protocol.Builder{82, 4}.U32(id).U32(1).F64(0).U8(1)); err != nil {
		t.Fatal(err)
	}
	if _, count, err := s.Store.MessageBoardPage(ctx, 0, 10); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err := s.messageBoardCommand(ctx, c, post); err != nil {
		t.Fatal(err)
	}
	posts, _, _ = s.Store.MessageBoardPage(ctx, 0, 10)
	if posts[0].ID == id {
		t.Fatal("deleted ID was reused")
	}
	for _, bad := range [][]byte{{82, 1}, {82, 1, 255, 255, 255, 255}, {82, 1, 1, 0, 0, 0, 'T', 153, 0, 0, 0}, {82, 4, 1}} {
		if err := s.messageBoardCommand(ctx, c, bad); err != protocol.ErrMalformed {
			t.Fatal(bad, err)
		}
	}
}
