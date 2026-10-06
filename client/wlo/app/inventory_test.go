package app

import (
	"bytes"
	"encoding/binary"
	"image"
	"os"
	"testing"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
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

// Drive actual input routing; direct OnEnter/OnClick calls alone cannot prove
// the inventory interaction users see works.
func inventoryClick(t *testing.T, c *Client, control seui.Control) {
	t.Helper()
	at := control.Base().Rect()
	x, y := at.Min.X+at.Dx()/2, at.Min.Y+at.Dy()/2
	c.Input.X, c.Input.Y = x, y
	c.Input.Hovered = nil
	c.Frame()
	if c.Input.Hovered != control {
		t.Fatalf("control not reachable: %s (%d,%d), hovered %T", control.Base().Name, x, y, c.Input.Hovered)
	}
	c.UI.MouseDown(seui.ButtonLeft, 0, x, y)
	c.UI.MouseUp(seui.ButtonLeft, 0, x, y)
}

func TestInventorySelectedConsumableAndReplies(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.Inventory.Show()
	sent := wire(t, c)
	c.items[32011] = assets.NativeItem{Definition: game.ItemDefinition{ID: 32011, Name: "Healing food", Status: [2]uint16{25, 26}, Values: [2]int32{400, 400}}}
	c.InventoryState.Bag[0] = game.Item{ID: 32011, Count: 3}
	c.Stats.HP, c.Stats.MaxHP = 50, 181
	if out := os.Getenv("ITEM_USE_SNAPSHOT"); out != "" {
		c.InventoryState.Bag[1] = game.Item{ID: 32176, Count: 5}
		c.inventoryPacket([]byte{23, 5})
		c.Frame()
		c.Inventory.Slots[1].Hint()
		savePNG(t, out+".consumable.png", c)
	}
	pet := game.Pet{ID: 12032, Level: 1, HP: 100, SP: 40, Potential: 10, Base: game.Attributes{Constitution: 4, Wisdom: 3}}
	c.dispatch(pet.ListRecord([]byte{15, 8}, 3, "Robinson", game.PetTemplate{}))
	c.Inventory.Slots[0].DblClick()
	if c.UI.Modal != nil || c.Inventory.UseDialog != nil {
		t.Fatal("ordinary consumable opened a dialog")
	}
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{23, 15, 1, 1, 0, 0}) {
		t.Fatalf("player consume %x", got)
	}
	if c.InventoryState.Bag[0].Count != 3 || c.Stats.HP != 50 {
		t.Fatal("optimistic consume")
	}
	inventoryClick(t, c, c.Inventory.RotateRight)
	if c.Inventory.Selected != 3 || c.Inventory.DisplayStats().HP != 100 {
		t.Fatal("selector did not select the pet")
	}
	c.Inventory.Slots[0].RightUp(0, 0, 0)
	if len(sent()) != 1 {
		t.Fatal("right-click used the selected pet consumable")
	}
	c.Inventory.Slots[0].DblClick()
	if got := sent(); len(got) != 2 || !bytes.Equal(got[1], []byte{23, 15, 1, 1, 3, 0}) {
		t.Fatalf("pet consume %x", got)
	}
	c.dispatch([]byte{23, 9, 1, 1})
	c.dispatch([]byte{8, 2, 4, 3, 0, 25, 1, 150, 0, 0, 0, 0, 0, 0, 0})
	if c.InventoryState.Bag[0].Count != 2 || c.Inventory.DisplayStats().HP != 150 || c.Stats.HP != 50 {
		t.Fatal("pet consume replies altered player or failed")
	}
	inventoryClick(t, c, c.Inventory.RotateLeft)
	if c.Inventory.Selected != 0 || c.Inventory.DisplayStats() != c.Stats {
		t.Fatal("selector did not return to player")
	}
	c.InventoryState.Bag[0] = game.Item{ID: game.ItemPotentialPill, Count: 1}
	c.Stats.Potential = 5
	c.Inventory.Use(1, false)
	if c.Inventory.UseDialog == nil || !c.Inventory.UseDialog.Visible {
		t.Fatal("Potential Pill must retain its own confirmation")
	}
	for _, child := range c.Inventory.UseDialog.Children {
		if b, ok := child.(*seui.FixedButton); ok && b.Image == c.Pics.Find("Btn_OK_1") {
			inventoryClick(t, c, b)
			break
		}
	}
	c.dispatch([]byte{23, 9, 1, 1})
	c.dispatch([]byte{23, 213, 1, 0, 4})
	if c.Stats.Potential != 4 || !c.InventoryState.Bag[0].Empty() {
		t.Fatal("pill failure reply handling")
	}
	c.Inventory.Select(3)
	release := binary.LittleEndian.AppendUint32([]byte{15, 2}, c.World.Player.ID)
	c.dispatch(append(release, 3))
	c.Frame()
	if c.Inventory.Selected != 0 || c.Inventory.RotateRight.Enabled {
		t.Fatal("released selection was retained")
	}
}

func TestInventoryRemoteOpensLocally(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.Inventory.Show()
	sent := wire(t, c)
	item := game.Item{ID: 34058, Count: 1}
	c.InventoryState.Bag[0] = item
	c.Inventory.Slots[0].DblClick()
	if c.Inventory.Remote == nil || !c.Inventory.Remote.Visible || c.Inventory.Remote.Image < 0 || len(sent()) != 0 || c.InventoryState.Bag[0] != item {
		t.Fatal("Remote did not open locally without consumption")
	}
	for _, child := range c.Inventory.Remote.Children {
		if _, button := child.(*seui.FixedButton); button && child.Base().Image < 0 {
			t.Fatalf("missing Remote control asset at %d,%d", child.Base().Left, child.Base().Top)
		}
	}
	if c.Inventory.Visible || c.Inventory.Remote.Image != c.Pics.Find("form_autoPlay_1") {
		t.Fatal("Remote did not close Inventory or used secondary skin")
	}
	c.Inventory.Show()
	if !c.Inventory.Visible || !c.Inventory.Remote.Visible {
		t.Fatal("Inventory cannot coexist with Remote")
	}
	c.Inventory.Hide()
	if !c.Inventory.Remote.Visible {
		t.Fatal("closing Inventory closed Remote")
	}
	inventoryClick(t, c, c.Inventory.Remote.Checks[0])
	if c.Inventory.Remote.Checks[0].Image != c.Pics.Find("btn_Check_1") {
		t.Fatal("Remote checkbox did not update")
	}
	c.Inventory.Remote.Thresholds[0].SetPos(75)
	c.Inventory.Show()
	c.Inventory.Slots[0].DblClick()
	if c.Inventory.Remote.Thresholds[0].Pos != 75 || c.Inventory.Remote.Checks[0].Image != c.Pics.Find("btn_Check_1") {
		t.Fatal("reopening Remote discarded local settings")
	}
	inventoryClick(t, c, c.Inventory.Remote.Settings)
	if c.Inventory.Remote.Plus == nil || !c.Inventory.Remote.Plus.Visible {
		t.Fatal("Level-up Setting did not open secondary window")
	}
	inventoryClick(t, c, c.Inventory.Remote.Plus.Tabs[2])
	if c.Inventory.Remote.Plus.Image != c.Pics.Find("Form_AutoPlayerPlus_2") {
		t.Fatal("missing Remote options tab")
	}
	if out := os.Getenv("ITEM_USE_SNAPSHOT"); out != "" {
		c.Frame()
		savePNG(t, out+".remote.png", c)
	}
	c.Inventory.Show()
	c.Inventory.Remote.Hide()
	if !c.Inventory.Visible || c.Inventory.Remote.Visible {
		t.Fatal("closing Remote changed Inventory visibility")
	}
	c.Inventory.Remote.Show()
	c.Inventory.ResetRemote()
	if c.Inventory.Remote.Visible || c.Inventory.Remote.Plus.Visible {
		t.Fatal("character boundary retained Remote windows")
	}
}

func TestInventoryFullRecoveryWarning(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	c.Inventory.Show()
	sent := wire(t, c)
	c.items[32011] = assets.NativeItem{Definition: game.ItemDefinition{EquipSlot: 8, Status: [2]uint16{25, 26}, Values: [2]int32{400, 400}}}
	item := game.Item{ID: 32011, Count: 3}
	c.InventoryState.Bag[0] = item
	c.Stats.HP, c.Stats.MaxHP, c.Stats.SP, c.Stats.MaxSP = 181, 181, 100, 100
	c.Inventory.Slots[0].DblClick()
	if len(sent()) != 0 || c.InventoryState.Bag[0] != item {
		t.Fatal("unneeded recovery sent a request or consumed the item")
	}
	if len(c.Notices.items) != 1 || string(c.Notices.items[0].text) != "This item cannot benefit the selected target right now." {
		t.Fatal("full recovery did not show native warning")
	}
	if c.UI.Modal != nil || !c.Inventory.Visible {
		t.Fatal("warning blocked or closed Inventory")
	}
	*now = now.Add(inventoryWarningDuration)
	c.Notices.update(*now)
	if len(c.Notices.items) != 0 {
		t.Fatal("warning did not expire")
	}
	c.Stats.SP--
	c.Inventory.Slots[0].DblClick()
	if len(sent()) != 1 || !bytes.Equal(sent()[0], []byte{23, 15, 1, 1, 0, 0}) {
		t.Fatal("combined recovery rejected a target needing SP")
	}
}
