package assetsql

import (
	"testing"
	"wonderland-gonline/internal/assets"
)

func itemWithPrice(price uint32) assets.NativeItem {
	var item assets.NativeItem
	item.Record[114] = byte(price)
	item.Record[115] = byte(price >> 8)
	item.Record[116] = byte(price >> 16)
	item.Record[117] = byte(price >> 24)
	return item
}

func TestNativeBuyPricesReadTheDecodedRecord(t *testing.T) {
	prices := nativeBuyPrices(map[uint16]assets.NativeItem{
		10002: itemWithPrice(10),
		10003: itemWithPrice(0),
	})
	if len(prices) != 1 || prices[10002] != 10 {
		t.Fatalf("prices %v", prices)
	}
}
