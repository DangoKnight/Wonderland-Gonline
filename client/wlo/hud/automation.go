package hud

import (
	"encoding/json"
	"fmt"
	"wonderland-gonline/client/wlo/seui"
)

func (s *Social) AutomationInfo() seui.AutomationInfo {
	friends := []map[string]any{}
	for _, f := range s.Friends {
		friends = append(friends, map[string]any{"id": f.ID, "name": seui.DecodeText(f.Name), "nickname": seui.DecodeText(f.Nickname), "level": f.Level, "element": f.Element, "selected": f.Ticked, "mail": f.Mail})
	}
	nickname, blood, year, month, day := s.Profile()
	return seui.AutomationInfo{Kind: "social", Actions: []string{"tab", "page", "selectFriend", "mailFriend", "deleteSelected", "mailSelected", "profile", "emote", "pose", "shortcut", "phrase"}, Value: map[string]any{"friends": friends, "tab": s.Tab, "page": s.Page, "nickname": seui.DecodeText(nickname), "blood": blood, "year": year, "month": month, "day": day, "shortcuts": s.Slots(), "phrases": s.Phrases()}}
}
func (s *Social) AutomationInvoke(action string, raw json.RawMessage) error {
	switch action {
	case "tab":
		var tab int
		if seui.DecodeAutomation(raw, &tab) != nil || tab < 0 || tab >= len(s.Tabs) {
			return fmt.Errorf("invalid social tab")
		}
		s.SelectTab(tab)
	case "page":
		var page int
		if seui.DecodeAutomation(raw, &page) != nil || page < 1 || page > (len(s.Friends)+socialRows-1)/socialRows {
			return fmt.Errorf("invalid friends page")
		}
		s.Page = page
		s.Refresh()
	case "selectFriend", "mailFriend":
		var id uint32
		if seui.DecodeAutomation(raw, &id) != nil {
			return fmt.Errorf("friend ID required")
		}
		index := -1
		for i, f := range s.Friends {
			if f.ID == id {
				index = i
				break
			}
		}
		if index < 0 {
			return fmt.Errorf("friend unavailable")
		}
		s.Page = index/socialRows + 1
		if action == "selectFriend" {
			s.TickRow(index % socialRows)
		} else {
			s.MailRow(index % socialRows)
		}
	case "deleteSelected":
		if s.Ticked == 0 {
			return fmt.Errorf("no selected friends")
		}
		s.delFriend()
	case "mailSelected":
		if s.Ticked == 0 {
			return fmt.Errorf("no selected friends")
		}
		s.sendMail()
	case "profile":
		var p struct {
			Nickname         string
			Blood            byte
			Year, Month, Day int
		}
		if seui.DecodeAutomation(raw, &p) != nil || p.Blood > bloodTypes || p.Year < 0 || p.Year > 9999 || p.Month < 0 || p.Month > 12 || p.Day < 0 || p.Day > 31 {
			return fmt.Errorf("invalid profile")
		}
		name, err := seui.EncodeText(p.Nickname)
		if err != nil || len(name) > nicknameMax {
			return fmt.Errorf("invalid nickname")
		}
		s.SetProfile(name, p.Blood, p.Year, p.Month, p.Day)
		s.SaveSetUp()
	case "emote":
		var code byte
		if seui.DecodeAutomation(raw, &code) != nil || code <= EmoticonBase || int(code) > EmoticonBase+emoteCells || s.Emote == nil {
			return fmt.Errorf("invalid emote")
		}
		s.Emote(code)
	case "pose":
		var cell int
		if seui.DecodeAutomation(raw, &cell) != nil || cell < 1 || cell >= len(PoseTable) || s.Pose == nil {
			return fmt.Errorf("invalid pose")
		}
		s.Pose(cell)
	case "shortcut":
		var p struct {
			Index int
			Slot  EmoteSlot
		}
		if seui.DecodeAutomation(raw, &p) != nil || p.Index < 0 || p.Index >= EmoteSlots {
			return fmt.Errorf("invalid shortcut")
		}
		if p.Slot.Kind == 0 {
			if p.Slot.Value != 0 {
				return fmt.Errorf("empty shortcut must have zero value")
			}
		} else if (p.Slot.Kind != SlotEmoticon && p.Slot.Kind != SlotAction) || p.Slot.Value < 1 || p.Slot.Value > emoteCells || s.Env.Pics.Find(slotPicture(p.Slot)) < 0 {
			return fmt.Errorf("shortcut is not available in this client")
		}
		slots := s.Slots()
		slots[p.Index] = p.Slot
		s.SetSlots(slots)
		if s.EmotesChanged != nil {
			s.EmotesChanged()
		}
	case "phrase":
		var p struct {
			Index int
			Text  string
		}
		if seui.DecodeAutomation(raw, &p) != nil || p.Index < 0 || p.Index >= CtrlPhrases {
			return fmt.Errorf("invalid phrase slot")
		}
		text, err := seui.EncodeText(p.Text)
		if err != nil || len(text) > ctrlEditMax {
			return fmt.Errorf("invalid phrase")
		}
		phrases := s.Phrases()
		phrases[p.Index] = text
		s.SetPhrases(phrases)
		if s.EmotesChanged != nil {
			s.EmotesChanged()
		}
	default:
		return fmt.Errorf("unsupported social action")
	}
	return nil
}

func automationMail(m Mail) map[string]any {
	return map[string]any{"kind": m.Kind, "read": m.Read, "sender": m.Sender, "name": seui.DecodeText(m.Name), "text": seui.DecodeText(m.Text), "date": m.Date}
}
func (m *Mailbox) AutomationInfo() seui.AutomationInfo {
	inbox, sent := []map[string]any{}, []map[string]any{}
	for _, mail := range m.Inbox {
		inbox = append(inbox, automationMail(mail))
	}
	for _, mail := range m.Sent {
		sent = append(sent, automationMail(mail))
	}
	return seui.AutomationInfo{Kind: "mailbox", Actions: []string{"tab", "page", "open", "delete", "compose"}, Value: map[string]any{"inbox": inbox, "sent": sent, "tab": m.Tab, "page": m.Page, "selected": m.Selected, "unread": m.Unread}}
}
func (m *Mailbox) AutomationInvoke(action string, raw json.RawMessage) error {
	var index int
	switch action {
	case "tab":
		if seui.DecodeAutomation(raw, &index) != nil || index < 0 || index > 1 {
			return fmt.Errorf("invalid mailbox tab")
		}
		m.SelectTab(index)
	case "page":
		if seui.DecodeAutomation(raw, &index) != nil || index < 1 || index > max(1, m.Pages()) {
			return fmt.Errorf("invalid mailbox page")
		}
		m.Page = index
	case "open", "delete":
		if seui.DecodeAutomation(raw, &index) != nil || index < 1 || index > len(m.list()) {
			return fmt.Errorf("invalid mail index")
		}
		m.Page = (index-1)/mailboxRows + 1
		if action == "delete" {
			m.Selected = index
			m.deleteSelected()
		} else if m.Tab == 0 {
			m.OpenInbox(index)
		} else if m.OpenSent != nil {
			m.Selected = index
			m.OpenSent(index, m.Sent[index-1])
		}
	case "compose":
		if m.Compose == nil {
			return fmt.Errorf("mail compose unavailable")
		}
		m.Compose()
	default:
		return fmt.Errorf("unsupported mailbox action")
	}
	return nil
}

func (m *MailRead) AutomationInfo() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "mail", Actions: []string{}, Value: automationMail(m.Mail)}
}

func (m *MailRead) AutomationInvoke(string, json.RawMessage) error {
	return fmt.Errorf("use an explicit mail control")
}
