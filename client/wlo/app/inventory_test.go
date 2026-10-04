package app

import (
	"bytes"
	"image"
	"os"
	"testing"
	"wonderland-go/client/wlo/login"
	"wonderland-go/client/wlo/surface"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

func TestInventoryFlow(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	c.MainButtons.Buttons[mainInventoryButton].OnClick()
	if !c.Inventory.Visible {
		t.Fatal("toolbar did not open inventory")
	}
	// Native initial bag record, then an additive grant for the same item.
	p := []byte{23, 5, 1, 0x39, 0x1b, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	c.dispatch(p)
	if c.InventoryState.Bag[0].Count != 2 {
		t.Fatal("bag snapshot")
	}
	c.Inventory.Use(1, false)
	got := sent()
	if len(got) != 1 || !bytes.Equal(got[0], []byte{23, 96, 1}) {
		t.Fatal("use", got)
	}
	if c.InventoryState.Bag[0].Count != 2 {
		t.Fatal("request changed bag")
	}
	c.dispatch([]byte{23, 9, 1, 1})
	if c.InventoryState.Bag[0].Count != 1 {
		t.Fatal("reply did not remove item")
	}
	c.dispatch(p)
	if c.InventoryState.Bag[0].Count != 3 {
		t.Fatal("grant was not additive")
	}
	c.Inventory.SetMode(2)
	right := c.Inventory.Right()
	c.Inventory.SetMode(0)
	if c.Inventory.Right() != right {
		t.Fatal("native mode anchor")
	}
	c.Stats.Level = 1
	c.Stats.Element = 3
	c.Stats.HP = 181
	c.Stats.MaxHP = 181
	c.Stats.SP = 100
	c.Stats.MaxSP = 100
	c.Stats.INT = 6
	c.Stats.WIS = 1
	c.Stats.AGI = 1
	c.InventoryState.Bag[0] = game.Item{ID: 32011, Count: 5}
	c.inventoryPacket([]byte{23, 5})
	c.Frame()
	if out := os.Getenv("INVENTORY_SNAPSHOT"); out != "" {
		savePNG(t, out, c)
	}
	if !c.InventoryKey(0x1b) || c.Inventory.Visible {
		t.Fatal("Escape")
	}
	c.dispatch(selfPacket(10002, 2, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "Other"))
	if c.InventoryState.Bag != (game.Inventory{}) {
		t.Fatal("character switch retained bag")
	}
}

func TestInventoryDragAndEquipment(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	var sent [][]byte
	c.Inventory.Send = func(p []byte) { sent = append(sent, append([]byte(nil), p...)) }
	c.Inventory.Show()
	c.InventoryState.Bag[0] = game.Item{ID: 32011, Count: 5}
	src, dst := c.Inventory.Slots[0], c.Inventory.Slots[1]
	a, b := src.Abs(), dst.Abs()
	src.LeftDown(0, a.X+10, a.Y+10)
	src.LeftUpOutside(0, b.X+10, b.Y+10)
	if len(sent) != 1 || !bytes.Equal(sent[0], []byte{23, 10, 1, 1, 2}) {
		t.Fatal("native single-item drag", sent)
	}
	if c.InventoryState.Bag[0].Count != 5 || !c.InventoryState.Bag[1].Empty() {
		t.Fatal("optimistic move")
	}
	c.dispatch([]byte{23, 10, 1, 1, 2})
	src.LeftDown(4, a.X+10, a.Y+10)
	src.LeftUpOutside(4, b.X+10, b.Y+10)
	if c.Inventory.Dialog == nil || !c.Inventory.Dialog.Visible || len(sent) != 1 {
		t.Fatal("split should require quantity")
	}
	c.Frame()
	if out := os.Getenv("INVENTORY_SNAPSHOT"); out != "" {
		savePNG(t, out+".quantity.png", c)
	}
	c.Inventory.Dialog.Count.SetText([]byte("2"))
	c.Inventory.Dialog.Count.OnEnter()
	if len(sent) != 2 || !bytes.Equal(sent[1], []byte{23, 10, 1, 2, 2}) {
		t.Fatal("split", sent)
	}
	// Gear reply swaps existing worn equipment back to the source bag cell.
	c.items[100] = assets.NativeItem{Definition: game.ItemDefinition{EquipSlot: 1}}
	c.InventoryState.Bag[0] = game.Item{ID: 100, Count: 1}
	gear := c.Inventory.Worn[0].Abs()
	src.LeftDown(0, a.X+10, a.Y+10)
	src.LeftUpOutside(0, gear.X+10, gear.Y+10)
	if !bytes.Equal(sent[len(sent)-1], []byte{23, 11, 1}) {
		t.Fatal("equip request", sent)
	}
	if !c.InventoryState.Equipment[0].Empty() {
		t.Fatal("optimistic equipment")
	}
	c.dispatch([]byte{23, 17, 1, 1})
	if c.InventoryState.Equipment[0].ID != 100 || len(c.World.Player.Items) != 1 || c.World.Player.Items[0] != 100 {
		t.Fatal("equipment reply")
	}
	c.Inventory.Use(1, true)
	if !bytes.Equal(sent[len(sent)-1], []byte{23, 12, 1, 1}) {
		t.Fatal("unequip request", sent)
	}
	c.dispatch([]byte{23, 16, 1, 1})
	c.dispatch([]byte{23, 212, 255, 1, 100, 0, 1})
	before := len(sent)
	if !c.Inventory.Dialog.Visible || len(sent) != before {
		t.Fatal("destroy was not confirmed")
	}
	c.Inventory.Dialog.Count.OnEnter()
	if len(sent) != before+1 || !bytes.Equal(sent[len(sent)-1], []byte{23, 124, 1, 1, 0}) {
		t.Fatal("destroy confirmation", sent)
	}
	c.Inventory.Use(1, false)
	before = len(sent)
	c.mapReady = false
	c.Inventory.Use(1, false)
	if len(sent) != before {
		t.Fatal("loading allowed item use")
	}
}

func TestInventoryDisconnectClosesQuantity(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.Inventory.Show()
	c.items[100] = assets.NativeItem{Definition: game.ItemDefinition{EquipSlot: 1}}
	c.InventoryState.Bag[0] = game.Item{ID: 100, Count: 1}
	c.dispatch([]byte{23, 212, 255, 1, 100, 0, 1})
	if c.UI.Modal == nil {
		t.Fatal("no confirmation")
	}
	c.disconnected()
	if c.Inventory.Visible || c.UI.Modal != nil || c.Inventory.Dialog.Visible {
		t.Fatal("inventory modal blocked reconnect")
	}
}

func TestInventoryQuantityDoesNotBlockMinigame(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.Inventory.Show()
	c.items[100] = assets.NativeItem{Definition: game.ItemDefinition{EquipSlot: 1}}
	c.InventoryState.Bag[0] = game.Item{ID: 100, Count: 1}
	c.dispatch([]byte{23, 212, 255, 1, 100, 0, 1})
	c.dispatch(moleStart)
	if c.Inventory.Dialog.Visible || c.UI.Modal != nil || c.Inventory.Visible {
		t.Fatal("inventory confirmation blocks game")
	}
	c.dispatch([]byte{57, 2})
	if !c.Inventory.Visible || c.Inventory.Dialog.Visible {
		t.Fatal("stale confirmation restored with HUD")
	}
}

// Captures the preview action without changing world role animation.
type inventoryPreviewRecorder struct {
	login.RoleView
	action int32
	x, y   int
}

func (r *inventoryPreviewRecorder) DrawBody(_ *surface.Surface, x, y int, action int32) {
	r.action, r.x, r.y = action, x, y
}

func TestInventoryNativePresentation(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.Inventory.Show()
	f := c.Inventory
	// Same native display values and attributes as Inventory_Original_01.png.
	c.Stats.Level = 1
	c.Stats.Element = 3
	c.Stats.STR = 5
	c.Stats.INT = 1
	c.Stats.WIS = 1
	c.Stats.AGI = 1
	for _, p := range [][]byte{
		{8, 1, 41, 0, 10, 0, 0, 0}, {8, 1, 42, 0, 3, 0, 0, 0}, {8, 1, 43, 0, 2, 0, 0, 0},
		{8, 1, 44, 0, 2, 0, 0, 0}, {8, 1, 45, 0, 1, 0, 0, 0},
	} {
		c.dispatch(p)
	}
	if got := c.Stats.CombatValues(); got != ([5]uint16{10, 3, 2, 2, 1}) {
		t.Fatal("native display values", got)
	}
	recorder := &inventoryPreviewRecorder{}
	f.Preview = recorder
	c.Screen.Fill(image.Rect(0, 0, 800, 600), 0)
	c.Pics.Draw(c.Screen, f.Image, f.Left, f.Top, true)
	background := append([]uint16(nil), c.Screen.Pix...)
	f.Paint()
	if recorder.action != 17 || recorder.x != f.Left+100 || recorder.y != f.Top+211 {
		t.Fatal("preview pose/position", recorder)
	}
	// A transparent label changes glyph pixels only: it must not paint white
	// rectangles over the blue striped numeric fields.
	changed := 0
	for _, at := range []image.Point{{44, 337}, {130, 337}} {
		for y := at.Y; y < at.Y+15; y++ {
			for x := at.X; x < at.X+16; x++ {
				i := (f.Top+y)*c.Screen.W + f.Left + x
				if c.Screen.Pix[i] != background[i] {
					changed++
					if c.Screen.Pix[i] == 0xffff {
						t.Fatalf("white text background at %d,%d", x, y)
					}
				}
			}
		}
	}
	if changed == 0 {
		t.Fatal("numbers not drawn")
	}
	// Bag counts include quantity one, unlike worn equipment. With no icon
	// loaded, this isolates the glyph from white pixels in item artwork.
	c.InventoryState.Bag[0] = game.Item{ID: 100, Count: 1}
	beforeCount := append([]uint16(nil), c.Screen.Pix...)
	f.Slots[0].Paint()
	countPixels := 0
	for y := 93; y < 108; y++ {
		for x := 193; x < 201; x++ {
			i := (f.Top+y)*c.Screen.W + f.Left + x
			if c.Screen.Pix[i] != beforeCount[i] {
				countPixels++
				if c.Screen.Pix[i] == 0xffff {
					t.Fatal("opaque bag count")
				}
			}
		}
	}
	if countPixels == 0 {
		t.Fatal("quantity one missing")
	}
	// The element widget is explicitly positioned at (19,56) by the native
	// constructor. Compare its nontransparent pixels against the cached icon.
	icon := c.Pics.Entries[c.Pics.Find("Icon_Element_3_1")].Image
	for y := 0; y < icon.H; y++ {
		for x := 0; x < icon.W; x++ {
			want := icon.Pix[y*icon.W+x]
			if want == icon.Key {
				continue
			}
			got := c.Screen.Pix[(f.Top+56+y)*c.Screen.W+f.Left+19+x]
			if got != want {
				t.Fatalf("element placement at icon pixel %d,%d", x, y)
			}
		}
	}
	// The native arrows select pet views; they must not rotate into idle or
	// attack animation groups while those views are unavailable.
	if f.RotateLeft.Enabled || f.RotateRight.Enabled {
		t.Fatal("unported pet selectors enabled")
	}
}

func TestInventoryOriginalSampleSnapshot(t *testing.T) {
	out := os.Getenv("INVENTORY_REFERENCE_SNAPSHOT")
	if out == "" {
		t.Skip("set INVENTORY_REFERENCE_SNAPSHOT")
	}
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.Inventory.Show()
	c.Stats.Level = 1
	c.Stats.Element = 3
	c.Stats.STR = 5
	c.Stats.INT = 1
	c.Stats.WIS = 1
	c.Stats.AGI = 1
	c.Stats.HP = 181
	c.Stats.MaxHP = 181
	c.Stats.SP = 100
	c.Stats.MaxSP = 100
	for i, v := range []uint32{10, 3, 2, 2, 1} {
		c.Stats.Apply(byte(41+i), v)
	}
	// Body/head and authored colour values are from the existing Ship Deck fixture.
	// Use that body's initial orange garment and shoes so battle layering is shown.
	c.InventoryState.Reset([]uint16{21002, 24002})
	c.refreshEquipment()
	c.inventoryPacket([]byte{23, 5})
	c.InventoryState.Bag[0] = game.Item{ID: 32011, Count: 5}
	c.inventoryPacket([]byte{23, 5})
	c.Frame()
	savePNG(t, out, c)
}

// Move quantities follow confirmed source counts and remaining stack space.
func TestInventoryInteractionSamples(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.Inventory.Show()
	f := c.Inventory
	c.InventoryState.Bag[0] = game.Item{ID: 32011, Count: 30}
	src, dst := f.Slots[0], f.Slots[1]
	a, b := src.Abs(), dst.Abs()
	split := func() { src.LeftDown(4, a.X+10, a.Y+10); src.LeftUpOutside(4, b.X+10, b.Y+10) }
	split()
	if string(f.Dialog.Count.Text) != "30" {
		t.Fatal("source quantity", string(f.Dialog.Count.Text))
	}
	if c.Input.Focused == f.Dialog.Count {
		t.Fatal("quantity editor acquired unsolicited focus")
	}
	c.Pics.Draw(c.Screen, f.Dialog.Image, f.Dialog.Left, f.Dialog.Top, true)
	before := append([]uint16(nil), c.Screen.Pix...)
	f.Dialog.Paint()
	for y := 6; y < 24; y++ {
		for x := 32; x < 207; x++ {
			at := (f.Dialog.Top+y)*c.Screen.W + f.Dialog.Left + x
			if c.Screen.Pix[at] != before[at] {
				t.Fatal("native title overwritten")
			}
		}
	}
	if out := os.Getenv("INVENTORY_INTERACTION_SNAPSHOT"); out != "" {
		c.Frame()
		savePNG(t, out+".move.png", c)
	}
	f.Dialog.Hide()
	c.InventoryState.Bag[1] = game.Item{ID: 32011, Count: 45}
	split()
	if !f.Dialog.Visible || string(f.Dialog.Count.Text) != "5" {
		t.Fatal("destination capacity", string(f.Dialog.Count.Text))
	}
	f.Dialog.Hide()
	c.InventoryState.Bag[1].Count = 50
	split()
	if f.Dialog.Visible {
		t.Fatal("full destination opened quantity prompt")
	}
	c.InventoryState.Bag[7] = game.Item{ID: 34253, Count: 5}
	c.Screen.Fill(image.Rect(0, 0, 800, 600), 0)
	c.Frame()
	f.Slots[7].Hint()
	if out := os.Getenv("INVENTORY_INTERACTION_SNAPSHOT"); out != "" {
		savePNG(t, out+".tooltip.png", c)
	}
}
