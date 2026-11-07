package mall

import (
	"fmt"
	"image"
	"strconv"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/localization"
	"wonderland-gonline/internal/protocol"
)

const (
	mallGameRows     = 4
	mallGameColumns  = 2
	mallGamePageSize = mallGameRows * mallGameColumns
	forgeGridLeft    = 274
	forgeGridTop     = 50
	forgeCellStep    = 34
	forgeGridColumns = 5
	forgeImageX      = 610
	forgeImageY      = 212
	// Native TSport kinds without a gameplay engine yet (do not substitute
	// unrelated minigames for these native menu positions).
	mallGameTileTurning = 21
	mallGameClaw        = 7
	mallGameCardPairs   = 16
	mallGameBoxing      = 9
)

// FUN_0024c9f4: display positions are independent of TSport kinds.
var mallGames = [...]struct {
	picture, kind int
	enabled       bool
}{
	{9, mallGameTileTurning, false}, {2, mallGameClaw, false}, {3, protocol.ArcadeSlots, true}, {4, mallGameCardPairs, false},
	{5, protocol.ArcadeSlots3, true}, {10, protocol.ArcadeEgg2, true}, {7, protocol.ArcadeSlots2, true}, {8, protocol.ArcadeMario3, true},
	{1, mallGameBoxing, false}, {6, protocol.ArcadeEgg, true},
}

func (f *Form) initServices() {
	f.ForgeUpgrades = clientassets.DefaultMallForging()
	for i := range f.Games {
		row := i
		b := seui.NewFixedButton(f.Env, f)
		b.Init("", 238+(i%mallGameColumns)*266, 90, 247, 0, 0, true, 90, 247, 45+(i/mallGameColumns)*104)
		b.OnClick = func() { f.openGame(row) }
		f.Games[i] = b
	}
	f.ForgeConfirm = seui.NewFixedButton(f.Env, f)
	f.ForgeConfirm.Init("Btn_confirm_1", 564, 20, 56, 0, 0, true, 20, 56, 352)
	f.ForgeConfirm.OnClick = f.confirmForge
	f.ForgeCancel = seui.NewFixedButton(f.Env, f)
	f.ForgeCancel.Init("Btn_Cancel_1", 637, 20, 56, 0, 0, true, 20, 56, 352)
	f.ForgeCancel.OnClick = func() {
		if !f.ForgePending {
			f.ForgeSlot = 0
		}
	}
}
func (f *Form) refreshServices() {
	games := !f.InCart && !f.State.Bonus && f.Category == TabGames
	forge := !f.InCart && !f.State.Bonus && f.Category == TabForging
	for i, b := range f.Games {
		index := (f.Page-1)*mallGamePageSize + i
		b.Visible = games && index < len(mallGames)
		// These thumbnails contain one image, unlike three-state buttons.
		// Paint them below so hovering never selects a nonexistent state row.
		b.Image = -1
		b.Enabled = !f.State.Busy() && !f.ForgePending
	}
	f.ForgeConfirm.Visible, f.ForgeCancel.Visible = forge, forge
	f.ForgeConfirm.Enabled = f.ForgeSlot != 0 && !f.ForgePending && f.selectedForgeMatches()
	f.ForgeCancel.Enabled = !f.ForgePending
	if games || forge {
		for _, b := range f.Add {
			b.Visible = false
		}
		f.Checkout.Visible = false
		f.ViewCart.Visible = false
	}
	f.Previous.Visible = !f.InCart && !forge
	f.Next.Visible = f.Previous.Visible
	for i, b := range f.Tabs {
		b.Enabled = !f.ForgePending
		b.Visible = !f.InCart && (!f.State.Bonus || i < int(TabFurniture))
	}
}
func (f *Form) openGame(row int) {
	index := (f.Page-1)*mallGamePageSize + row
	if index < 0 || index >= len(mallGames) || f.State.Busy() || f.ForgePending {
		return
	}
	entry := mallGames[index]
	if !entry.enabled {
		f.notice("ui.mall.game_unavailable")
		return
	}
	// Keep the mall open while awaiting entry. The minigame manager temporarily
	// hides visible forms and restores them when its native end reply arrives.
	f.send([]byte{protocol.CommandMall, protocol.MallGameCategory, byte(entry.kind)})
}
func (f *Form) paintGames() {
	f.Env.Screen.Fill(image.Rect(f.Left+218, f.Top+33, f.Left+765, f.Top+511), 0xd73e)
	f.Env.Screen.Frame(image.Rect(f.Left+218, f.Top+33, f.Left+765, f.Top+511), 0x7bd4)
	for i, b := range f.Games {
		index := (f.Page-1)*mallGamePageSize + i
		if b.Visible && index < len(mallGames) {
			f.pic(fmt.Sprintf("Icon_Game777_%d", mallGames[index].picture), b.Left, b.Top)
		}
	}
	f.text(240, 466, 512, 2, localization.UI("ui.mall.game_notice"))
	f.text(296, 523, 60, 1, fmt.Sprintf("%d / %d", f.Page, f.Pages()))
}
func (f *Form) forgeBag() game.Inventory {
	if f.Bag != nil {
		return f.Bag()
	}
	return game.Inventory{}
}
func (f *Form) forgeOwner(slot byte) byte {
	if slot < 1 || int(slot) > game.BagSize {
		return 0
	}
	bag := f.forgeBag()
	definitions := map[uint16]game.ItemDefinition{}
	for _, it := range bag {
		definitions[it.ID] = f.Items[it.ID].Definition
	}
	cells, err := bag.Occupancy(definitions)
	if err != nil {
		return 0
	}
	return cells[slot-1]
}
func (f *Form) selectedForgeMatches() bool {
	return f.ForgeSlot > 0 && int(f.ForgeSlot) <= game.BagSize && f.forgeBag()[f.ForgeSlot-1] == f.ForgeItem && !f.ForgeItem.Empty()
}
func (f *Form) selectForge(slot byte) {
	if f.ForgePending || slot < 1 || int(slot) > game.BagSize {
		return
	}
	bag := f.forgeBag()
	slot = f.forgeOwner(slot)
	if slot == 0 {
		return
	}
	item := bag[slot-1]
	if item.Empty() || item.Count != 1 || item.Locked {
		return
	}
	if upgrade, ok := f.ForgeUpgrades[item.ID]; ok {
		if upgrade.Next == 0 {
			f.notice("ui.mall.forge_maximum")
			return
		}
	} else if !f.Items[item.ID].Definition.CanPointForge() || item.Forge() >= game.MaxForgeProgress {
		f.notice("ui.mall.forge_ineligible")
		return
	}
	f.ForgeSlot, f.ForgeItem = slot, item
	f.Refresh()
}
func (f *Form) confirmForge() {
	if f.ForgePending || !f.selectedForgeMatches() {
		return
	}
	slot, item := f.ForgeSlot, f.ForgeItem
	prompt := fmt.Sprintf(localization.UI("ui.mall.forge_confirm_points"), game.PointForgeCost)
	if u, ok := f.ForgeUpgrades[item.ID]; ok {
		prompt = fmt.Sprintf(localization.UI("ui.mall.forge_confirm_scrolls"), u.Scrolls)
	}
	accept := func() {
		if f.Visible && f.Category == TabForging && !f.ForgePending && f.ForgeSlot == slot && f.ForgeItem == item && f.selectedForgeMatches() {
			f.ForgePending = true
			f.ForgeAt = f.Now()
			if !f.send([]byte{protocol.CommandMall, protocol.MallForge, slot}) {
				f.notice("ui.mall.timeout")
			}
			f.Refresh()
		}
	}
	if f.Confirm != nil {
		f.Confirm(prompt, accept)
	}
}
func (f *Form) ForgeReply(p []byte) bool {
	if len(p) < 3 || p[0] != protocol.CommandMall || p[1] != protocol.MallForgeResult {
		return false
	}
	switch p[2] {
	case protocol.MallForgeProgressSucceeded:
		if len(p) != 6 || p[4] > game.MaxForgeProgress || p[5] != 0 {
			return false
		}
	case protocol.MallForgeRollFailed, protocol.MallForgeInsufficientPoints, protocol.MallForgeInsufficientScrolls, protocol.MallForgeUpgradeSucceeded, protocol.MallForgeRejected:
		if len(p) != 3 {
			return false
		}
	default:
		return false
	}
	if !f.ForgePending {
		return true
	}
	if p[2] == protocol.MallForgeProgressSucceeded {
		if p[3] != f.ForgeSlot || !f.selectedForgeMatches() || f.ApplyForge == nil || !f.ApplyForge(p[3], p[4]) {
			return false
		}
		f.ForgeItem = f.forgeBag()[f.ForgeSlot-1]
	}
	f.ForgePending = false
	key := "ui.mall.forge_rejected"
	switch p[2] {
	case protocol.MallForgeProgressSucceeded, protocol.MallForgeUpgradeSucceeded:
		key = "ui.mall.forge_success"
	case protocol.MallForgeRollFailed:
		key = "ui.mall.forge_failed"
	case protocol.MallForgeInsufficientPoints:
		key = "ui.mall.insufficient_points"
	case protocol.MallForgeInsufficientScrolls:
		key = "ui.mall.forge_need_scrolls"
	}
	if p[2] == protocol.MallForgeUpgradeSucceeded {
		f.ForgeSlot = 0
	}
	f.notice(key)
	f.Refresh()
	return true
}
func (f *Form) paintForge() {
	f.Env.Screen.Fill(image.Rect(f.Left+218, f.Top+33, f.Left+765, f.Top+511), 0xd73e)
	f.pic("Form_ BeatScaffold_1", 273, 46)
	for i, it := range f.forgeBag() {
		if it.Empty() {
			continue
		}
		x, y := forgeGridLeft+i%forgeGridColumns*forgeCellStep, forgeGridTop+i/forgeGridColumns*forgeCellStep
		width, height := f.Items[it.ID].Definition.Footprint()
		r := image.Rect(f.Left+x, f.Top+y, f.Left+x+width*forgeCellStep-2, f.Top+y+height*forgeCellStep-2)
		icon := f.Env.Pics.Find(strconv.Itoa(int(f.Items[it.ID].Icon)))
		w, h := f.Env.Pics.Size(icon)
		if w > 0 && h > 0 {
			scale := min(1.0, float64(r.Dx())/float64(w), float64(r.Dy())/float64(h))
			w, h = max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
			left, top := r.Min.X+(r.Dx()-w)/2, r.Min.Y+(r.Dy()-h)/2
			f.Env.Pics.DrawStretch(f.Env.Screen, icon, image.Rect(left, top, left+w, top+h), true)
		}
		if it.Count > 1 {
			f.text(x, y+20, 32, 1, strconv.Itoa(int(it.Count)))
		}
		if byte(i+1) == f.ForgeSlot {
			f.Env.Screen.Frame(r, 0xf800)
		}
	}
	cost, grade := 0, 0
	if f.selectedForgeMatches() {
		f.item(Entry{ID: f.ForgeItem.ID}, forgeImageX, forgeImageY)
		grade = int(f.ForgeItem.Forge())
		if u, ok := f.ForgeUpgrades[f.ForgeItem.ID]; ok {
			grade = max(0, int(u.Scrolls)-1)
		} else {
			cost = game.PointForgeCost
		}
	}
	f.text(676, 66, 36, 1, strconv.Itoa(cost))
	f.text(646, 90, 56, 1, fmt.Sprintf("+%d", grade))
}
