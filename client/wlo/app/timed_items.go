package app

import (
	"encoding/binary"
	"fmt"
	"time"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/localization"
	"wonderland-gonline/internal/protocol"
)

const (
	timedShortcutSize        = 32
	timedStatueIcon   uint16 = 34101
)

// Native FUN_0018f4ec selects a representative potion for each tenths multiplier.
var timedPotionIcons = map[byte]uint16{15: 34107, 20: 34108, 25: 34109, 30: 34110, 35: 34111, 40: 34112, 45: 34113, 50: 34114, 55: 34115, 60: 34116, 80: 34123, 100: 34173}

type timedStatus struct {
	kind        game.TimedItemKind
	tenths      byte
	paused      bool
	minutes     uint32
	training    uint16
	boost, pass *seui.FixedButton
	art         [2]uint16
}

// Native status packets own these controls. Reconnects use the server's saved
// remaining time; the client never grants or extends an entitlement locally.
func (c *Client) timedStatusPacket(p []byte) bool {
	if len(p) < 2 {
		return false
	}
	s := &c.timedStatus
	notice := ""
	switch {
	case p[0] == protocol.CommandInventory && p[1] == protocol.InventoryStopwatchStatus:
		if len(p) == 4 {
			s.paused = p[3] != 0
		}
	case p[0] == protocol.CommandCharacterState && p[1] == protocol.CharacterStateStatueTime:
		if len(p) == 7 {
			s.kind = game.TimedItemStatue
			s.minutes = binary.LittleEndian.Uint32(p[3:])
			s.tenths = 20
			c.Notices.Show([]byte(fmt.Sprintf(localization.UI("ui.timed.exp_time"), float64(s.tenths)/10, s.minutes)), 3*time.Second, c.Now())
		} else if len(p) == 4 && p[2] == protocol.TimedStatueExpired {
			s.kind = game.TimedItemNone
			s.paused = false
			notice = "ui.timed.exp_expired"
		}
	case p[0] == protocol.CommandCharacterState && p[1] == protocol.CharacterStateEXPPotion:
		if len(p) < 4 {
			return true
		}
		switch p[2] {
		case protocol.TimedEffectInitial:
			if len(p) == 5 {
				s.kind = game.TimedItemNone
				if p[3] != 0 {
					s.kind = game.TimedItemPotion
				}
				s.tenths = p[4]
			}
		case protocol.TimedPotionActivated:
			if len(p) == 9 {
				s.kind = game.TimedItemPotion
				s.minutes = binary.LittleEndian.Uint32(p[4:])
				s.tenths = p[8]
				c.Notices.Show([]byte(fmt.Sprintf(localization.UI("ui.timed.exp_time"), float64(s.tenths)/10, s.minutes)), 3*time.Second, c.Now())
			}
		case protocol.TimedEffectRejected:
			notice = "ui.timed.exp_repeated"
			if p[3] == protocol.TimedRejectIncompatible {
				notice = "ui.timed.exp_incompatible"
			}
		case protocol.TimedPotionExpired:
			s.kind = game.TimedItemNone
			s.paused = false
			notice = "ui.timed.exp_expired"
		}
	case p[0] == protocol.CommandInventory && p[1] == protocol.InventoryTrainingTime:
		if len(p) == 7 && p[2] == protocol.TrainingTimeUpdate {
			s.training = binary.LittleEndian.Uint16(p[4:])
			if p[6] == protocol.TimedNotify {
				c.Notices.Show([]byte(fmt.Sprintf(localization.UI("ui.timed.training_time"), s.training)), 3*time.Second, c.Now())
			}
		} else if len(p) == 3 && p[2] == protocol.TrainingTimeExpired {
			s.training = 0
			notice = "ui.timed.training_expired"
		}
	default:
		return false
	}
	if notice != "" {
		c.Notices.Show([]byte(localization.UI(notice)), 3*time.Second, c.Now())
	}
	c.timedStatusButtons()
	return true
}

func (c *Client) timedStatusButtons() {
	if c.Env == nil || c.UI == nil {
		return
	}
	s := &c.timedStatus
	for i := 0; i < 2; i++ {
		ptr := &s.boost
		id := timedStatueIcon
		if s.kind == game.TimedItemPotion {
			if icon, ok := timedPotionIcons[s.tenths]; ok {
				id = icon
			}
		}
		visible := s.kind != game.TimedItemNone
		if i == 1 {
			ptr = &s.pass
			id = game.ItemTrainingVoucher
			visible = s.training > 0
		}
		if !visible {
			if *ptr != nil {
				(*ptr).Hide()
			}
			continue
		}
		if *ptr == nil {
			b := seui.NewFixedButton(c.Env, nil)
			b.Init("", ScreenWidth-timedShortcutSize-8, 0, 0, 0, 0, false, timedShortcutSize, timedShortcutSize, 450-i*38)
			b.OnTop = true
			training := i == 1
			b.OnClick = func() {
				if c.Net == nil {
					return
				}
				if training {
					_ = c.Net.Send([]byte{protocol.CommandInventory, protocol.InventoryTrainingControl, protocol.TrainingControlEnter})
					return
				}
				if s.paused {
					_ = c.Net.Send([]byte{protocol.CommandInventory, protocol.InventoryStopwatchControl, 0})
					return
				}
				sub := byte(protocol.CharacterStateStatueTime)
				if s.kind == game.TimedItemPotion {
					sub = protocol.CharacterStateEXPPotion
				}
				_ = c.Net.Send([]byte{protocol.CommandCharacterState, sub})
			}
			c.UI.Add(b)
			*ptr = b
		}
		b := *ptr
		if s.art[i] != id {
			icon := fmt.Sprint(c.items[id].Icon)
			c.loadPictures(icon)
			b.SetAnimation(icon, 0, 1, 1, time.Hour, timedShortcutSize, timedShortcutSize, 0)
			b.StartAnimation()
			s.art[i] = id
		}
		training := i == 1
		b.BindLanguage("hint", func() {
			hint := fmt.Sprintf(localization.UI("ui.timed.exp_hint"), float64(c.timedStatus.tenths)/game.EXPMultiplierBase)
			if c.timedStatus.paused {
				hint += " " + localization.UI("ui.timed.paused")
			}
			if training {
				hint = fmt.Sprintf(localization.UI("ui.timed.training_time"), c.timedStatus.training)
			}
			b.SetHint([]byte(hint))
		})
		b.Show()
	}
}

func (c *Client) clearTimedStatus() {
	s := &c.timedStatus
	if s.boost != nil {
		s.boost.Hide()
	}
	if s.pass != nil {
		s.pass.Hide()
	}
	*s = timedStatus{boost: s.boost, pass: s.pass, art: s.art}
}
