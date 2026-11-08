package server

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"time"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
)

// Public AC82 notice board. SQL owns posts and enforces authorship and quota.
func (s *Server) messageBoardCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	switch p[1] {
	case protocol.MessageBoardPost:
		r := protocol.NewReader(p[2:])
		titleLen := r.U32()
		if titleLen > protocol.MessageBoardTitleMaxBytes {
			return protocol.ErrMalformed
		}
		title := r.Bytes(int(titleLen))
		contentLen := r.U32()
		if contentLen > protocol.MessageBoardContentMaxBytes {
			return protocol.ErrMalformed
		}
		content := r.Bytes(int(contentLen))
		if r.Remaining() == 1 && r.U8() != protocol.MessageBoardWireContext1 {
			return protocol.ErrMalformed
		}
		if r.Err() != nil || r.Remaining() != 0 {
			return protocol.ErrMalformed
		}
		_, err := s.Store.CreateMessageBoardPost(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, title, content, time.Now())
		if errors.Is(err, store.ErrMessageBoardAuthorLimit) {
			return c.send([]byte{protocol.CommandMessageBoard, protocol.MessageBoardAuthorLimit})
		}
		if errors.Is(err, store.ErrMessageBoardFull) {
			return c.send([]byte{protocol.CommandMessageBoard, protocol.MessageBoardFull})
		}
		if errors.Is(err, store.ErrMessageBoardText) {
			return protocol.ErrMalformed
		}
		if err != nil {
			return err
		}
		if err := c.send([]byte{protocol.CommandMessageBoard, protocol.MessageBoardPosted, protocol.MessageBoardWireContext1}); err != nil {
			return err
		}
		return s.messageBoardCommand(ctx, c, []byte{protocol.CommandMessageBoard, protocol.MessageBoardPage, 1})
	case protocol.MessageBoardDelete:
		if !messageBoardEntryRequest(p) {
			return protocol.ErrMalformed
		}
		err := s.Store.DeleteMessageBoardPost(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, binary.LittleEndian.Uint32(p[2:]))
		if errors.Is(err, sql.ErrNoRows) {
			return c.send([]byte{protocol.CommandMessageBoard, protocol.MessageBoardMissing})
		}
		if err != nil {
			return err
		}
		return s.messageBoardCommand(ctx, c, []byte{protocol.CommandMessageBoard, protocol.MessageBoardPage, 1})
	case protocol.MessageBoardPage:
		// Two/three-byte requests are retained for older Gonline clients.
		// Native Notice serializes page followed by its board context byte.
		if len(p) != 2 && len(p) != 3 && len(p) != protocol.MessageBoardNativePageBytes {
			return protocol.ErrMalformed
		}
		page := byte(1)
		if len(p) >= 3 {
			page = p[2]
		}
		if len(p) == protocol.MessageBoardNativePageBytes && p[3] != protocol.MessageBoardWireContext1 {
			return protocol.ErrMalformed
		}
		if page == 0 {
			return protocol.ErrMalformed
		}
		posts, count, err := s.Store.MessageBoardPage(ctx, (int(page)-1)*protocol.MessageBoardPageSize, protocol.MessageBoardPageSize)
		if err != nil {
			return err
		}
		last := max(1, int((count+protocol.MessageBoardPageSize-1)/protocol.MessageBoardPageSize))
		if int(page) > last {
			return s.messageBoardCommand(ctx, c, []byte{protocol.CommandMessageBoard, protocol.MessageBoardPage, byte(min(last, 255))})
		}
		entries, err := serverwire.MessageBoardEntries(posts)
		if err != nil {
			return err
		}
		return s.sendAll(c, [][]byte{protocol.Builder{protocol.CommandMessageBoard, protocol.MessageBoardPagination}.U32(uint32(count)).U8(page), entries})
	case protocol.MessageBoardRead:
		if !messageBoardEntryRequest(p) {
			return protocol.ErrMalformed
		}
		post, err := s.Store.MessageBoardEntry(ctx, binary.LittleEndian.Uint32(p[2:]))
		if errors.Is(err, sql.ErrNoRows) {
			return c.send([]byte{protocol.CommandMessageBoard, messageBoardNotFound})
		}
		if err != nil {
			return err
		}
		packet, err := serverwire.MessageBoardContent(post)
		if err != nil {
			return err
		}
		return c.send(packet)
	default:
		return ErrUnsupported
	}
}

func messageBoardEntryRequest(p []byte) bool {
	// The ID is authoritative; native page/date are presentation metadata.
	// Keep the earlier ID-only Gonline layout compatible.
	return len(p) == messageBoardReadRequestBytes ||
		(len(p) == protocol.MessageBoardNativeEntryRequestBytes && p[len(p)-1] == protocol.MessageBoardWireContext1)
}

const (
	messageBoardReadRequestBytes = 6
	messageBoardNotFound         = 7 // Native AC82:7: "Info not found. Try again later".
)
