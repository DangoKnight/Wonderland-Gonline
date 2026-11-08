package wire

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/store"
)

func TestMessageBoardRawPackets(t *testing.T) {
	// 1899-12-30 (OLE epoch) makes the timestamp eight independent zero bytes.
	post := store.MessageBoardPost{ID: 7, AuthorID: 9, AuthorName: []byte("A"), Title: []byte("T"), Content: []byte("Hi"), PostedAtMillis: -2209161600000}
	list, err := MessageBoardEntries([]store.MessageBoardPost{post})
	// Legacy reference capture confirms author ID precedes message ID.
	want := []byte{82, 1, 1, 0, 0, 0, 'A', 1, 0, 0, 0, 'T', 0, 0, 0, 0, 0, 0, 0, 0, 9, 0, 0, 0, 7, 0, 0, 0}
	if err != nil || !bytes.Equal(list, want) {
		t.Fatal(list, err)
	}
	detail, err := MessageBoardContent(post)
	want = []byte{82, 2, 1, 0, 0, 0, 'A', 1, 0, 0, 0, 'T', 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 9, 0, 0, 0, 'H', 'i'}
	if err != nil || !bytes.Equal(detail, want) {
		t.Fatal(detail, err)
	}
	post.Title = bytes.Repeat([]byte{'x'}, 31)
	if _, err := MessageBoardEntries([]store.MessageBoardPost{post}); err == nil {
		t.Fatal("oversized title accepted")
	}
}
