package wire

import (
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

// MessageBoardEntries follows FUN_001ff95c. String lengths are u32 (not the
// ordinary PackString byte prefix); the timestamp is Delphi TDateTime.
func MessageBoardEntries(posts []store.MessageBoardPost) ([]byte, error) {
	if len(posts) > protocol.MessageBoardPageSize {
		return nil, protocol.ErrMalformed
	}
	p := protocol.Builder{protocol.CommandMessageBoard, protocol.MessageBoardEntries}
	for _, post := range posts {
		if post.ID == 0 || len(post.AuthorName) < 1 || len(post.AuthorName) > protocol.MessageBoardAuthorMaxBytes || len(post.Title) < 1 || len(post.Title) > protocol.MessageBoardTitleMaxBytes {
			return nil, protocol.ErrMalformed
		}
		date := float64(post.PostedAtMillis)/millisecondsPerDay + oleAutomationUnixEpochDays
		p = p.U32(uint32(len(post.AuthorName))).Bytes(post.AuthorName).U32(uint32(len(post.Title))).Bytes(post.Title).F64(date).U32(post.AuthorID).U32(post.ID)
	}
	return p, nil
}

// FUN_001ffd84 reads the title, date, content length, author ID and content.
// Unlike the list row, detail has no post ID between its date and content.
func MessageBoardContent(post store.MessageBoardPost) ([]byte, error) {
	if _, err := MessageBoardEntries([]store.MessageBoardPost{post}); err != nil {
		return nil, err
	}
	if len(post.Content) < 1 || len(post.Content) > messageBoardContentMaxBytes {
		return nil, protocol.ErrMalformed
	}
	date := float64(post.PostedAtMillis)/millisecondsPerDay + oleAutomationUnixEpochDays
	return protocol.Builder{protocol.CommandMessageBoard, protocol.MessageBoardPage}.U32(uint32(len(post.AuthorName))).Bytes(post.AuthorName).U32(uint32(len(post.Title))).Bytes(post.Title).F64(date).U32(uint32(len(post.Content))).U32(post.AuthorID).Bytes(post.Content), nil
}

const messageBoardContentMaxBytes = 2048 // Preserve administrator records; new posts have a separate limit.
