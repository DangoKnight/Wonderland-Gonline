package team

import (
	"fmt"
	"image"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/protocol"
)

const instanceDefinitionPageSize = 10

// FUN_0026ab14 and FUN_00183d44 use 17-pixel rows in Btn_ArrowL/R_5.
const instancePageArrowSize = 17
const instanceRowTop = 87
const instanceRowStep = 70
const definitionRowTop = 94
const definitionRowHeight = 29
const definitionListWidth = 149

type InstanceListing struct {
	ID            uint16
	Creator, Name string
	Members       byte
}
type InstanceMember struct {
	ID    uint32
	Level byte
	Name  string
}
type Instances struct {
	seui.Component
	Owner               *Form
	Definitions         []assets.InstanceDefinition
	Listings            []InstanceListing
	Page, Pages         byte
	Room, Definition    uint16
	Members             []InstanceMember
	New                 *creationForm
	Selected            int
	DefinitionPage      int
	Rows                [protocol.InstancePageSize]*seui.FixedButton
	CreateRows          [instanceDefinitionPageSize]*seui.FixedButton
	RoomRows            [protocol.InstancePageSize]*seui.Editor
	Labels              [instanceDefinitionPageSize]*seui.Editor
	Detail              *seui.Editor
	Metadata            [4]*seui.Editor
	Description         [9]*seui.Editor
	PageLabel           *seui.Editor
	DefinitionPageLabel *seui.Editor
	Name                *seui.Editor
	Info                *seui.Editor
	Join, Leave, Start  *seui.FixedButton
}

func NewInstances(owner *Form) *Instances {
	f := &Instances{Owner: owner, Selected: -1}
	f.InitComponent(f, owner.Env, owner)
	f.Init("", 0, 0, 0, 0, 0, false, formHeight, formWidth, 0)
	f.Visible = false
	button := func(parent seui.Control, asset string, x, y, w int, fn func()) *seui.FixedButton {
		b := seui.NewFixedButton(f.Env, parent)
		h := 20
		if asset == "Btn_ArrowL_5" || asset == "Btn_ArrowR_5" {
			w, h = instancePageArrowSize, instancePageArrowSize
		}
		b.Init(asset, x, h, w, 0, 0, true, h, w, y)
		b.OnClick = fn
		return b
	}

	button(f, "Btn_CreateDup", 149, 465, 71, f.ShowCreate)
	button(f, "Btn_ArrowL_5", 135, 430, 21, func() {
		if f.Page > 1 {
			f.Browse(f.Page - 1)
		}
	})
	button(f, "Btn_ArrowR_5", 220, 430, 21, func() {
		if f.Page < f.Pages {
			f.Browse(f.Page + 1)
		}
	})
	for i := range f.Rows {
		row := i
		f.Rows[i] = button(f, "Btn_Module_1", 279, instanceRowTop+i*instanceRowStep, 56, func() {
			if row < len(f.Listings) {
				f.Request(protocol.Builder{protocol.CommandInstance, protocol.InstanceJoinRoom}.U16(f.Listings[row].ID))
			}
		})
		f.RoomRows[i] = f.label(f, 30, instanceRowTop+i*instanceRowStep, 240, 42)
	}
	f.Info = f.label(f, 275, 59, 55, 20)
	f.Leave = button(f, "Btn_LeaveTeam_1", 30, 385, 70, func() {
		owner.Confirm("Leave this instance room?", 0, func() { f.Request([]byte{protocol.CommandInstance, protocol.InstanceLeaveRoom}) })
	})
	f.Start = button(f, "Btn_Create", 260, 385, 56, func() { f.Request([]byte{protocol.CommandInstance, protocol.InstanceStartRoom}) })
	f.New = &creationForm{owner: f}
	f.New.InitForm(f.New, f.Env)
	f.New.Dockable = false
	f.New.Init("Form_CreateDupMis", 420, 0, 0, 0, 0, false, 463, 377, 75)
	button(f.New, "Btn_Cancel_1", 202, 419, 56, f.New.Hide)
	button(f.New, "Btn_Close_s_1", 327, 24, 18, f.New.Hide)
	button(f.New, "Btn_Create", 126, 419, 56, f.Create)
	button(f.New, "Btn_ArrowL_5", 45, 390, 17, func() {
		if f.DefinitionPage > 0 {
			f.DefinitionPage--
			f.Refresh()
		}
	})
	button(f.New, "Btn_ArrowR_5", 130, 390, 17, func() {
		if (f.DefinitionPage+1)*instanceDefinitionPageSize < len(f.Definitions) {
			f.DefinitionPage++
			f.Refresh()
		}
	})
	for i := range f.CreateRows {
		row := i
		f.CreateRows[i] = button(f.New, "", 19, definitionRowTop+i*definitionRowHeight, definitionListWidth, func() { f.Selected = f.DefinitionPage*instanceDefinitionPageSize + row; f.Refresh() })
		f.CreateRows[i].Height = definitionRowHeight
		f.Labels[i] = f.label(f.New, 28, definitionRowTop+i*definitionRowHeight, definitionListWidth-9, definitionRowHeight)
		label := f.Labels[i]
		label.DrawText = func(x, y int, _ []byte) {
			f.Env.Text.Draw(x, label.Abs().Y+3, 0, false, true, f.Env.Screen, label.Text, 15, definitionListWidth-9, 0, textInk, 0)
		}
	}
	f.Detail = f.label(f.New, 175, 55, 180, 22)
	for i := range f.Metadata {
		f.Metadata[i] = f.label(f.New, 265, 95+i*23, 70, 22)
	}
	for i := range f.Description {
		f.Description[i] = f.label(f.New, 175, 205+i*18, 175, 22)
	}
	f.PageLabel = f.label(f, 172, 425, 60, 20)
	f.DefinitionPageLabel = f.label(f.New, 78, 385, 60, 20)
	f.Name = seui.NewEditor(f.Env, f.New)
	f.Name.Init("", 230, 20, 120, 0, 0, false, 20, 120, 365)
	f.Name.SetMaxLen(protocol.InstanceNameLimit)
	f.Name.Visible = false // Native creation uses the selected dungeon name.
	inner := seui.NewComponent(f.Env, f)
	inner.Init("Form_DupChoose", 26, 0, 0, 0, 0, false, 404, 314, 85)
	owner.Env.UI.Add(f.New)
	return f
}
func (f *Instances) label(parent seui.Control, x, y, w, h int) *seui.Editor {
	e := seui.NewEditor(f.Env, parent)
	e.Init("", x, 0, 0, 0, 0, false, h, w, y)
	e.ReadOnly = true
	e.Enabled = false
	e.SetColor(textInk)
	return e
}
func (f *Instances) Request(p []byte) {
	if f.Owner.CanAct != nil && !f.Owner.CanAct() {
		return
	}
	f.Owner.send(p)
}
func (f *Instances) Show() {
	f.Owner.SelectTab(true)
	f.Owner.Form.Show()
	f.Browse(1)
	f.Refresh()
}
func (f *Instances) Hide() { f.New.Hide(); f.Visible = false; f.Owner.Hide() }

// The shared Teams form owns dragging and hit testing; this is only tab content.
func (f *Instances) HitTest(int, int) bool { return false }
func (f *Instances) Reset() {
	f.Hide()
	f.Listings = nil
	f.Members = nil
	f.Room = 0
	f.Definition = 0
	f.Page = 0
	f.Pages = 0
	f.Selected = -1
	f.Refresh()
}
func (f *Instances) Browse(page byte) {
	if page == 0 {
		page = 1
	}
	f.Request([]byte{protocol.CommandInstance, protocol.InstanceBrowse, page})
}
func (f *Instances) ShowCreate() {
	if f.Room != 0 {
		f.Owner.Notice("Leave the current instance room first.")
		return
	}
	f.New.Show()
	f.Refresh()
}
func (f *Instances) Create() {
	if f.Selected < 0 || f.Selected >= len(f.Definitions) {
		f.Owner.Notice("Select an instance first.")
		return
	}
	p, err := protocol.Builder{protocol.CommandInstance, protocol.InstanceCreate}.U16(f.Definitions[f.Selected].ID).String(string(f.Name.Text))
	if err != nil {
		f.Owner.Notice(err.Error())
		return
	}
	f.Request(p)
}
func (f *Instances) Refresh() {
	for i, b := range f.Rows {
		b.Visible = f.Room == 0 && i < len(f.Listings)
		b.Enabled = f.Room == 0
		f.RoomRows[i].SetText(nil)
		if f.Room != 0 && i < len(f.Members) {
			m := f.Members[i]
			f.RoomRows[i].SetText(fmt.Appendf(nil, "%s LV %d", m.Name, m.Level))
		}
		if f.Room == 0 && i < len(f.Listings) {
			r := f.Listings[i]
			f.RoomRows[i].SetText(fmt.Appendf(nil, "%s (%d players)\n%s", r.Name, r.Members, r.Creator))
		}
	}
	info := fmt.Sprintf("%d", f.Room)
	f.Info.SetText([]byte(info))
	f.Leave.Visible = f.Room != 0
	f.Start.Visible = len(f.Members) > 0 && f.Members[0].ID == f.Owner.State.Self
	for i, b := range f.CreateRows {
		index := f.DefinitionPage*instanceDefinitionPageSize + i
		b.Visible = index < len(f.Definitions)
		f.Labels[i].SetText(nil)
		if index < len(f.Definitions) {
			f.Labels[i].SetText([]byte(f.Definitions[index].Name))
		}
	}
	f.PageLabel.SetText(fmt.Appendf(nil, "%d/%d", f.Page, f.Pages))
	f.DefinitionPageLabel.SetText(fmt.Appendf(nil, "%d/%d", f.DefinitionPage+1, max(1, (len(f.Definitions)+instanceDefinitionPageSize-1)/instanceDefinitionPageSize)))
	f.Detail.SetText(nil)
	for _, label := range f.Metadata {
		label.SetText(nil)
	}
	for _, label := range f.Description {
		label.SetText(nil)
	}
	if f.Selected >= 0 && f.Selected < len(f.Definitions) {
		d := f.Definitions[f.Selected]
		guild := "None"
		if d.GuildOnly {
			guild = "Required"
		}
		f.Detail.SetText([]byte(d.Name))
		for i, value := range []string{fmt.Sprint(d.Capacity), fmt.Sprint(d.MinimumLevel), guild, fmt.Sprintf("%d min", d.Minutes)} {
			f.Metadata[i].SetText([]byte(value))
		}
		// The native description wraps at the column edge, including long words.
		description := []rune(d.Description)
		var lines []string
		for len(description) > 0 {
			count := min(22, len(description))
			lines = append(lines, string(description[:count]))
			description = description[count:]
		}
		for i, line := range lines {
			if i >= len(f.Description) {
				break
			}
			f.Description[i].SetText([]byte(line))
		}
	}
}
func (f *Instances) Apply(p []byte) bool {
	if len(p) < 2 || p[0] != protocol.CommandInstance {
		return false
	}
	r := protocol.NewReader(p[2:])
	switch p[1] {
	case protocol.InstanceBrowse:
		pages, page, count := r.U8(), r.U8(), r.U8()
		if count > protocol.InstancePageSize {
			return false
		}
		rows := make([]InstanceListing, 0, count)
		for i := byte(0); i < count; i++ {
			row := InstanceListing{ID: r.U16(), Creator: r.String(), Name: r.String(), Members: r.U8()}
			guildLen := r.U8()
			r.Bytes(int(guildLen))
			rows = append(rows, row)
		}
		if r.Err() != nil || r.Remaining() != 0 {
			return false
		}
		f.Pages, f.Page, f.Listings = pages, page, rows
	case protocol.InstanceRoomSnapshot:
		room, definition, count := r.U16(), r.U16(), r.U8()
		members := make([]InstanceMember, 0, count)
		for i := byte(0); i < count; i++ {
			members = append(members, InstanceMember{r.U32(), r.U8(), r.String()})
		}
		if r.Err() != nil || r.Remaining() != 0 || room == 0 && count != 0 {
			return false
		}
		f.Room, f.Definition, f.Members = room, definition, members
		if room != 0 {
			f.New.Hide()
		}
	case protocol.InstanceMembership:
		operation := r.U8()
		switch operation {
		case 1:
			r.U8()
			r.U16()
			r.U8()
			_ = r.String() // Creator name is supplied by the room snapshot.
			r.U32()
		case 2:
		default:
			return false
		}
		if r.Err() != nil || r.Remaining() != 0 {
			return false
		}
	case protocol.InstanceDefinitions:
		count := r.U8()
		for i := byte(0); i < count; i++ {
			r.U16()
			r.U8()
		}
		if r.Err() != nil || r.Remaining() != 0 {
			return false
		}
	case protocol.InstanceStatus:
		code := r.U8()
		if r.Err() != nil || r.Remaining() != 0 {
			return false
		}
		messages := map[byte]string{protocol.InstanceUnavailable: "This instance's dungeon content is pending verification.", protocol.InstanceLevelTooLow: "Level too low.", protocol.InstanceRoomsFull: "The instance room is full.", protocol.InstanceRejected: "Instance request rejected."}
		message := messages[code]
		if message == "" {
			message = fmt.Sprintf("Instance status %d", code)
		}
		f.Owner.Notice(message)
	default:
		return false
	}
	f.Refresh()
	return true
}

// The native list highlights the entire selected row beneath its text.
type creationForm struct {
	seui.Form
	owner *Instances
}

func (f *creationForm) Paint() {
	f.Form.Paint()
	row := f.owner.Selected - f.owner.DefinitionPage*instanceDefinitionPageSize
	if row >= 0 && row < instanceDefinitionPageSize && f.owner.Selected < len(f.owner.Definitions) {
		a := f.Abs()
		y := a.Y + definitionRowTop + row*definitionRowHeight
		f.Env.Screen.Fill(image.Rect(a.X+19, y, a.X+19+definitionListWidth, y+definitionRowHeight), surface.RGB565(136, 188, 240))
	}
}
