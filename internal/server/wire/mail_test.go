package wire

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/store"
)

func TestTextMailNativeGolden(t *testing.T) {
	packet, err := TextMailPacket(store.TextMail{SenderID: 0x12345678, SentAtMillis: 0, Content: []byte{0xa4, 0xa4, 0xa4, 0xe5}})
	expected := []byte{14, 1, 0x78, 0x56, 0x34, 0x12, 0, 0, 0, 0, 64, 248, 216, 64, 0xa4, 0xa4, 0xa4, 0xe5}
	if err != nil || !bytes.Equal(packet, expected) {
		t.Fatal(packet, err)
	}
}
