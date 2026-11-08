package inventory

import (
	"strconv"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/internal/localization"
)

// Assignment artwork is a single icon, not a three-state button sheet.
// Retain button input handling while painting the same frame in every state.
type remoteAssignmentButton struct{ seui.FixedButton }

func (b *remoteAssignmentButton) Paint() { b.GrBasic.Paint() }

// THL_AutoPlayFormPlus: constructor FUN_001ba580 and refresh FUN_001d6294.
// Keep native positions; the labels are supplied by the localized form skin.
func (p *remotePlusForm) initAdvanced() {
	if load := p.owner.owner.LoadCardPictures; load != nil {
		load("Icon_sk60022s", "Icon_sk60021s", "Icon_sk60041s")
	}
	for i, y := range []int{73, 156, 277, 312, 347, 418} {
		i := i
		p.Checks[i] = remoteButton(p.Env, p, "btn_UnCheck_1", 27, y, 16, 16, func() {
			if !p.saveFields() {
				return
			}
			p.draft[p.tab].Enabled[i] = !p.draft[p.tab].Enabled[i]
			p.refreshAdvanced()
		})
	}
	positions := [RemoteAssignmentCount][2]int{{162, 91}, {162, 123}, {114, 177}, {114, 210}, {114, 243}, {187, 269}, {187, 305}, {162, 410}, {92, 375}}
	for i, xy := range positions {
		i := i
		b := &remoteAssignmentButton{}
		b.InitFixedButton(b, p.Env, p)
		b.Init("", xy[0], 30, 30, 0, 0, true, 30, 30, xy[1])
		b.OnClick = func() { p.cycleAssignment(i) }
		b.PixelHit = false // Empty/transparent cells must remain clickable.
		b.OnUp = func() {
			if !p.saveFields() {
				return
			}
			a := &p.draft[p.tab].Assignments[i]
			a.ID, a.Item = 0, false
			p.refreshAdvanced()
		}
		p.Assignments[i] = b
	}
	for i, xy := range [6][2]int{{166, 183}, {166, 213}, {166, 243}, {66, 275}, {66, 311}, {166, 380}} {
		i := i
		p.Targets[i] = remoteButton(p.Env, p, "Btn_Role_1", xy[0], xy[1], 43, 20, func() {
			if !p.saveFields() {
				return
			}
			index := [...]int{2, 3, 4, 5, 6, 8}[i]
			a := &p.draft[p.tab].Assignments[index]
			a.Target = a.Target%RemoteTargetParty + 1
			p.refreshAdvanced()
		})
	}
	for i, y := range []int{276, 311, 346} {
		e := seui.NewEditor(p.Env, p)
		e.Init("", 137, 0, 0, 0, 0, false, 18, 52, y)
		e.Color, e.Filter, e.MaxLen, e.Limit = inventoryTextInk, 1, 5, true
		p.Fields[i] = e
	}
	p.Compare = remoteButton(p.Env, p, "Btn_Smaller", 89, 344, 43, 20, func() {
		if !p.saveFields() {
			return
		}
		p.draft[p.tab].EnemyAbove = !p.draft[p.tab].EnemyAbove
		p.refreshAdvanced()
	})
}

func (p *remotePlusForm) saveFields() bool {
	if p.tab >= len(p.draft) {
		return true
	}
	values := [3]int{}
	for i, e := range p.Fields {
		if e == nil {
			return true
		}
		n, err := strconv.Atoi(string(e.Text))
		if err != nil || n < 0 || n > remoteSettingLimit {
			p.owner.owner.notice(localization.UI("ui.inventory.enter_valid_remote_settings_c1f18582"))
			return false
		}
		values[i] = n
	}
	a := &p.draft[p.tab]
	a.HP, a.SP, a.EnemyHP = values[0], values[1], values[2]
	return true
}

func (p *remotePlusForm) refreshAdvanced() {
	visible := p.tab < len(p.draft)
	for _, b := range p.Checks {
		b.SetVisible(visible)
	}
	for _, b := range p.Assignments {
		b.SetVisible(visible)
	}
	for _, b := range p.Targets {
		b.SetVisible(visible)
	}
	for _, e := range p.Fields {
		e.SetVisible(visible)
	}
	p.Compare.SetVisible(visible)
	if !visible {
		return
	}
	a := &p.draft[p.tab]
	for i, b := range p.Checks {
		name := "btn_UnCheck_1"
		if a.Enabled[i] {
			name = "btn_Check_1"
		}
		b.Image = p.Env.Pics.Find(name)
	}
	var choices []RemoteChoice
	if p.owner.owner.RemoteChoices != nil {
		choices = p.owner.owner.RemoteChoices(p.tab == 1)
	}
	for i, b := range p.Assignments {
		b.Image = -1
		b.SetHint(nil)
		if !a.Assignments[i].Item {
			icon, key := "", ""
			switch a.Assignments[i].ID {
			case skills.BasicAttack:
				icon, key = "Icon_sk60022s", "ui.app.attack_4cd548f3"
			case skills.Defense:
				icon, key = "Icon_sk60021s", "ui.app.defend_8e2b7761"
			case skills.Flee:
				icon, key = "Icon_sk60041s", "ui.app.flee_4c629e5d"
			}
			if icon != "" {
				b.Image = p.Env.Pics.Find(icon)
				b.SetHint([]byte(localization.UI(key)))
				continue
			}
		}
		if p.owner.owner.RemoteChoices == nil {
			continue
		}
		for _, choice := range choices {
			if choice.ID == a.Assignments[i].ID && choice.Item == a.Assignments[i].Item {
				b.Image = p.Env.Pics.Find(choice.Icon)
				b.SetHint(choice.Name)
				break
			}
		}
	}
	for i, b := range p.Targets {
		index := [...]int{2, 3, 4, 5, 6, 8}[i]
		target := a.Assignments[index].Target
		if target == 0 {
			target = RemoteTargetEnemy
			if i == 3 || i == 4 {
				target = RemoteTargetParty
			}
		}
		b.Image = p.Env.Pics.Find("Btn_Role_" + strconv.Itoa(int(target)))
	}
	for i, value := range []int{a.HP, a.SP, a.EnemyHP} {
		p.Fields[i].SetText([]byte(strconv.Itoa(value)))
	}
	name := "Btn_Smaller"
	if a.EnemyAbove {
		name = "Btn_Surpass"
	}
	p.Compare.Image = p.Env.Pics.Find(name)
}

// FUN_001d6f84 (other build FUN_001bce84): empty/custom -> Basic Atk,
// Basic Atk -> Defense -> Flee -> Basic Atk, for all nine assignment cells.
func (p *remotePlusForm) cycleAssignment(index int) {
	if p.tab >= len(p.draft) || !p.saveFields() {
		return
	}
	a := &p.draft[p.tab].Assignments[index]
	next := uint16(skills.BasicAttack)
	if !a.Item {
		switch a.ID {
		case skills.BasicAttack:
			next = skills.Defense
		case skills.Defense:
			next = skills.Flee
		}
	}
	a.ID, a.Item = next, false
	p.refreshAdvanced()
}

func (f *Form) AssignRemoteSkill(id uint16, pet bool, x, y int) bool {
	if f.Remote == nil || f.Remote.Plus == nil || !f.Remote.Plus.Visible {
		return false
	}
	p := f.Remote.Plus
	if (p.tab == 1) != pet {
		return false
	}
	return p.assignAt(id, false, x, y)
}

func (p *remotePlusForm) assignAt(id uint16, item bool, x, y int) bool {
	if p.tab >= len(p.draft) || p.owner.owner.RemoteChoices == nil {
		return false
	}
	for i, b := range p.Assignments {
		if !b.HitTest(x, y) {
			continue
		}
		if i < 2 && item {
			return true
		}
		if !p.saveFields() {
			return true
		}
		for _, choice := range p.owner.owner.RemoteChoices(p.tab == 1) {
			if choice.ID == id && choice.Item == item {
				p.draft[p.tab].Assignments[i].ID, p.draft[p.tab].Assignments[i].Item = id, item
				p.refreshAdvanced()
				break
			}
		}
		return true
	}
	return false
}
