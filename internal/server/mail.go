package server

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"time"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

const (
	textMailDeliveryBatchSize  = 32
	oleAutomationUnixEpochDays = 25569
	millisecondsPerDay         = 24 * 60 * 60 * 1000
)

// textMailPacket follows AC14.Recv1: sender, OLE Automation date and a
// byte-length-prefixed native string. UTC timestamps make replay independent
// of the server's timezone. Input text is raw Big5-compatible bytes, not UTF-8.
func textMailPacket(message store.TextMail) ([]byte, error) {
	date := float64(message.SentAtMillis)/millisecondsPerDay + oleAutomationUnixEpochDays
	return protocol.Builder{protocol.CommandFriends, protocol.FriendsTextMail}.U32(message.SenderID).F64(date).String(string(message.Content))
}

// textMailCommand runs under worldMu. The reference ignores the type byte;
// retain it for provenance without assigning unverified attachment semantics.
func (s *Server) textMailCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < protocol.TextMailRequestHeaderBytes {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	kind, recipient := r.U8(), r.U32()
	content := bytes.TrimRight(r.Rest(), "\x00") // Native accepts trailing NUL terminators.
	if r.Err() != nil || store.ValidateMailContent(content) != nil {
		return protocol.ErrMalformed
	}
	message, err := s.Store.SendTextMail(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, recipient, kind, content, time.Now())
	if errors.Is(err, sql.ErrNoRows) {
		return c.send(headBanner("That character does not exist."))
	}
	if err != nil {
		return err
	}
	// Success means accepted durably, even if the recipient is offline.
	if err := c.send(protocol.Builder{protocol.CommandFriends, protocol.FriendsMailSent}.U32(recipient).U8(protocol.FriendsMailSentSuccess)); err != nil {
		return err
	}
	if target := s.friendPlayer(message.ReceiverID); target != nil && target.ready {
		if err := s.deliverPendingTextMail(ctx, target); err != nil {
			s.Log.Warn("mail delivery deferred", "recipient", message.ReceiverID, "error", err)
			target.conn.Close()
		}
	}
	return nil
}

// Delivery is at least once: the native protocol has no receipt acknowledgment.
// A crash between a successful write and the durable marker can repeat a message.
func (s *Server) deliverPendingTextMail(ctx context.Context, c *Session) error {
	for {
		messages, err := s.Store.PendingTextMail(ctx, c.character.ID, textMailDeliveryBatchSize)
		if err != nil {
			return err
		}
		if len(messages) == 0 {
			return nil
		}
		for _, message := range messages {
			packet, err := textMailPacket(message)
			if err != nil {
				return err
			}
			if err := c.send(packet); err != nil {
				return err
			}
			if err := s.Store.MarkTextMailDelivered(ctx, c.character.ID, message.ID); err != nil {
				return err
			}
		}
	}
}
