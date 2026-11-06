package inventory

import (
	"bytes"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestServiceOpeningIgnoresSelectionClicks(t *testing.T) {
	for _, mode := range []ServiceMode{ServiceBuy, ServiceSell, ServiceStorage, ServiceHotel} {
		f, _ := serviceFixture()
		env := &seui.Env{Pics: picdb.New()}
		seui.NewManager(env, &seui.Input{})
		f.InitFixedForm(f, env)
		env.UI.Add(f)
		f.Inventory.Env = env
		now := time.Unix(100, 0)
		f.Inventory.Now = func() time.Time { return now }
		f.Stock = []ShopEntry{{Slot: 1, ID: 42008, Price: 10}}
		f.Storage[0] = game.Item{ID: 42008, Count: 1}
		f.Inventory.State.Bag[0] = f.Storage[0]
		f.Hotel[0] = UsePet{ID: 17004}
		f.SaleMode = 1
		f.Open(mode)
		r := f.rows[0]
		at := r.Abs()
		x, y := at.X+1, at.Y+1
		for _, delay := range []time.Duration{0, serviceOpeningInputDelay - time.Millisecond} {
			now = time.Unix(100, 0).Add(delay)
			r.LeftUp(0, x, y)
			r.LeftUp(controlSplit, x, y)
			if f.Cart[0] != 0 || f.Selected[0][0] || f.Inventory.Dialog != nil {
				t.Fatalf("mode %d accepted opening click", mode)
			}
		}
		now = time.Unix(100, 0).Add(serviceOpeningInputDelay)
		r.LeftUp(0, x, y)
		if mode == ServiceBuy && f.Cart[0] != 1 || mode != ServiceBuy && !f.Selected[0][0] {
			t.Fatalf("mode %d rejected click after delay", mode)
		}
		f.Open(mode)
		if f.inputReady() {
			t.Fatalf("mode %d did not restart delay", mode)
		}
	}
}

func TestServiceBuyControlQuantity(t *testing.T) {
	f, sent := serviceFixture()
	env := &seui.Env{Pics: picdb.New()}
	seui.NewManager(env, &seui.Input{})
	f.Inventory.Env = env
	f.InitFixedForm(f, env)
	f.Visible, f.Mode = true, ServiceBuy
	f.Stock = []ShopEntry{{Slot: 5, ID: 42008, Price: 10}}
	env.UI.Modal = f
	if !f.buyQuantity(0) {
		t.Fatal("stackable item did not open quantity")
	}
	d := f.Inventory.Dialog
	if d == nil || env.UI.Modal != d || f.Cart[0] != 0 || len(*sent) != 0 {
		t.Fatal("quantity selection mutated cart or sent request")
	}
	d.Count.SetText([]byte("12"))
	d.confirm()
	if f.Cart[0] != 12 || f.CartSlots[0] != 5 || f.Total() != 120 || env.UI.Modal != f {
		t.Fatal("quantity not applied or shop modal lost")
	}
	f.buyQuantity(0)
	if d.limit != game.MaxItemStack-12 {
		t.Fatal("remaining stack capacity")
	}
	d.Count.SetText([]byte("50"))
	d.confirm()
	if !d.Visible || f.Cart[0] != 12 {
		t.Fatal("accepted excessive quantity")
	}
	d.Hide()
	if f.Cart[0] != 12 || env.UI.Modal != f {
		t.Fatal("cancel changed cart")
	}
	f.Cart[0] = game.MaxItemStack
	f.buyQuantity(0)
	if d.limit != game.MaxItemStack {
		t.Fatal("full stack blocked another quantity")
	}
	d.Count.SetText([]byte("12"))
	d.confirm()
	if f.Cart[1] != 12 || f.CartSlots[1] != 5 || f.Total() != 620 {
		t.Fatal("quantity did not create second stack")
	}
	f.buyQuantity(0)
	f.Hide()
	if d.Visible || env.UI.Modal != nil {
		t.Fatal("orphaned quantity dialog")
	}
	f.Visible = true
	n := f.Inventory.State.Items[42008]
	n.Definition.Type = 1
	f.Inventory.State.Items[42008] = n
	if f.buyQuantity(0) || d.Visible {
		t.Fatal("non-stackable item opened quantity")
	}
}

func serviceFixture() (*ServiceForm, *[][]byte) {
	bag := &Form{State: &State{Items: map[uint16]assets.NativeItem{42008: {Definition: game.ItemDefinition{ID: 42008, Type: 10}}, 45001: {Definition: game.ItemDefinition{ID: 45001, Type: 10}}}}}
	f := &ServiceForm{Inventory: bag}
	var sent [][]byte
	f.Send = func(p []byte) { sent = append(sent, append([]byte(nil), p...)) }
	return f, &sent
}
func TestServiceCartNativeRequestAndAuthority(t *testing.T) {
	f, sent := serviceFixture()
	f.Mode = ServiceBuy
	f.Stock = []ShopEntry{{Slot: 5, ID: 42008, Price: 10}, {Slot: 3, ID: 45001, Price: 30}}
	f.Gold = func() uint32 { return 83 }
	f.Select(0, 0)
	f.Select(0, 1)
	f.Select(0, 1)
	f.Confirm()
	if !bytes.Equal((*sent)[0], []byte{27, 1, 2, 5, 1, 3, 2}) || f.Total() != 70 || !f.Waiting || f.Inventory.State.Bag != (game.Inventory{}) {
		t.Fatal("native cart changed resources", *sent, f.Total())
	}
	f.Confirm()
	if len(*sent) != 1 {
		t.Fatal("pending cart resent")
	}
	f.Waiting = false
	f.Gold = func() uint32 { return 0 }
	f.Confirm()
	if len(*sent) != 1 {
		t.Fatal("insufficient funds sent")
	}
}

func TestServiceBuyMultipleStacks(t *testing.T) {
	f, sent := serviceFixture()
	f.Mode = ServiceBuy
	f.Stock = []ShopEntry{{Slot: 5, ID: 42008, Price: 10}}
	for i := 0; i < 125; i++ {
		f.Select(0, 0)
	}
	if f.Cart[0] != 50 || f.Cart[1] != 50 || f.Cart[2] != 25 || f.Total() != 1250 {
		t.Fatal("multiple stacks", f.Cart, f.Total())
	}
	f.Confirm()
	if len(*sent) != 1 || !bytes.Equal((*sent)[0], []byte{27, 1, 3, 5, 50, 5, 50, 5, 25}) {
		t.Fatal("missing repeated stock entries", *sent)
	}
}
func TestServiceStorageCapturedIncrementalSequence(t *testing.T) {
	f, _ := serviceFixture()
	add := make([]byte, 33)
	copy(add, []byte{30, 2, 24, 164, 1})
	if !f.StoragePacket(add) || f.Storage[0].ID != 42008 {
		t.Fatal("storage add")
	}
	if !f.StoragePacket([]byte{30, 5, 1, 1}) || !f.Storage[0].Empty() {
		t.Fatal("storage remove")
	}
	before := f.Storage
	for _, p := range [][]byte{{30, 5, 0, 1}, {30, 2, 1}, {30, 5, 51, 1}} {
		if f.StoragePacket(p) || f.Storage != before {
			t.Fatal("malformed storage published", p)
		}
	}
}
func TestServiceHotelCapturedCopyAndUnlock(t *testing.T) {
	f, sent := serviceFixture()
	f.Mode = ServiceHotel
	f.Inventory.State.Pets[1] = UsePet{ID: 17004, Name: []byte("Fragrance")}
	f.Selected[1][1] = true
	f.Transfer(true)
	if !bytes.Equal((*sent)[0], []byte{31, 3, 2}) || f.Inventory.State.Pets[1].ID == 0 {
		t.Fatal("speculative deposit")
	}
	if !f.HotelPacket([]byte{31, 3, 1, 2}) || f.Hotel[0].ID != 17004 {
		t.Fatal("hotel copy")
	}
	f.Inventory.State.Pets[1] = UsePet{}
	if !f.HotelPacket([]byte{31, 9}) || f.Waiting {
		t.Fatal("deposit unlock")
	}
	if !f.HotelPacket([]byte{31, 5, 1, 2}) || f.Inventory.State.Pets[1].ID != 17004 {
		t.Fatal("withdrawal copy")
	}
	if !f.HotelPacket([]byte{31, 4, 1}) || f.Hotel[0].ID != 0 {
		t.Fatal("hotel removal")
	}
}
func TestServiceMultiSlotSelectionResolvesCoveredCell(t *testing.T) {
	f, _ := serviceFixture()
	f.Mode = ServiceStorage
	n := f.Inventory.State.Items[42008]
	n.Definition.CellWidth, n.Definition.CellHeight = 2, 2
	f.Inventory.State.Items[42008] = n
	f.Inventory.State.Bag[0] = game.Item{ID: 42008, Count: 1}
	f.Select(1, 6)
	if !f.Selected[1][0] || f.Selected[1][6] {
		t.Fatal("covered cell did not select owner")
	}
}
