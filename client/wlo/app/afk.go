package app

import (
	"encoding/json"
	"fmt"
	"golang.org/x/text/encoding/traditionalchinese"
	"image"
	"time"
	"unicode/utf8"
	"wonderland-gonline/client/wlo/sessionstream"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/localization"
	"wonderland-gonline/internal/protocol"
)

const (
	afkLogLimit        = 256
	afkRequestTimeout  = 10 * time.Second
	afkLineHeight      = 18
	afkLogRows         = 10
	afkLogTextBytes    = 240
	afkObserveInterval = 250 * time.Millisecond
)

var afkReturnRect = image.Rect(650, 10, 785, 37)

type afkLogEntry struct {
	At   time.Time
	Text string
}
type afkRuntime struct {
	active, pending, resync   bool
	groupRequested            bool
	binding                   protocol.AFKStreamBinding
	privateState, commonState protocol.AFKState
	sequence                  uint64
	latest                    *protocol.AFKSharedState
	sceneReceived, prepared   bool
	requested                 time.Time
	state                     protocol.AFKState
	log                       []afkLogEntry
	scroll                    int
	items                     map[uint16]int
	noticeAt                  time.Time
	battle                    bool
	turn                      int
	observeAt                 time.Time
	remoteWasActive           bool
	pet                       uint16
}

// AFK is transient, per authenticated connection, never a workspace preference.
func (c *Client) AFKSupported() bool {
	return c.connection.enabled && c.connection.features&protocol.ConnectionAFK != 0
}
func (c *Client) AFKActive() bool { return c.afk.active }

func (c *Client) afkInteractive() bool {
	return c.UI.Modal != nil || c.event.active || c.held && !c.battle.state.Active || c.Talk.Shown() || c.movie != nil || c.sport != nil || c.constellation != nil || c.settingsPromptForm != nil || !c.clinicUntil.IsZero() ||
		c.Inventory.Visible || c.Compound.Visible || c.Team.Visible || c.Settings.Visible || c.Services.Visible || c.Trade.Visible || c.Mall.Visible || c.Social.Visible || c.MailCompose.Visible || c.MailRead.Visible || c.MailSend.Visible || c.Mailbox.Visible || c.WorldPK.Visible || c.Notebook.Visible
}

func (c *Client) SetAFK(on bool) error {
	if !c.AFKSupported() || c.World == nil {
		return fmt.Errorf("%s", localization.Text("ui.afk.unsupported"))
	}
	if c.afk.pending || c.afk.resync {
		return fmt.Errorf("%s", localization.Text("ui.afk.pending"))
	}
	if on == c.afk.active {
		return nil
	}
	if on && (!c.mapReady || c.sceneFrozen() || c.afkInteractive()) {
		return fmt.Errorf("%s", localization.Text("ui.afk.rejected"))
	}
	mode := byte(protocol.AFKNormal)
	if on {
		mode = protocol.AFKActive
	}
	packet := []byte{protocol.CommandAFK, protocol.AFKSet, mode}
	c.afk.groupRequested = on && c.afkWorkspace != nil && c.afkWorkspace.afkToken != [protocol.AFKGroupTokenBytes]byte{} && c.connection.features&protocol.ConnectionAFKAggregation != 0
	if c.afk.groupRequested {
		packet = append(packet, c.afkWorkspace.afkToken[:]...)
	}
	if err := c.Net.Send(packet); err != nil {
		return err
	}
	c.cancelSessionInput()
	c.afk.pending, c.afk.requested = true, c.Now()
	return nil
}

func (c *Client) afkLog(text string) {
	if len(text) > afkLogTextBytes {
		text = text[:afkLogTextBytes]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	a := &c.afk
	if len(a.log) == afkLogLimit {
		copy(a.log, a.log[1:])
		a.log = a.log[:afkLogLimit-1]
	}
	a.log = append(a.log, afkLogEntry{c.Now(), text})
}

func (c *Client) afkPacket(p []byte) bool {
	if len(p) == 0 || p[0] != protocol.CommandAFK {
		return false
	}
	if !c.AFKSupported() || len(p) < 2 {
		return true
	}
	a := &c.afk
	switch p[1] {
	case protocol.AFKSet:
		if len(p) != 3 || p[2] > protocol.AFKActive || !a.pending {
			return true
		}
		a.pending = false
		a.active = p[2] == protocol.AFKActive
		if a.active {
			a.items = c.afkBagCounts()
			a.remoteWasActive = c.remote.active
			if c.TeamState != nil {
				a.pet = c.TeamState.BattlePet
			}
			a.battle, a.turn = c.battle.state.Active, int(c.battle.state.Turn)
			c.afkLog(localization.Text("ui.afk.enabled"))
			if c.Music != nil {
				c.Music.Stop()
			}
			c.applyLocalSettings()
		}
	case protocol.AFKRejected:
		if len(p) == 2 && a.pending {
			a.pending = false
			c.afkLog(localization.Text("ui.afk.rejected"))
			c.Notices.Show(clientassets.Big5Text(localization.Text("ui.afk.rejected")), 3*time.Second, c.Now())
		}
	case protocol.AFKSnapshot, protocol.AFKDelta:
		if !a.active {
			return true
		}
		base := a.state
		if a.binding.Stream != 0 {
			base = a.privateState
		}
		state, err := c.decodeAFKUpdate(base, p)
		if err != nil {
			c.Net.Close()
			c.disconnected()
			return true
		}
		if a.binding.Stream != 0 {
			a.privateState = state
			if a.commonState.Units == nil {
				return true
			}
			state = c.mergedAFKState()
		}
		c.applyAFKState(state)
	case protocol.AFKStreamBind:
		c.bindAFKStream(p)
	case protocol.AFKSharedSnapshot:
		stream, sequence, update, err := protocol.DecodeAFKSharedUpdate(p)
		if err != nil || c.World == nil || a.binding.Carrier != c.World.Player.ID || a.binding.Stream != stream || sequence <= a.sequence {
			return true
		}
		state, err := c.decodeAFKUpdate(a.commonState, update)
		if err != nil {
			c.Net.Close()
			c.disconnected()
			return true
		}
		shared := protocol.AFKSharedState{Stream: stream, Sequence: sequence, State: state}
		if c.acceptSharedAFK(shared) {
			a.latest = &shared
		}
	case protocol.AFKSceneBegin:
		if len(p) != 2 || !a.pending || !a.active {
			return true
		}
		a.binding = protocol.AFKStreamBinding{}
		a.latest = nil
		a.resync = true
		a.sceneReceived, a.prepared = false, false
		if c.World != nil {
			for id := range c.World.Peers {
				c.World.RemovePeer(id)
			}
		}
	case protocol.AFKSceneEnd:
		if len(p) != 2 || !a.resync {
			return true
		}
		a.active, a.pending, a.sceneReceived = false, false, true
		c.battle.loadAttempted = false
		c.battle.music = false
	}
	return true
}

func (c *Client) afkIgnore(p []byte) bool {
	if !c.afk.active || c.afk.resync || c.World == nil {
		return false
	}
	var party []uint32
	if c.TeamState != nil {
		party = c.TeamState.Members
	}
	return protocol.AFKNonessential(p, c.World.Player.ID, party)
}

func (c *Client) afkBagCounts() map[uint16]int {
	counts := map[uint16]int{}
	if c.InventoryState != nil {
		for _, item := range c.InventoryState.Bag {
			if !item.Empty() {
				counts[item.ID] += int(item.Count)
			}
		}
	}
	return counts
}

func (c *Client) afkTick() {
	a := &c.afk
	if a.resync && a.sceneReceived {
		settled, total := 0, 0
		if c.Sprites != nil {
			settled, total = c.Sprites.NativeProgress()
		}
		if a.prepared && settled == total && c.mapReady {
			a.resync, a.sceneReceived, a.prepared = false, false, false
			c.battle.music = false
			c.afkLog(localization.Text("ui.afk.restored"))
			c.applyLocalSettings()
			if !c.battle.state.Active {
				c.playMapMusic()
			}
		}
		return
	}
	if a.pending && c.Now().Sub(a.requested) > afkRequestTimeout {
		c.Net.Close()
		c.disconnected()
		return // Never reveal a partially refreshed scene.
	}
	if !a.active {
		return
	}
	if c.mapReady && c.mapLoading.active {
		c.stopMapLoading()
	}
	if !a.pending && c.mapReady && !c.mapLoading.active && c.afkInteractive() {
		c.afkLog(localization.Text("ui.afk.attention"))
		_ = c.SetAFK(false)
	}
	if a.battle != c.battle.state.Active {
		if c.battle.state.Active {
			c.afkLog(localization.Text("ui.afk.battle_started"))
		} else {
			c.afkLog(localization.Text("ui.afk.battle_ended") + " " + c.AgentBattleState().Result)
		}
		a.battle = c.battle.state.Active
	}
	if a.turn != int(c.battle.state.Turn) {
		a.turn = int(c.battle.state.Turn)
		c.afkLog(fmt.Sprintf(localization.Text("ui.afk.turn"), a.turn))
	}
	if a.remoteWasActive != c.remote.active {
		a.remoteWasActive = c.remote.active
		c.afkLog(fmt.Sprintf(localization.Text("ui.afk.remote_changed"), c.remote.active))
	}
	if c.Now().Before(a.observeAt) {
		return
	}
	a.observeAt = c.Now().Add(afkObserveInterval)
	for _, n := range c.Notices.items {
		if n.start.After(a.noticeAt) {
			text, _ := traditionalchinese.Big5.NewDecoder().String(string(n.text))
			c.afkLog(text)
			a.noticeAt = n.start
		}
	}
	c.Notices.update(c.Now())
}

// Observe receipts after native application, including packets inside a round
// chain. Record each inventory change instead of losing gain/use pairs between
// presentation frames. No inferred loot or client-side resource mutations.
func (c *Client) afkRecordPacket(p []byte) {
	if !c.afk.active || len(p) < 2 {
		return
	}
	if p[0] == protocol.CommandInventory {
		counts := c.afkBagCounts()
		for id, count := range counts {
			if d := count - c.afk.items[id]; d != 0 {
				c.afkLog(fmt.Sprintf(localization.Text("ui.afk.item_changed"), c.AgentItemName(id), d, count))
			}
		}
		for id, count := range c.afk.items {
			if counts[id] == 0 && count != 0 {
				c.afkLog(fmt.Sprintf(localization.Text("ui.afk.item_changed"), c.AgentItemName(id), -count, 0))
			}
		}
		c.afk.items = counts
	}
	if p[0] == protocol.CommandChat && (p[1] == protocol.ChatSystemMessage || p[1] == protocol.ChatGMMessage) && len(p) > chatIDEnd+1 {
		text, _ := traditionalchinese.Big5.NewDecoder().String(string(p[chatIDEnd+1:]))
		c.afkLog(text)
	}
	if (p[0] == protocol.CommandBattlePet || p[0] == protocol.CommandPetControl) && c.TeamState != nil && c.afk.pet != c.TeamState.BattlePet {
		c.afk.pet = c.TeamState.BattlePet
		c.afkLog(fmt.Sprintf(localization.Text("ui.afk.pet_changed"), c.afk.pet))
	}
}

func (c *Client) afkClick(x, y int) bool {
	if c.afk.active || c.afk.resync {
		if image.Pt(x, y).In(afkReturnRect) && !c.afk.pending {
			_ = c.SetAFK(false)
		}
		return true
	}
	return false
}

func (c *Client) drawAFK() {
	if !c.afk.active && !c.afk.resync {
		return
	}
	c.Screen.Fill(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0x1082)
	c.afkText(16, 15, localization.Text("ui.afk.title"), 600)
	c.Screen.Frame(afkReturnRect, 0x7bef)
	caption := localization.Text("ui.afk.return")
	if c.afk.pending || c.afk.resync {
		caption = localization.Text("ui.afk.synchronizing")
	}
	c.afkText(afkReturnRect.Min.X+8, afkReturnRect.Min.Y+6, caption, 125)
	c.afkText(16, 44, fmt.Sprintf(localization.Text("ui.afk.location"), c.afk.state.Map, c.afk.state.X, c.afk.state.Y, c.remote.active, c.battle.auto), 760)
	c.afkText(16, 64, fmt.Sprintf(localization.Text("ui.afk.timers"), c.Stats.Gold, c.timedStatus.training, c.timedStatus.minutes, c.timedStatus.paused), 760)
	y := 94
	for _, u := range c.afk.state.Units {
		kind := localization.Text("ui.afk.player")
		if u.Kind == protocol.AFKPet {
			kind = localization.Text("ui.afk.pet")
		}
		c.afkText(16, y, fmt.Sprintf(localization.Text("ui.afk.unit_stats"), kind, u.Name, u.Level, u.HP, u.MaxHP, u.SP, u.MaxSP, u.EXP, u.EXPNext), 770)
		y += afkLineHeight
	}
	y = max(280, y+10)
	c.Screen.Fill(image.Rect(12, y-5, 788, y-4), 0x4208)
	rows := min(afkLogRows, (ScreenHeight-y-22)/afkLineHeight)
	c.afk.scroll = min(c.afk.scroll, max(0, len(c.afk.log)-rows))
	end := len(c.afk.log) - c.afk.scroll
	start := max(0, end-rows)
	for _, line := range c.afk.log[start:end] {
		c.afkText(16, y, line.At.Format("15:04:05")+"  "+line.Text, 770)
		y += afkLineHeight
	}
	c.afkText(16, ScreenHeight-20, localization.Text("ui.afk.help"), 770)
}

func (c *Client) afkText(x, y int, text string, width int) {
	c.Env.Text.Draw(x, y, 0, false, true, c.Screen, clientassets.Big5Text(text), 15, width, 0, 0xffff, 0)
}

func (c *Client) applyAFKState(state protocol.AFKState) {
	if c.native != nil && c.replica == nil {
		data, _ := json.Marshal(state)
		c.recordNative(sessionstream.Event{Kind: "afkState", Arguments: data})
	}
	a := &c.afk
	if a.state.Map != 0 && state.Map != a.state.Map {
		c.afkLog(fmt.Sprintf(localization.Text("ui.afk.map_changed"), a.state.Map, state.Map))
	}
	for _, u := range state.Units {
		found := false
		for _, old := range a.state.Units {
			if old.Owner != u.Owner || old.ID != u.ID || old.Kind != u.Kind {
				continue
			}
			found = true
			if old.Level != u.Level {
				c.afkLog(fmt.Sprintf(localization.Text("ui.afk.level_changed"), u.Name, old.Level, u.Level))
			}
			if old.EXP != u.EXP {
				c.afkLog(fmt.Sprintf(localization.Text("ui.afk.exp_changed"), u.Name, old.EXP, u.EXP))
			}
			if old.HP != u.HP || old.SP != u.SP {
				c.afkLog(fmt.Sprintf(localization.Text("ui.afk.vitals_changed"), u.Name, u.HP, u.MaxHP, u.SP, u.MaxSP))
				if old.HP > 0 && u.HP == 0 {
					c.afkLog(fmt.Sprintf(localization.Text("ui.afk.knocked_out"), u.Name))
				}
			}
			if old.Amity != u.Amity {
				c.afkLog(fmt.Sprintf(localization.Text("ui.afk.amity_changed"), u.Name, old.Amity, u.Amity))
			}
		}
		if !found && len(a.state.Units) > 0 {
			c.afkLog(fmt.Sprintf(localization.Text("ui.afk.unit_added"), u.Name))
		}
	}
	for _, old := range a.state.Units {
		found := false
		for _, u := range state.Units {
			found = found || u.Owner == old.Owner && u.ID == old.ID && u.Kind == old.Kind
		}
		if !found {
			c.afkLog(fmt.Sprintf(localization.Text("ui.afk.unit_removed"), old.Name))
		}
	}
	a.state = state
}
