package app

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	battleResultsDuration  = 6 * time.Second // FUN_00394fe8's native results timeout.
	battleResultsMinWidth  = 220
	battleResultsTop       = 34 // Immediately below the top command icons.
	battleResultsTextTop   = 28
	battleResultsPadding   = 26
	battleResultsMaxRows   = 5 // The native notice board shows at most five lines.
	battleResultsRowHeight = 18
)

type battleResultActor struct {
	id           uint16
	name         string // Native Big5 bytes, kept intact for the game font.
	exp          uint32
	level, amity byte
}

type battleResultSnapshot struct {
	player battleResultActor
	gold   uint32
	pets   [game.MaxPets]battleResultActor
}

// FUN_00394f68 snapshots progression on entry; FUN_00394fe8 compares it on
// exit. This extends that native message with confirmed gold/item receipts.
// Nothing here predicts rewards, changes inventory or sends a request.
type battleResultsRuntime struct {
	owner          uint32
	active, closed bool
	before, after  battleResultSnapshot
	items          map[uint16]uint64
	joined         map[uint16]string
	form           *battleResultsForm
	until          time.Time
	lines          []string
}

func resultPet(p inventory.UsePet, slot int) battleResultActor {
	name := string(p.Name)
	if name == "" {
		name = fmt.Sprintf("Pet %d", slot+1)
	}
	return battleResultActor{id: p.ID, name: name, exp: p.Stats.EXP, level: p.Stats.Level, amity: p.Amity}
}

func (c *Client) beginBattleResults() {
	r := &c.battleResults
	if r.active && r.closed {
		c.showBattleResults()
	}
	r.owner = c.World.Player.ID
	r.active, r.closed = true, false
	r.before = battleResultSnapshot{player: battleResultActor{name: string(c.World.Player.Name), exp: c.Stats.EXP, level: c.Stats.Level}, gold: c.Stats.Gold}
	for i, p := range c.InventoryState.Pets {
		r.before.pets[i] = resultPet(p, i)
	}
	r.after = r.before
	r.items = map[uint16]uint64{}
	r.joined = map[uint16]string{}
}

func (c *Client) resetBattleResults() {
	f := c.battleResults.form
	if f != nil {
		f.Hide()
	}
	c.battleResults = battleResultsRuntime{form: f}
}

// Called after the normal native interpreter, also for deferred Gonline
// settlement. Absolute snapshots update the latest total, never add a delta
// twice. Keep collecting through the close packet and final map/stat refresh.
func (c *Client) recordBattleResultsPacket(p []byte) {
	r := &c.battleResults
	if !r.active || c.World == nil || c.World.Player.ID != r.owner || len(p) < 2 {
		return
	}
	if p[0] == protocol.CommandStats && p[1] == protocol.StatsStatUpdate && len(p) > 2 {
		switch p[2] {
		case world.StatEXP:
			r.after.player.exp = c.Stats.EXP
		case world.StatLevel:
			r.after.player.level = c.Stats.Level
		}
	}
	if p[0] == protocol.CommandCharacterState && p[1] == protocol.CharacterStateWireCode3 {
		r.after.player.exp, r.after.player.level = c.Stats.EXP, c.Stats.Level
	}
	if p[0] == protocol.CommandGold && p[1] == protocol.GoldBalance {
		r.after.gold = c.Stats.Gold
	}
	if p[0] == protocol.CommandStats && p[1] == protocol.StatsWireCode2 || p[0] == protocol.CommandPetControl {
		for i, p := range c.InventoryState.Pets {
			ownedBefore := false
			for _, before := range r.before.pets {
				ownedBefore = ownedBefore || before.id == p.ID
			}
			if p.ID != 0 && !ownedBefore {
				r.joined[p.ID] = resultPet(p, i).name
			}
			// Retain the last confirmed progression when a dead pet deserts;
			// never attribute a replacement's EXP to the former occupant.
			if p.ID != 0 && (r.before.pets[i].id == 0 || p.ID == r.before.pets[i].id) {
				r.after.pets[i] = resultPet(p, i)
			}
		}
	}
}

func (c *Client) recordBattleResultItems(before game.Inventory) {
	r := &c.battleResults
	if !r.active {
		return
	}
	for i, item := range c.InventoryState.Bag {
		if item.Empty() {
			continue
		}
		previous := 0
		if before[i].ID == item.ID {
			previous = int(before[i].Count)
		}
		if added := int(item.Count) - previous; added > 0 {
			r.items[item.ID] += uint64(added)
		}
	}
}

func resultDelta(label, name string, before, after uint32) string {
	if before == after {
		return ""
	}
	if after > before {
		return fmt.Sprintf("%s%s gained: %d", name, label, uint64(after)-uint64(before))
	}
	return fmt.Sprintf("%s%s lost: %d", name, label, uint64(before)-uint64(after))
}

func (c *Client) battleResultLines() []string {
	r := &c.battleResults
	var lines []string
	add := func(line string) {
		if line != "" {
			lines = append(lines, line)
		}
	}
	actor := func(before, after battleResultActor, amity bool) {
		name := after.name
		if name != "" {
			name += " "
		}
		add(resultDelta("EXP", name, before.exp, after.exp))
		if after.level > before.level {
			add(fmt.Sprintf("%slevel up: %d", name, after.level))
		}
		if amity {
			add(resultDelta("Amity", name, uint32(before.amity), uint32(after.amity)))
		}
	}
	actor(r.before.player, r.after.player, false)
	for i, before := range r.before.pets {
		after := r.after.pets[i]
		if before.id != 0 && before.id == after.id {
			actor(before, after, true)
		}
	}
	joined := make([]int, 0, len(r.joined))
	for id := range r.joined {
		joined = append(joined, int(id))
	}
	sort.Ints(joined)
	for _, id := range joined {
		add("Companion joined: " + r.joined[uint16(id)])
	}
	add(resultDelta("Gold", "", r.before.gold, r.after.gold))
	ids := make([]int, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	if len(ids) > 0 {
		add("Items obtained:")
	}
	for _, id := range ids {
		name := string(clientassets.Big5Text(c.items[uint16(id)].Definition.Name))
		if name == "" {
			name = fmt.Sprintf("Item #%d", id)
		}
		add(fmt.Sprintf("%s x%d", name, r.items[uint16(id)]))
	}
	return lines
}

func (c *Client) showBattleResults() {
	r := &c.battleResults
	r.lines = c.battleResultLines()
	r.active, r.closed = false, false
	if len(r.lines) == 0 {
		return
	}
	if r.form == nil {
		r.form = newBattleResultsForm(c.Env)
		c.UI.Add(r.form)
	}
	r.form.setLines(r.lines)
	r.form.AnimateMessage(func() time.Time { return c.Now() })
	r.until = c.Now().Add(battleResultsDuration)
	// FUN_00394fe8 publishes one multiline notice, replacing earlier board
	// entries. Clear transient loot notices so they cannot cover the summary.
	c.UI.CancelMessageExit(&c.Notices)
	c.Notices = Notices{panel: c.Notices.panel, Now: c.Notices.Now}
	r.form.Show()
}

func (c *Client) battleResultsTick(now time.Time) {
	r := &c.battleResults
	if r.active && r.closed && !c.battle.state.Active && c.sceneFade.mode == sceneFadeNone && c.mapReady && !c.sceneFrozen() {
		c.showBattleResults()
	}
	if r.form != nil && r.form.Visible {
		if c.World == nil || c.sceneFrozen() {
			r.form.Hide()
			return
		}
		if !now.Before(r.until) {
			r.form.Hide()
		}
	}
}

type battleResultsForm struct {
	seui.Form
	list   *battleResultsList
	scroll *seui.ScrollButton
}

// The native results use the centered notice board, not a titled dialog.
// Retain list navigation for long reward summaries, drawing each visible line
// with the notice board's fixed-width font and centered alignment.
type battleResultsList struct{ *seui.SelectText }

func (l *battleResultsList) Paint() {
	at := l.Abs()
	for row := 0; row < l.RowsShown && l.TopIndex+row < len(l.Items); row++ {
		line := l.Items[l.TopIndex+row]
		width := len(line) * noticeCharW
		x := at.X + (l.Width-width)/2 + noticeTextShift
		l.Env.Text.Draw(x, at.Y+row*battleResultsRowHeight, 0, false, true, l.Env.Screen,
			line, battleResultsRowHeight, width+noticeCharW, 0, noticeInk, noticeStyle)
	}
}

func newBattleResultsForm(env *seui.Env) *battleResultsForm {
	f := &battleResultsForm{}
	f.InitForm(f, env)
	f.Name = "Battle results"
	f.Init("panel22", 0, 0xa8, 0x14c, 0, 0, true, 0, battleResultsMinWidth, battleResultsTop)
	f.SetMargins(0x32, 0x20, 0x20, 0x3c)
	f.Dockable, f.Draggable, f.PixelHit = false, false, false
	// Initialize the standard navigation model, then bind its virtual Paint
	// to the centered results renderer before attaching it to the form.
	f.list = &battleResultsList{seui.NewSelectText(env, nil)}
	f.list.Component.InitComponent(f.list, env, f)
	f.list.RowHeight, f.list.LineHeight = battleResultsRowHeight, battleResultsRowHeight
	f.list.Color = noticeInk
	f.list.Embossed = true
	f.list.FocusBox, f.list.ShowHover, f.list.ShowSel, f.list.ShowIcons = false, false, false, false
	f.list.TextX, f.list.TextY = 0, 0
	f.scroll = seui.NewScrollButton(env, f)
	f.scroll.Init("Rail_H5", 0, 182, 17, 0, 0, true, 180, 17, battleResultsTextTop)
	f.scroll.SetThumb("bar_H4", 0, 21, 9, 0)
	f.scroll.SetCaps(7, 16)
	f.list.Scroll = f.scroll
	f.scroll.OnChange = func() { f.list.TopIndex = f.scroll.Pos }
	return f
}

func (f *battleResultsForm) setLines(lines []string) {
	wrapped := wrap([]byte(strings.Join(lines, "\r")))
	longest := 0
	for _, line := range wrapped {
		longest = max(longest, len(line))
	}
	f.Width = max(battleResultsMinWidth, longest*noticeCharW+noticeSideW)
	f.Left, f.Top = (ScreenWidth-f.Width)/2, battleResultsTop
	rows := min(battleResultsMaxRows, max(1, len(wrapped)))
	f.Height = battleResultsTextTop + rows*battleResultsRowHeight + battleResultsPadding
	f.list.Clear()
	f.list.SetBounds(noticeSideW/2, battleResultsTextTop, rows*battleResultsRowHeight, f.Width-noticeSideW)
	for _, line := range wrapped {
		f.list.Add(line, "-1", "")
	}
	f.list.SetTop(0)
	f.scroll.Height = rows * battleResultsRowHeight
	f.scroll.Left = f.Width - 36
	f.scroll.SetRange(len(wrapped), rows)
	f.scroll.SetVisible(len(wrapped) > rows)
}

func (f *battleResultsForm) WheelDown() { f.list.ScrollDown() }
func (f *battleResultsForm) WheelUp()   { f.list.ScrollUp() }
