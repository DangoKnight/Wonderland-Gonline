package inventory

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const giftCardRows = 4

type cardDialog struct {
	seui.Form
	owner          *Form
	slot           byte
	item           game.Item
	rewards        []protocol.GiftCardReward
	page, selected int
	waiting        bool
	rows           [giftCardRows]*seui.FixedButton
	confirm        *seui.FixedButton
}

func (f *Form) requestCard(slot byte, it game.Item) {
	if f.LoadCardPictures != nil {
		f.LoadCardPictures("Form_GiftExchange_1", "Btn_Close_S_1", "Btn_Close_1", "Btn_GiftExchange_1", "btn_UnCheck_1", "btn_Check_1", "Btn_ArrowL_5", "Btn_ArrowR_5")
	}
	if f.CardDialog == nil {
		d := &cardDialog{owner: f, selected: -1}
		d.InitForm(d, f.Env)
		d.Name = "TGiftExchangeCardForm"
		d.Dockable = false
		d.Init("Form_GiftExchange_1", 176, 0, 0, 0, 0, false, 293, 447, 153)
		button := func(name string, x, y, w, h int, click func()) *seui.FixedButton {
			b := seui.NewFixedButton(f.Env, d)
			b.Init(name, x, h, w, 0, 0, true, h, w, y)
			b.OnClick = click
			return b
		}
		button("Btn_Close_S_1", 393, 22, 18, 18, d.Hide)
		button("Btn_Close_1", 240, 250, 56, 20, d.Hide)
		d.confirm = button("Btn_GiftExchange_1", 152, 250, 56, 20, d.exchange)
		button("Btn_ArrowL_5", 175, 224, 17, 17, func() {
			if d.page > 0 {
				d.page--
				d.sync()
			}
		})
		button("Btn_ArrowR_5", 256, 224, 17, 17, func() {
			if (d.page+1)*giftCardRows < len(d.rewards) {
				d.page++
				d.sync()
			}
		})
		for i := range d.rows {
			row := i
			d.rows[i] = button("btn_UnCheck_1", 25, 90+i*34, 16, 16, func() {
				if !d.waiting {
					d.selected = d.page*giftCardRows + row
					d.sync()
				}
			})
		}
		f.Env.UI.Add(d)
		f.CardDialog = d
	}
	d := f.CardDialog
	d.slot, d.item, d.rewards, d.page, d.selected, d.waiting = slot, it, nil, 0, -1, false
	f.send(protocol.Builder{protocol.CommandInventory, protocol.InventoryGiftCard, protocol.GiftCardIndex, slot}.U16(it.ID).U8(0))
}
func (d *cardDialog) sync() {
	for i, b := range d.rows {
		b.Visible = d.page*giftCardRows+i < len(d.rewards)
		b.Enabled = !d.waiting
		b.Image = d.Env.Pics.Find("btn_UnCheck_1")
		if d.selected == d.page*giftCardRows+i {
			b.Image = d.Env.Pics.Find("btn_Check_1")
		}
	}
	d.confirm.Enabled = d.selected >= 0 && !d.waiting
}
func (d *cardDialog) Hide() {
	if d.Env.UI.Modal == d {
		d.Env.UI.SetModal(false, d)
	}
	d.Form.Hide()
	d.rewards = nil
	d.slot = 0
}
func (d *cardDialog) KeyDown(key uint16, _ byte) {
	if key == 0x1b {
		d.Hide()
	}
}
func (d *cardDialog) exchange() {
	if d.waiting || d.selected < 0 || d.selected >= len(d.rewards) {
		return
	}
	f := d.owner
	if f.State.Bag[d.slot-1] != d.item || !f.allowed() {
		d.Hide()
		return
	}
	r := d.rewards[d.selected]
	d.waiting = true
	d.sync()
	f.send(protocol.Builder{protocol.CommandInventory, protocol.InventoryGiftCard, protocol.GiftCardDetails, d.slot, 1, byte(d.selected + 1)}.U16(r.ItemID))
}
func (d *cardDialog) Paint() {
	if d.slot == 0 || d.owner.State.Bag[d.slot-1] != d.item {
		d.Hide()
		return
	}
	d.Env.Pics.Draw(d.Env.Screen, d.Image, d.Left, d.Top, true)
	text := func(x, y, w int, s string) {
		d.Env.Text.Draw(d.Left+x, d.Top+y, 0, false, true, d.Env.Screen, []byte(s), 16, w, 0, inventoryTextInk, 0)
	}
	text(51, 54, 350, d.owner.State.Items[d.item.ID].Definition.Name)
	for i := 0; i < giftCardRows; i++ {
		j := d.page*giftCardRows + i
		if j >= len(d.rewards) {
			break
		}
		r := d.rewards[j]
		d.owner.drawItem(game.Item{ID: r.ItemID, Count: r.Quantity}, d.Left+51, d.Top+81+i*34, true)
		text(92, 90+i*34, 300, d.owner.State.Items[r.ItemID].Definition.Name)
	}
	text(204, 224, 50, fmt.Sprintf("%d/%d", d.page+1, (len(d.rewards)+giftCardRows-1)/giftCardRows))
}

// Card metadata shares an outbound code with older vehicle stalls. Only consume
// strictly shaped catalogs, and show choices only for our pending owned card.
func (f *Form) CardPacket(p []byte) bool {
	if len(p) == 2 && p[0] == protocol.CommandInventory && p[1] == protocol.InventoryItemUse && f.CardDialog != nil && f.CardDialog.waiting {
		f.CardDialog.Hide()
		return false
	}
	if len(p) < 3 || p[0] != protocol.CommandInventory || p[1] != protocol.InventoryGiftCard {
		return false
	}
	switch p[2] {
	case protocol.GiftCardIndex:
		if len(p) < 5 {
			return false
		}
		n := int(binary.LittleEndian.Uint16(p[3:]))
		if len(p) != 5+3*n {
			return false
		}
		if n > 0 {
			query := protocol.Builder{protocol.GiftCardCatalogRequest, protocol.GiftCardIndex}.U16(uint16(n))
			for i := 0; i < n; i++ {
				query = query.U16(binary.LittleEndian.Uint16(p[5+3*i:]))
			}
			f.send(query)
		}
		return true
	case protocol.GiftCardDetails:
		cards, ok := protocol.ParseGiftCardDetails(p)
		if !ok {
			return false
		}
		d := f.CardDialog
		if d == nil || d.slot == 0 {
			return true
		}
		for _, card := range cards {
			if card.ID == d.item.ID && f.State.Bag[d.slot-1] == d.item {
				if f.LoadCardPictures != nil {
					for _, r := range card.Rewards {
						f.LoadCardPictures(strconv.Itoa(int(f.State.Items[r.ItemID].Icon)))
					}
				}
				d.rewards = card.Rewards
				d.page, d.selected = 0, -1
				d.sync()
				d.Show()
				f.Env.UI.SetModal(true, d)
				break
			}
		}
		return true
	case protocol.GiftCardRedeemed:
		if len(p) != 9 {
			return false
		}
		if f.CardDialog != nil {
			f.CardDialog.Hide()
		}
		return true
	case protocol.GiftCardOpen:
		if len(p) != 4 {
			return false
		}
		if d := f.CardDialog; d != nil && d.slot == p[3] && len(d.rewards) > 0 {
			d.Show()
			d.Env.UI.SetModal(true, d)
		}
		return true
	}
	return false
}
