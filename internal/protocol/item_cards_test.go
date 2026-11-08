package protocol

import (
	"bytes"
	"reflect"
	"testing"
)

func TestGiftCardNativeCatalogLayout(t *testing.T) {
	// Authored independently from TGiftExchangeCardForm's Gec.Dat reader.
	raw := []byte{23, 108, 2, 1, 0, 24, 195, 1, 2, 39, 100, 1, 40, 100, 3}
	want := []GiftCard{{ID: 49944, Version: 1, Rewards: []GiftCardReward{{25639, 1}, {25640, 3}}}}
	got, ok := ParseGiftCardDetails(raw)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatal(got, ok)
	}
	if !bytes.Equal(GiftCardDetailsPacket(want), raw) {
		t.Fatal("wire changed")
	}
	stall := append([]byte(nil), raw...)
	stall[1] = 133 // Native vehicle-stall replies are not card catalogs.
	if _, ok := ParseGiftCardDetails(stall); ok {
		t.Fatal("accepted vehicle-stall reply as card metadata")
	}
	for i := 0; i < len(raw); i++ {
		if _, ok := ParseGiftCardDetails(raw[:i]); ok {
			t.Fatal("accepted truncated catalog", i)
		}
	}
	if _, ok := ParseGiftCardDetails(append(raw, 0)); ok {
		t.Fatal("accepted trailing bytes")
	}
}
