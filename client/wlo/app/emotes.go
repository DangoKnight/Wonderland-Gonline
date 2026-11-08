package app

import "wonderland-gonline/internal/localization"

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

// Emotes: the Social window's Emote tab and its keys. Alt+N uses Alt slot
// N (FUN_0027f248) and Ctrl+N puts Ctrl phrase N in the chat bar
// (0x27f140). An action plays the pose the table at 0x4bd7bc names
// (FUN_00430f90), turned to the player's facing (FUN_00430474), and sends
// AC32:2 with the resulting action, which other clients show as is; Alt+1
// is action 13 by default, the kneeling sit (46..53). An emoticon plays
// expression 100 + n (FUN_00430700) and sends AC32:1. The original keeps
// the slots and phrases in user\<id>\hotkey (FUN_0027b254, read by
// FUN_00278b34); the port keeps them with the other per-character
// preferences in emotes-<id>.json.
type emoteFile struct {
	Slots   [hud.EmoteSlots]hud.EmoteSlot `json:"slots"`
	Phrases [hud.CtrlPhrases]string       `json:"phrases"`
	// Attractive is the Set Up tab's switch, the hotkey file's last byte.
	Attractive bool `json:"attractive"`
}

func (c *Client) wireEmotes(s *hud.Social) {
	s.Emote = c.playEmote
	s.Pose = func(cell int) { c.playPose(cell) }
	s.Phrase = func(text []byte) {
		if c.ChatBar != nil {
			c.ChatBar.Message.SetText(text)
		}
	}
	s.EmotesChanged = c.saveEmotes
}

// emotePath is the per-character file beside the settings.
func (c *Client) emotePath() string {
	if c.World == nil || c.settingsPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(c.settingsPath), fmt.Sprintf("emotes-%d.json", c.World.Player.ID))
}

// loadEmotes is FUN_00278b34's hotkey read, at world entry.
func (c *Client) loadEmotes() {
	if c.Social == nil {
		return
	}
	f := emoteFile{Slots: hud.DefaultSlots()}
	if p := c.emotePath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			if err := json.Unmarshal(b, &f); err != nil {
				c.Chat.Notice(localization.UI("ui.app.could_not_load_emotes_7c263d70") + err.Error())
				f = emoteFile{Slots: hud.DefaultSlots()}
			}
		}
	}
	var phrases [hud.CtrlPhrases][]byte
	for i, t := range f.Phrases {
		phrases[i] = []byte(t)
	}
	c.Social.SetSlots(f.Slots)
	c.Social.SetPhrases(phrases)
	c.Social.SetAttractive(f.Attractive)
	if c.World != nil {
		p := c.World.Player
		c.Social.LoadProfile(p.Nickname, p.Blood, p.BirthYear, p.BirthMonth, p.BirthDay)
	}
}

// saveEmotes is FUN_0027b254.
func (c *Client) saveEmotes() {
	p := c.emotePath()
	if p == "" || c.Social == nil {
		return
	}
	f := emoteFile{Slots: c.Social.Slots(), Attractive: c.Social.Attractive()}
	for i, t := range c.Social.Phrases() {
		f.Phrases[i] = string(t)
	}
	b, err := json.Marshal(f)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(p), 0o700)
	}
	if err == nil {
		err = os.WriteFile(p, b, 0o600)
	}
	if err != nil {
		c.Chat.Notice(localization.UI("ui.app.could_not_save_emotes_422aef05") + err.Error())
	}
}

// emoteBlocked are the states in which the original refuses a pose.
func (c *Client) emoteBlocked() bool {
	return c.World == nil || c.held || c.event.active || c.sceneFrozen() || c.World.Player.VehicleID != 0 || c.remote.battle.active
}

// playEmote is LAB_0027f1e4: the expression locally, then AC32:1.
func (c *Client) playEmote(code byte) {
	if c.World == nil {
		return
	}
	c.World.StartExpression(c.World.Player.ID, code, c.Now())
	c.Net.Send([]byte{protocol.CommandPose, protocol.PoseEmote, code})
}

// playPose is LAB_0027e814 / FUN_0027f248 for an action cell.
func (c *Client) playPose(cell int) bool {
	if c.emoteBlocked() {
		return false
	}
	if cell < 1 || cell >= len(hud.PoseTable) {
		return false
	}
	p := &c.World.Player
	action := byte(world.PoseAction(int(hud.PoseTable[cell]), int(p.Direction)))
	c.World.StopWalk()
	c.waterTravel = waterTravelState{}
	p.Direction = int32(action)
	c.Net.Send([]byte{protocol.CommandPose, protocol.PoseBroadcast, action})
	return true
}

// GroundRightClick is FUN_0043b6ac: face a scene point while retaining the
// current pose. FUN_0041218c supplies eight directions; two-facing poses map
// directions 0..3 to their first action and 4..7 to their second. The native
// path calls FUN_00430f90 and publishes the faced action through AC32:2.
func (c *Client) GroundRightClick(x, y int) {
	if c.exitMovementMode() {
		return
	}
	if c.cancelFieldSelection() {
		return
	}
	if c.emoteBlocked() || !c.mapReady || c.movie != nil || c.sport != nil ||
		c.battle.state.Active || c.UI.Modal != nil || c.Input.Hovered != nil ||
		c.Talk.Drawn() || c.World.Walking() || x < 0 || y < 0 || x >= ScreenWidth || y >= ScreenHeight {
		return
	}
	p := &c.World.Player
	base := world.ActionBase(int(p.Direction))
	if base == 0 { // Walking or an unresolved action has no held facing to change.
		return
	}
	cx, cy := c.World.Camera()
	x, y = c.World.ScenePoint(x, y)
	tx, ty := x+cx, y+cy
	if tx == p.X && ty == p.Y {
		return
	}
	facing := world.Facing(p.X, p.Y, tx, ty, 0)
	action := byte(world.PoseAction(base, facing))
	if int32(action) == p.Direction {
		return
	}
	p.Direction = int32(action)
	c.Net.Send([]byte{protocol.CommandPose, protocol.PoseBroadcast, action})
}

// EmoteKey is Alt+1..8 and Ctrl+1..5.
func (c *Client) EmoteKey(key uint16, shift byte) bool {
	if key < '1' || key > '9' || c.World == nil {
		return false
	}
	// Typing blocks the Alt slots; a Ctrl phrase still reaches the chat
	// bar while it has the focus.
	typing := false
	if ed, ok := c.Input.Focused.(*seui.Editor); ok {
		typing = c.ChatBar == nil || ed != c.ChatBar.Message
	}
	if typing {
		return false
	}
	n := int(key - '1')
	switch {
	case shift == shiftAlt && n < hud.EmoteSlots:
		slots := hud.DefaultSlots()
		if c.Social != nil {
			slots = c.Social.Slots()
		}
		sl := slots[n]
		switch sl.Kind {
		case hud.SlotAction:
			c.playPose(sl.Value)
		case hud.SlotEmoticon:
			c.playEmote(byte(sl.Value + hud.EmoticonBase))
		}
		return true
	case shift == shiftCtrl && n < hud.CtrlPhrases && c.Social != nil:
		c.Social.UsePhrase(n)
		return true
	}
	return false
}
