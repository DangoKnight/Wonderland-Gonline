package app

import "wonderland-gonline/internal/localization"

import (
	"encoding/binary"
	"fmt"
	"image"
	"strconv"
	"time"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"

	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	serviceNoticeDuration   = 3 * time.Second
	clinicAnimationDuration = 5 * time.Second // FUN_002b6d08 sets the native five-second rest timer.
	clinicBedPicture        = "8108"          // Rattan Bed (item 39011), as shown in the treatment reference.
	clinicLyingPose         = 26              // FUN_003bb51c: action 0x1a, frame 1.
	clinicBedItem           = 39011
	clinicSleepInterval     = 300 * time.Millisecond
	clinicSleepPicture      = "Word_Red_64U" // FUN_003ba370, font 1 upper-case strip.
	clinicSleepLetters      = 26
)

type clinicPrompt struct {
	seui.Form
	c    *Client
	cost uint32
}

func (c *Client) initServices() {
	f := inventory.NewServiceForm(c.Env, c.Inventory)
	c.Services = f
	f.LoadItem = func(id uint16) {
		if icon := c.items[id].Icon; icon != 0 {
			c.loadPictures(strconv.Itoa(int(icon)))
		}
	}
	f.Send = func(p []byte) { c.Net.Send(p) }
	f.Gold = func() uint32 { return c.Stats.Gold }
	f.Owner = func() uint32 {
		if c.World != nil {
			return c.World.Player.ID
		}
		return 0
	}
	f.Notice = func(s string) { c.Notices.Show([]byte(s), serviceNoticeDuration, c.Now()) }
	f.OnClose = func() {
		c.Net.Send([]byte{protocol.CommandEvent, protocol.EventChoice, eventChoiceCancel})
		c.Net.Send([]byte{protocol.CommandEvent, protocol.EventAcknowledge})
	}
	f.OnSubmit = func() {
		if c.event.active {
			c.Net.Send([]byte{protocol.CommandEvent, protocol.EventChoice, eventChoiceCancel})
			c.Net.Send([]byte{protocol.CommandEvent, protocol.EventAcknowledge})
		}
	}
	sprites := map[uint16]*role.NPC{}
	f.DrawPortrait = func(p inventory.UsePet, b image.Rectangle) {
		if c.npcTemplates == nil {
			c.npcTemplates, _ = world.NPCTemplates(c.Assets)
		}
		if portrait := c.resources.petPortrait(c.npcTemplates[uint32(p.ID)]); portrait != nil {
			c.Screen.DrawStretch(b, portrait, true)
		}
	}
	f.DrawPet = func(p inventory.UsePet, x, y int) {
		if c.lib == nil {
			return
		}
		if c.npcTemplates == nil {
			c.npcTemplates, _ = world.NPCTemplates(c.Assets)
		}
		sprite := sprites[p.ID]
		if sprite == nil {
			t := c.npcTemplates[uint32(p.ID)]
			sprite = role.NewNPC(c.lib, t.Look, t.Colors)
			sprite.Now = func() time.Time { return c.Now() }
			sprites[p.ID] = sprite
		}
		sprite.Draw(c.Screen, x, y, 8)
	}
	c.UI.Add(f)
}
func (c *Client) serviceNotice(s string) { c.Notices.Show([]byte(s), serviceNoticeDuration, c.Now()) }
func (c *Client) servicePacket(p []byte) bool {
	if len(p) < 2 || c.Services == nil {
		return false
	}
	switch p[0] {
	case protocol.CommandStats:
		if p[1] == protocol.StatsWireCode2 && c.Services.Visible && c.Services.Mode == inventory.ServiceHotel && len(p) == 15 && p[3] >= 1 && p[3] <= game.MaxPets && c.InventoryState.Pets[p[3]-1].ID == 0 {
			if len(c.Services.PendingPetStats) < game.MaxPets*32 {
				c.Services.PendingPetStats = append(c.Services.PendingPetStats, append([]byte(nil), p...))
			}
			return true
		}
		return false
	case protocol.CommandCharacterSelection:
		if p[1] == protocol.ShopCatalogOpen && len(p) == 6 {
			id := binary.LittleEndian.Uint32(p[2:])
			if id == protocol.ShopCatalogProps || id == protocol.ShopCatalogWeapons || id == protocol.ShopCatalogArmor {
				c.openServiceCatalog(id)
				return true
			}
		}
		return false
	case protocol.CommandGold:
		if p[1] != protocol.GoldGain && p[1] != protocol.GoldSpend {
			return false
		}
		if len(p) != 6 {
			return true
		}
		n := binary.LittleEndian.Uint32(p[2:])
		if p[1] == protocol.GoldGain {
			c.Stats.Gold = uint32(min(uint64(game.MaxGold), uint64(c.Stats.Gold)+uint64(n)))
			c.serviceNotice(fmt.Sprintf(localization.UI("ui.app.obtain_d_gold_bf448578"), n))
		} else {
			c.Stats.Gold -= min(c.Stats.Gold, n)
			c.serviceNotice(fmt.Sprintf(localization.UI("ui.app.decrease_d_gold_d49a78e2"), n))
		}
		return true
	case protocol.CommandLegacyStorage:
		if p[1] == protocol.LegacyStorageWireCode6 && len(p) == 2 {
			c.Services.Open(inventory.ServiceStorage)
			return true
		}
		return false
	case protocol.CommandStorage:
		if !c.Services.StoragePacket(p) && c.Unhandled != nil {
			c.Unhandled(p)
		}
		return true
	case protocol.CommandShop:
		switch p[1] {
		case protocol.ShopPropsWindow, protocol.ShopEquipmentWindow:
			c.Services.SaleMode = 0
			if p[1] == protocol.ShopPropsWindow {
				c.Services.SaleMode = 1
			}
			c.Services.Open(inventory.ServiceSell)
		case protocol.ShopBuy, protocol.ShopSell:
			if len(p) != 3 {
				return true
			}
			c.Services.Waiting = false
			if p[2] != 0 {
				messages := []string{"", localization.UI("ui.app.purchase_rejected_b3496b8d"), localization.UI("ui.app.not_enough_gold_c1d5d70f"), localization.UI("ui.app.inventory_full_fab91b5c")}
				if p[1] == protocol.ShopSell {
					messages = []string{"", localization.UI("ui.app.gold_limit_reached_ca5ca241"), localization.UI("ui.app.item_unavailable_210366e5"), localization.UI("ui.app.too_many_items_a536c916"), localization.UI("ui.app.cannot_sell_that_item_78ccfec5")}
				}
				s := localization.UI("ui.app.transaction_rejected_feee6f4b")
				if int(p[2]) < len(messages) {
					s = messages[p[2]]
				}
				c.serviceNotice(s)
			} else if p[1] == protocol.ShopBuy {
				c.Services.Hide()
			} else {
				c.Services.Selected = [2][game.BagSize]bool{}
			}
		}
		return true
	case protocol.CommandNPCService:
		switch p[1] {
		case protocol.ClinicRestOffer:
			if len(p) == 6 {
				c.showClinicPrompt(binary.LittleEndian.Uint32(p[2:]))
			}
		case protocol.ClinicRestConfirm:
			if len(p) == 3 {
				c.closeClinicPrompt()
				if p[2] != 0 {
					c.serviceNotice(localization.UI("ui.app.treatment_unavailable_c544600a"))
				}
				if c.clinicUntil.IsZero() {
					c.event.done = true
				}
			}
		case protocol.ClinicRestAnimation:
			if c.World == nil {
				return true
			}
			c.closeClinicPrompt()
			c.clinicUntil = c.Now().Add(clinicAnimationDuration)
			c.loadPictures(clinicBedPicture, clinicSleepPicture)
			c.World.StopWalk()
			c.groundHeld = false
			if c.sfx != nil {
				c.sfx.Play(`sound\wav0149.wav`)
			}
		default:
			if !c.Services.HotelPacket(p) {
				if c.Unhandled != nil {
					c.Unhandled(p)
				}
			} else if p[1] == protocol.PetHotelCopyToTeam {
				c.assignPetSkills()
				c.Skills.Refresh()
				c.BattleSkills.Refresh()
			}
		}
		return true
	}
	return false
}

// Event questions in item-picker mode contain the native shop slots and IDs.
func (c *Client) shopQuestion(q world.Question) bool {
	if q.Mode == 0 {
		return false
	}
	var stock []inventory.ShopEntry
	for i, e := range q.Entries {
		if e.Kind == world.QuestionItem && e.Type == world.QuestionOption {
			slot := byte(i + 1)
			if e.Slot != 0 {
				slot = e.Slot
			}
			stock = append(stock, inventory.ShopEntry{Slot: slot, ID: e.Value})
		}
	}
	if len(stock) == 0 {
		return false
	}
	c.Services.Stock = inventory.ShopStock(c.InventoryState.Items, stock)
	c.Services.Open(inventory.ServiceBuy)
	c.event.done = false
	return true
}
func (c *Client) closeClinicPrompt() {
	if c.clinicPrompt != nil {
		c.clinicPrompt.Hide()
		if c.UI.Modal == c.clinicPrompt {
			c.UI.Modal = nil
		}
		c.UI.Remove(c.clinicPrompt)
		c.clinicPrompt = nil
	}
}
func (c *Client) showClinicPrompt(cost uint32) {
	c.Talk.Hide()
	c.Services.Hide()
	c.closeClinicPrompt()
	if cost == ^uint32(0) {
		c.serviceNotice(localization.UI("ui.app.no_need_to_rest_eb9ea5fe"))
		c.event.done = true
		return
	}
	f := &clinicPrompt{c: c, cost: cost}
	f.InitForm(f, c.Env)
	f.AnimateMessage(func() time.Time { return c.Now() })
	f.Dockable = false
	f.Init("panel22", 260, 168, 324, 0, 0, true, 120, 280, 34)
	f.SetMargins(50, 32, 32, 60)
	e := seui.NewEditor(c.Env, f)
	e.Init("", 24, 0, 0, 0, 0, false, 44, 230, 24)
	e.ReadOnly = true
	e.Color = 0xffe0
	text := localization.UI("ui.app.this_stay_is_free_stay_562774b6")
	if cost != 0 {
		text = fmt.Sprintf(localization.UI("ui.app.rest_for_d_gold_9e7a0752"), cost)
	}
	e.SetText([]byte(text))
	for i, pic := range []string{"Btn_confirm_1", "Btn_Cancel_1"} {
		yes := i == 0
		b := seui.NewFixedButton(c.Env, f)
		b.Init(pic, 62+i*100, 20, 56, 0, 0, true, 20, 56, 82)
		b.OnClick = func() {
			c.closeClinicPrompt()
			if yes {
				c.Net.Send([]byte{protocol.CommandNPCService, protocol.ClinicRestConfirm})
			} else {
				c.Net.Send([]byte{protocol.CommandEvent, protocol.EventChoice, eventChoiceCancel})
				c.Net.Send([]byte{protocol.CommandEvent, protocol.EventAcknowledge})
			}
		}
	}
	c.clinicPrompt = f
	c.UI.Add(f)
	f.Show()
	c.UI.Modal = f
}
func (c *Client) drawClinicRest() {
	if c.clinicUntil.IsZero() || c.World == nil {
		return
	}
	if !c.Now().Before(c.clinicUntil) {
		c.clinicUntil = time.Time{}
		c.event.done = true
		return
	}
	c.Screen.FillAlpha(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0, 160)
	// FUN_003bb51c -> FUN_00459048: center the item's inventory
	// footprint source rectangle, independently of transparent picture margins.
	bed := c.items[clinicBedItem].Definition
	w, h := bed.Footprint()
	w, h = w*32+(w-1)*2, h*32+(h-1)*2
	picture := c.Pics.Find(clinicBedPicture)
	pw, ph := c.Pics.Size(picture)
	left, top := max(0, (pw-2)/2-w/2), max(0, (ph-2)/2-h/2)
	bedTop := (ScreenHeight - h) / 2
	c.Pics.DrawRect(c.Screen, picture, (ScreenWidth-w)/2, bedTop, image.Rect(left, top, left+w, top+h), true)
	if human, ok := c.Inventory.Preview.(*role.Human); ok {
		preview := *human
		preview.SetVehicle(0)
		preview.SetPetMount(nil)
		preview.Hold(1, false)
		preview.DrawBody(c.Screen, ScreenWidth/2, ScreenHeight/2-10, clinicLyingPose)
	}
	if c.clinicSleepGlyph == nil {
		index := c.Pics.Find(clinicSleepPicture)
		width, height := c.Pics.Size(index)
		height /= clinicSleepLetters
		if width > 0 && height > 0 {
			c.clinicSleepGlyph = surface.New(width, height)
			c.Pics.DrawRect(c.clinicSleepGlyph, index, 0, 0, image.Rect(0, 25*height, width, 26*height), true)
		}
	}
	phase := int(c.Now().Sub(c.clinicUntil.Add(-clinicAnimationDuration))/clinicSleepInterval+1) & 3
	if phase != 0 && c.clinicSleepGlyph != nil {
		position := [...]image.Point{{}, {ScreenWidth / 2, bedTop - 10}, {ScreenWidth/2 + 10, bedTop - 45}, {ScreenWidth/2 + 25, bedTop - 100}}
		scale := [...]float64{0, 0.2, 0.4, 0.7}
		at := position[phase]
		width := int(float64(c.clinicSleepGlyph.W)*scale[phase] + 0.5)
		height := int(float64(c.clinicSleepGlyph.H)*scale[phase] + 0.5)
		c.Screen.DrawStretch(image.Rect(at.X, at.Y, at.X+width, at.Y+height), c.clinicSleepGlyph, true)
	}
}

func (c *Client) openServiceCatalog(id uint32) {
	if c.World == nil {
		return
	}
	m, err := world.MapRecord(c.Assets, c.World.Player.Map)
	if err != nil {
		return
	}
	kind := byte(2)
	if id == protocol.ShopCatalogProps {
		kind = 3
	}
	for _, g := range m.Interactive {
		if len(g.Header) > 0 && g.Header[0] == kind {
			var stock []inventory.ShopEntry
			for at := 0; at+5 <= len(g.Members); at += 5 {
				r := g.Members[at:]
				if r[0] != 0 && r[3] == world.QuestionItem && r[4] == world.QuestionOption {
					stock = append(stock, inventory.ShopEntry{Slot: r[0], ID: binary.LittleEndian.Uint16(r[1:])})
				}
			}
			c.Services.Stock = inventory.ShopStock(c.InventoryState.Items, stock)
			c.Services.Open(inventory.ServiceBuy)
			return
		}
	}
}

func (c *Client) resetServices() {
	if c.Services != nil {
		c.Services.Reset()
	}
	c.closeClinicPrompt()
	c.clinicUntil = time.Time{}
}
func (c *Client) clinicTick() {
	if !c.clinicUntil.IsZero() && !c.Now().Before(c.clinicUntil) {
		c.clinicUntil = time.Time{}
		c.event.done = true
	}
}
