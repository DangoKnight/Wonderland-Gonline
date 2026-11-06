package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
)

func TestNPCServicesCapturedGoldAndItemReceipts(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.Stats.Gold = 83
	for _, p := range [][]byte{{26, 2, 10, 0, 0, 0}, {26, 2, 60, 0, 0, 0}} {
		c.dispatch(p)
	}
	if c.Stats.Gold != 13 {
		t.Fatal("deductions treated as balances", c.Stats.Gold)
	}
	c.dispatch([]byte{26, 1, 68, 0, 0, 0})
	if c.Stats.Gold != 81 {
		t.Fatal("gain not added")
	}
	c.dispatch([]byte{26, 4, 25, 0, 0, 0})
	if c.Stats.Gold != 25 {
		t.Fatal("balance not replaced")
	}
}
func TestNPCServicesShopQuestionAndClinic(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.Stats.Level = 5
	c.Stats.HP, c.Stats.MaxHP = 185, 196
	c.Stats.SP, c.Stats.MaxSP = 100, 106
	capture := wire(t, c)
	c.World.Questions[6] = world.Question{Mode: 1, Entries: []world.QuestionEntry{{Slot: 5, Value: 420, Kind: world.QuestionItem, Type: world.QuestionOption}}}
	c.ask(3, 23, 6, 0)
	if !c.Services.Visible || c.Services.Mode != inventory.ServiceBuy || len(c.Services.Stock) != 1 || len(capture()) != 0 {
		t.Fatal("item question cancelled")
	}
	c.dispatch([]byte{31, 2, 0, 0, 0, 0})
	if c.clinicPrompt == nil || c.UI.Modal != c.clinicPrompt {
		t.Fatal("missing treatment confirmation")
	}
	c.dispatch([]byte{31, 10})
	if c.clinicUntil.Sub(*now) != 5*time.Second {
		t.Fatal("native rest duration")
	}
	if dir := os.Getenv("NPC_SERVICES_SNAPSHOT_DIR"); dir != "" {
		*now = now.Add(200 * time.Millisecond)
		c.Frame()
		save(t, c, filepath.Join(dir, "clinic.png"))
	}
	c.dispatch([]byte{31, 1, 0})
	if c.clinicPrompt != nil {
		t.Fatal("confirmation not closed")
	}
}

func TestNPCServicesKelanShopFromExtractedMap(t *testing.T) {
	c, _, _ := enteredClient(t)
	m, err := world.MapRecord(c.Assets, 12000)
	if err != nil {
		t.Fatal(err)
	}
	c.World.Questions = world.MapQuestions(m)
	c.Stats.Gold = 83
	capture := wire(t, c)
	c.ask(3, 23, 6, 0)
	if !c.Services.Visible || c.Services.Mode != inventory.ServiceBuy || len(c.Services.Stock) != 5 {
		t.Fatalf("native shop not opened: %+v", c.World.Questions[6])
	}
	for i, id := range []uint16{32011, 32002, 45001, 45007, 42008} {
		if c.Services.Stock[i].ID != id {
			t.Fatalf("stock %d: %+v", i, c.Services.Stock[i])
		}
	}
	c.Services.Select(0, 4)
	c.Services.Select(0, 2)
	c.Services.Select(0, 2)
	if c.Services.Total() != 70 {
		t.Fatalf("price: %d", c.Services.Total())
	}
	c.Services.Confirm()
	packets := capture()
	if len(packets) == 0 || !bytes.Equal(packets[0], []byte{27, 1, 2, 5, 1, 3, 2}) {
		t.Fatalf("cart: %v", packets)
	}
	c.dispatch([]byte{27, 1, 0})
	if c.Services.Visible {
		t.Fatal("successful purchase did not close shop")
	}
	if c.World.Questions[5].Mode != world.QuestionDialogueMode || c.World.Questions[8].Mode != world.QuestionEquipmentMode {
		t.Fatal("native dialogue/equipment modes")
	}
}
func TestNPCServicesWindowsAndReset(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.Stats.Level = 5
	c.Stats.MaxHP, c.Stats.HP = 196, 196
	c.Stats.MaxSP, c.Stats.SP = 106, 106
	c.Stats.Gold = 83
	for i, id := range []uint16{32011, 32002, 45001, 45007, 42008} {
		c.InventoryState.Bag[i] = game.Item{ID: id, Count: byte(i + 1)}
	}
	c.InventoryState.Pets[1] = inventory.UsePet{ID: 17004, Name: []byte("Fragrance"), Stats: world.Stats{Element: 1, Level: 5, HP: 185, MaxHP: 185, SP: 50, MaxSP: 50}, Amity: 64}
	f := c.Services
	for _, mode := range []inventory.ServiceMode{inventory.ServiceBuy, inventory.ServiceSell, inventory.ServiceStorage, inventory.ServiceHotel} {
		f.SaleMode = 1
		f.Stock = []inventory.ShopEntry{{Slot: 1, ID: 32011, Price: 10}, {Slot: 2, ID: 32002, Price: 50}, {Slot: 3, ID: 45001, Price: 30}, {Slot: 4, ID: 45007, Price: 30}, {Slot: 5, ID: 42008, Price: 10}}
		f.Open(mode)
		if f.Image < 0 {
			t.Fatalf("missing native service art mode %d", mode)
		}
		if mode == inventory.ServiceBuy {
			f.Select(0, 2)
			f.Select(0, 3)
			f.Select(0, 3)
		} else {
			f.Select(1, 1)
		}
		c.Frame()
		c.Frame()
		if dir := os.Getenv("NPC_SERVICES_SNAPSHOT_DIR"); dir != "" {
			save(t, c, filepath.Join(dir, map[inventory.ServiceMode]string{inventory.ServiceBuy: "buy.png", inventory.ServiceSell: "sell.png", inventory.ServiceStorage: "storage.png", inventory.ServiceHotel: "hotel.png"}[mode]))
		}
	}
	f.HotelPacket([]byte{31, 3, 1, 2})
	f.Storage[0] = game.Item{ID: 42008, Count: 1}
	c.resetInventory(c.World.Player)
	f.HotelPacket([]byte{31, 5, 1, 2})
	if f.Visible || !f.Storage[0].Empty() || c.InventoryState.Pets[1].ID != 0 {
		t.Fatal("service leaked across character entry")
	}
}

func TestNPCServicesHotelStatsBeforeCopy(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.Services.Open(inventory.ServiceHotel)
	c.Services.Hotel[0] = inventory.UsePet{ID: 17004, Name: []byte("Fragrance")}
	c.dispatch([]byte{8, 2, 4, 2, 0, 25, 1, 185, 0, 0, 0, 0, 0, 0, 0})
	c.dispatch([]byte{31, 5, 1, 2})
	c.announceNewPets([game.MaxPets]inventory.UsePet{})
	if len(c.petAnnouncements) != 0 {
		t.Fatal("hotel withdrawal queued recruitment dialogue")
	}
	if c.InventoryState.Pets[1].ID != 17004 || c.InventoryState.Pets[1].Stats.HP != 185 {
		t.Fatal("lost pre-copy vitals", c.InventoryState.Pets[1])
	}
}

func TestNPCServicesHotelPackedSelectionAndHover(t *testing.T) {
	c, now, _ := enteredClient(t)
	f := c.Services
	pet := inventory.UsePet{ID: 17004, Name: []byte("Fragrance"), Stats: world.Stats{Element: 1, Level: 5, HP: 185, MaxHP: 185, SP: 50, MaxSP: 50}, Amity: 64}
	f.Hotel[4] = pet
	c.InventoryState.Pets[2] = inventory.UsePet{ID: 18001, Name: []byte("Roca"), Stats: world.Stats{Element: 2, Level: 5}}
	f.Open(inventory.ServiceHotel)
	*now = now.Add(300 * time.Millisecond)
	// Screenshot's first visible storage row, despite underlying slot five.
	c.Input.X, c.Input.Y = f.Left+70, f.Top+75
	c.Frame()
	if f.PreviewPet.ID != pet.ID {
		t.Fatal("hovered stored pet has no preview")
	}
	c.UI.MouseDown(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
	c.UI.MouseUp(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
	if !f.Selected[0][4] || f.Selected[0][0] {
		t.Fatal("display row used as storage slot")
	}
	c.Input.X, c.Input.Y = 0, 0
	c.Frame()
	if f.PreviewPet.ID != 0 || !f.Selected[0][4] {
		t.Fatal("preview and selection are not independent")
	}
	if dir := os.Getenv("NPC_SERVICES_SNAPSHOT_DIR"); dir != "" {
		save(t, c, filepath.Join(dir, "hotel-selected-stored.png"))
	}
	c.Input.X, c.Input.Y = f.Left+300, f.Top+75
	c.Frame()
	c.UI.MouseDown(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
	c.UI.MouseUp(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
	if !f.Selected[1][2] {
		t.Fatal("packed party row did not select original slot")
	}
}

func TestNPCServicesNativeSleepAnimation(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.dispatch([]byte{31, 10})
	start := *now
	previousRed := 0
	for i, elapsed := range []time.Duration{0, 300 * time.Millisecond, 600 * time.Millisecond, 900 * time.Millisecond} {
		*now = start.Add(elapsed)
		c.Frame()
		if c.clinicSleepGlyph == nil || c.clinicSleepGlyph.W < 40 || c.clinicSleepGlyph.H < 40 {
			t.Fatal("missing native uppercase letter artwork")
		}
		// Isolate the Z region: unrelated world sprites may also animate.
		frame := c.Screen.RGBA()
		red := 0
		for y := 130; y < 250; y++ {
			for x := 400; x < 485; x++ {
				color := frame.RGBAAt(x, y)
				if color.R > 240 && color.G < 80 && color.B < 80 {
					red++
				}
			}
		}
		if i < 3 && red <= previousRed || i == 3 && red != 0 {
			t.Fatalf("sleep phase %d has %d red pixels after %d", i, red, previousRed)
		}
		previousRed = red
		if dir := os.Getenv("NPC_SERVICES_SNAPSHOT_DIR"); dir != "" {
			save(t, c, filepath.Join(dir, fmt.Sprintf("clinic-phase-%d.png", i)))
		}
	}
}
