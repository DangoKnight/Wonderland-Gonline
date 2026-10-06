package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

func nativeMailRequest(recipient uint32, body []byte) []byte {
	return protocol.Builder{14, 1, 0}.U32(recipient).Bytes(body)
}

func TestTextMailNativeGolden(t *testing.T) {
	packet, err := textMailPacket(store.TextMail{SenderID: 0x12345678, SentAtMillis: 0, Content: []byte{0xa4, 0xa4, 0xa4, 0xe5}})
	expected := []byte{14, 1, 0x78, 0x56, 0x34, 0x12, 0, 0, 0, 0, 64, 248, 216, 64, 4, 0xa4, 0xa4, 0xa4, 0xe5}
	if err != nil || !bytes.Equal(packet, expected) {
		t.Fatal(packet, err)
	}
}

func TestTextMailLiveCrossMapAndDurableReceipt(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	ctx := context.Background()
	body := []byte{0xa4, 0xa4, 0xa4, 0xe5, '\n', 'H', 'i'}
	tradeDo(t, s, a, nativeMailRequest(b.character.ID, append(bytes.Clone(body), 0, 0)))
	ack := protocol.Builder{14, 9}.U32(b.character.ID).U8(0)
	if !contains(w[0].packets(t), ack) {
		t.Fatal("sender did not receive native receipt")
	}
	messages := w[2].packets(t)
	if len(messages) != 1 || len(messages[0]) != 15+len(body) || !bytes.Equal(messages[0][15:], body) || binary.LittleEndian.Uint32(messages[0][2:6]) != a.character.ID || messages[0][14] != byte(len(body)) {
		t.Fatal(messages)
	}
	if w[1].Len() != 0 {
		t.Fatal("mail leaked")
	}
	pending, err := s.Store.PendingTextMail(ctx, b.character.ID, 10)
	if err != nil || len(pending) != 0 {
		t.Fatal(pending, err)
	}
	tradeDo(t, s, b, []byte{12, 1})
	if w[2].Len() != 0 {
		t.Fatal("mail replayed on duplicate acknowledgment")
	}
}

func TestTextMailDeferredUntilLoginAndWarpAcknowledgment(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	s.leaveWorld(b)
	w[0].Reset()
	w[2].Reset()
	tradeDo(t, s, a, nativeMailRequest(b.character.ID, []byte("offline")))
	if w[2].Len() != 0 {
		t.Fatal("delivered offline")
	}
	tradeDo(t, s, b, []byte{12, 1})
	first := w[2].packets(t)
	count := 0
	for _, packet := range first {
		if len(packet) >= 15 && packet[0] == 14 && packet[1] == 1 {
			count++
		}
	}
	if count != 1 {
		t.Fatal("offline mail not replayed once", first)
	}
	s.depart(b, protocol.Builder{12}.U32(b.character.ID))
	w[2].Reset()
	tradeDo(t, s, a, nativeMailRequest(b.character.ID, []byte("loading")))
	if w[2].Len() != 0 {
		t.Fatal("delivered into loading snapshot")
	}
	tradeDo(t, s, b, []byte{12, 1})
	second := w[2].packets(t)
	count = 0
	for _, packet := range second {
		if len(packet) >= 15 && packet[0] == 14 && packet[1] == 1 && string(packet[15:]) == "loading" {
			count++
		}
	}
	if count != 1 {
		t.Fatal(second)
	}
}

func TestTextMailFailedRecipientStaysQueued(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	failed := &failedWorldConn{}
	b.conn = failed
	tradeDo(t, s, a, nativeMailRequest(b.character.ID, []byte("retry")))
	if !failed.closed {
		t.Fatal("failed recipient not closed")
	}
	if !contains(w[0].packets(t), protocol.Builder{14, 9}.U32(b.character.ID).U8(0)) {
		t.Fatal("recipient failure withheld accepted sender receipt")
	}
	pending, err := s.Store.PendingTextMail(context.Background(), b.character.ID, 10)
	if err != nil || len(pending) != 1 {
		t.Fatal(pending, err)
	}
	s.leaveWorld(b)
	b.conn = w[2]
	w[2].Reset()
	tradeDo(t, s, b, []byte{12, 1})
	pending, err = s.Store.PendingTextMail(context.Background(), b.character.ID, 10)
	if err != nil || len(pending) != 0 {
		t.Fatal(pending, err)
	}
}

func TestTextMailRejectsMalformedAndUnknownRecipient(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	for _, packet := range [][]byte{{14, 1}, {14, 1, 0, 1, 2, 3}, nativeMailRequest(b.character.ID, nil), nativeMailRequest(b.character.ID, bytes.Repeat([]byte{'a'}, 256)), nativeMailRequest(b.character.ID, []byte("a\x00b"))} {
		if err := s.dispatch(context.Background(), a, packet); !errors.Is(err, protocol.ErrMalformed) {
			t.Fatal(packet, err)
		}
	}
	tradeDo(t, s, a, nativeMailRequest(999999, []byte("unknown")))
	if contains(w[0].packets(t), protocol.Builder{14, 9}.U32(999999).U8(0)) || w[2].Len() != 0 {
		t.Fatal("unknown recipient accepted")
	}
	// Closed storage must produce neither a success acknowledgment nor delivery.
	s.Store.Close()
	if err := s.dispatch(context.Background(), a, nativeMailRequest(b.character.ID, []byte("closed"))); err == nil {
		t.Fatal("storage failure ignored")
	}
	if w[0].Len() != 0 || w[2].Len() != 0 {
		t.Fatal("failed persistence published success")
	}
}

func TestTextMailDeliveryBatchesKeepOrder(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	ctx := context.Background()
	for i := 0; i < 35; i++ {
		if _, err := s.Store.SendTextMail(ctx, store.CharacterRef{Account: a.account.ID, ID: a.character.ID}, b.character.ID, 0, []byte{byte('A' + i)}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	s.worldMu.Lock()
	err := s.deliverPendingTextMail(ctx, b)
	s.worldMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	packets := w[2].packets(t)
	if len(packets) != 35 {
		t.Fatal(len(packets))
	}
	for i, packet := range packets {
		if len(packet) != 16 || packet[15] != byte('A'+i) {
			t.Fatal(i, packet)
		}
	}
}

func TestTextMailSenderWriteFailureKeepsAcceptedMessage(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	a.conn = &failedWorldConn{}
	if err := s.dispatch(context.Background(), a, nativeMailRequest(b.character.ID, []byte("accepted"))); err == nil {
		t.Fatal("sender write failure ignored")
	}
	pending, err := s.Store.PendingTextMail(context.Background(), b.character.ID, 10)
	if err != nil || len(pending) != 1 || string(pending[0].Content) != "accepted" {
		t.Fatal(pending, err)
	}
	if w[2].Len() != 0 {
		t.Fatal("sender failure overtook receipt")
	}
}

func TestTextMailRespectsSenderWorldGates(t *testing.T) {
	for _, gate := range []string{"loading", "battle", "minigame"} {
		t.Run(gate, func(t *testing.T) {
			s, p, w := friendFixture(t)
			a, b := p[0], p[2]
			switch gate {
			case "loading":
				a.ready = false
			case "battle":
				a.battle = &battleRun{}
			case "minigame":
				a.event = &eventSession{onMinigame: func(byte) error { t.Fatal("mail consumed event"); return nil }}
			}
			err := s.dispatch(context.Background(), a, nativeMailRequest(b.character.ID, []byte("blocked")))
			if gate == "loading" && !errors.Is(err, protocol.ErrMalformed) {
				t.Fatal(err)
			}
			if gate != "loading" && err != nil {
				t.Fatal(err)
			}
			pending, err := s.Store.PendingTextMail(context.Background(), b.character.ID, 10)
			if err != nil || len(pending) != 0 || w[0].Len() != 0 || w[2].Len() != 0 {
				t.Fatal("mail bypassed interaction gate", pending, err)
			}
		})
	}
}
