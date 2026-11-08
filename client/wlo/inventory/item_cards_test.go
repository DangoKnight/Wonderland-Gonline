package inventory

import (
	"bytes"
	"testing"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestItemCardSelectionAndCancellation(t *testing.T) {
	f, sent := useFixture(50008)
	f.Env = &seui.Env{Pics: picdb.New()}
	seui.NewManager(f.Env, &seui.Input{})
	f.State.Items[50008] = assets.NativeItem{Definition: game.ItemDefinition{Type: game.ItemTypeRedemptionCard, EquipSlot: 8}}
	f.Selected = 1
	f.State.Pets[0] = UsePet{ID: 14156}
	before := f.State.Bag
	f.useBag(1)
	if len(*sent) != 1 || !bytes.Equal((*sent)[0], []byte{23, 133, 1, 1, 88, 195, 0}) {
		t.Fatal("must use the card, not the selected pet", *sent)
	}
	card := protocol.GiftCard{ID: 50008, Version: 1, Rewards: []protocol.GiftCardReward{{ItemID: 25639, Quantity: 1}, {ItemID: 25640, Quantity: 1}}}
	if !f.CardPacket(protocol.GiftCardDetailsPacket([]protocol.GiftCard{card})) || !f.CardDialog.Visible || f.Env.UI.Modal != f.CardDialog {
		t.Fatal("missing choice dialog")
	}
	f.CardDialog.selected = 1
	f.CardDialog.exchange()
	if !bytes.Equal((*sent)[1], []byte{23, 133, 2, 1, 1, 2, 40, 100}) || f.State.Bag != before {
		t.Fatal("choice changed local inventory", *sent)
	}
	f.CardDialog.Hide()
	if f.Env.UI.Modal != nil || f.State.Bag != before {
		t.Fatal("cancel consumed card or retained modal")
	}
	f.ResetUse()
	if f.CardPacket([]byte{23, 108, 2, 0, 0, 99}) {
		t.Fatal("mistook stall/malformed data for a card")
	}
}

func TestItemCardCatalogRequestMatchesLegacy(t *testing.T) {
	f, sent := useFixture(50008)
	if !f.CardPacket([]byte{23, 108, 1, 2, 0, 81, 195, 1, 88, 195, 1}) {
		t.Fatal("native card index not handled")
	}
	if len(*sent) != 1 || !bytes.Equal((*sent)[0], []byte{90, 1, 1, 2, 0, 81, 195, 88, 195}) {
		t.Fatal("catalog mode/count/IDs differ from Legacy", *sent)
	}
}
