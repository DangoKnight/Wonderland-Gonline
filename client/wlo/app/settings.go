package app

import "wonderland-gonline/internal/localization"

import (
	"io"
	"log"
	"os"
	"time"

	"math"
	"wonderland-gonline/client/debuglog"
	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/settings"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/protocol"
)

const (
	mainSettingsButton    = 6
	escapeKey             = 0x1b
	volumeAttenuationStep = 300 // native FUN_002875b8: hundredths of a dB per step
)

func (c *Client) initSettings(path string) {
	if path == "" {
		path = c.Assets.UserPath("settings.json")
	}
	c.settingsPath = path
	c.SettingsState = settings.NewState()
	if l, err := settings.Load(path); err == nil {
		c.SettingsState.Local = l
	} else {
		c.Chat.Notice(localization.UI("ui.app.could_not_load_settings_c698905e") + err.Error())
	}
	c.Settings = settings.NewForm(c.Env, c.SettingsState)
	c.UI.Add(c.Settings)
	c.Settings.Send = func(p []byte) {
		if err := c.Net.Send(p); err != nil {
			c.Chat.Notice(err.Error())
		}
	}
	c.Settings.Changed = func() {
		c.applyLocalSettings()
		if err := c.SettingsState.Local.Save(c.settingsPath); err != nil {
			c.Chat.Notice(localization.UI("ui.app.could_not_save_settings_e51e17c4") + err.Error())
		}
	}
	c.Settings.OnHide = c.closeSettingsPrompt
	c.Settings.Action = c.settingsAction
	c.Settings.Notice = c.Chat.Notice
	c.MainButtons.Buttons[mainSettingsButton].OnClick = func() {
		if c.Settings.Visible {
			c.Settings.Hide()
		} else if c.canOpenModule() {
			c.Settings.Show()
		}
	}
	c.applyLocalSettings()
}
func (c *Client) SettingsKey(key uint16) bool {
	if key != escapeKey {
		return false
	}
	if c.settingsPromptForm != nil {
		c.closeSettingsPrompt()
		return true
	}
	return c.Settings != nil && c.Settings.Escape()
}
func (c *Client) settingsPacket(p []byte) {
	if c.SettingsState.Apply(p) {
		return
	}
	// Native AC33:1 reports rejected settings; refresh from the authoritative server.
	if len(p) == 3 && p[1] == protocol.SettingsError {
		c.Chat.Notice(localization.UI("ui.app.the_server_rejected_that_setting_98cb9f44"))
		c.Settings.Send([]byte{protocol.CommandSettings, protocol.SettingsSnapshot})
		return
	}
	if c.Unhandled != nil {
		c.Unhandled(p)
	}
}
func audioGain(on bool, steps int) float64 {
	if !on {
		return 0
	}
	steps = max(0, min(settings.VolumeSteps, steps))
	return math.Pow(10, -float64((settings.VolumeSteps-steps)*volumeAttenuationStep)/hundredthsPerBel)
}
func (c *Client) effectsGain() float64 {
	if c.backgroundSession {
		return 0
	}
	if c.SettingsState == nil {
		return 1
	}
	l := c.SettingsState.Local
	return audioGain(l.SoundOn, l.SoundVolume)
}
func (c *Client) applyLocalSettings() {
	if c.SettingsState == nil {
		return
	}
	l := c.SettingsState.Local
	if c.Music != nil {
		gain := audioGain(l.MusicOn, l.MusicVolume)
		if c.backgroundSession {
			gain = 0
		}
		c.Music.SetVolume(gain)
	}
	if c.sfx != nil {
		c.sfx.SetVolume(c.effectsGain())
	}
	// Apply sound volume to active positional loops without restarting them.
	gain := c.effectsGain()
	if gain == 0 {
		c.stopAmbience()
	} else if c.World != nil {
		for _, ch := range c.ambient.ch {
			if ch != nil {
				ch.player.SetVolume(fromHundredths(ambientFalloff*abs(c.World.Player.Y-ch.srcY)) * gain)
			}
		}
	}
	if c.World != nil {
		c.World.HidePeerNames = !l.Info[settings.InfoOtherNames]
		c.World.HideOwnNickname = !l.Info[settings.InfoOwnNickname]
		c.World.HidePeerNicknames = !l.Info[settings.InfoOtherNicknames]
	}
	channels := [settings.ChannelCount]int{hud.ChannelLocal, hud.ChannelWhisper, hud.ChannelTeam, hud.ChannelGuild, hud.ChannelWorld}
	colors := make(map[int]uint16, len(channels))
	for i, ch := range channels {
		colors[ch] = settings.Palette[l.Colors[i]]
	}
	c.Chat.SetColors(colors)
	c.applyDebugLog(l)
}

// applyDebugLog starts, re-targets or stops the session transcript. The file is
// only created when the setting is on, and the recorder is installed on the
// socket so every packet in both directions is captured.
func (c *Client) applyDebugLog(l settings.Local) {
	if !l.DebugLog {
		c.stopDebugLog()
		return
	}
	if c.debugLog != nil && c.debugLogCap == l.DebugLogMaxLines {
		return
	}
	c.stopDebugLog()
	recorded, err := debuglog.Open(c.Assets.UserPath("logs"), l.DebugLogMaxLines, c.Now())
	if err != nil {
		c.Chat.Notice("Could not start the debug log: " + err.Error())
		return
	}
	c.debugLog = recorded
	c.debugLogCap = l.DebugLogMaxLines
	c.Net.SetObserver(recorded.Packet)
	// Game log lines share the transcript while recording is on.
	log.SetOutput(io.MultiWriter(os.Stderr, recorded))
	c.Chat.Notice("Debug log started: " + recorded.Path())
	if l.DebugLogMaxLines > 0 {
		c.Chat.Notice(settings.DebugLimitLabel(l.DebugLogMaxLines) + " lines kept; a long session still writes a large file.")
	} else {
		c.Chat.Notice("Line limit is off: a long session can fill the disk.")
	}
}

// stopDebugLog flushes and releases the transcript, restoring the standard
// logger's own output.
func (c *Client) stopDebugLog() {
	if c.debugLog == nil {
		return
	}
	c.Net.SetObserver(nil)
	log.SetOutput(os.Stderr)
	if err := c.debugLog.Close(); err != nil {
		c.Chat.Notice("Debug log: " + err.Error())
	}
	c.debugLog = nil
	c.debugLogCap = 0
}
func (c *Client) settingsAction(action string) {
	switch action {
	case "chat":
		c.Chat.Visible = !c.Chat.Visible
	case "readme":
		c.Chat.Notice("Wonderland Gonline: see docs/CLIENT.md and docs/GETTING_STARTED.md.")
	case "spawn":
		c.settingsPrompt(localization.UI("ui.app.select_return_point_12e68ead"), []string{localization.UI("ui.app.beach_c1624a26"), localization.UI("ui.app.record_bfdd5106"), localization.UI("ui.app.carnie_36d09707")}, func(i int) {
			c.Settings.Send([]byte{protocol.CommandCharacterState, protocol.CharacterStateTeleport, byte(i + 1)})
		})
	case "logout":
		c.logoutConfirmation(false)
	case "exit":
		c.logoutConfirmation(true)
	default:
		c.Chat.Notice(localization.UI("ui.app.this_settings_service_is_not_yet_availab_e6613048"))
	}
}

// settingsPrompt uses the same message panel/buttons as the native message form.
func (c *Client) settingsPrompt(message string, choices []string, selectChoice func(int)) {
	c.closeSettingsPrompt()
	f := seui.NewForm(c.Env)
	f.AnimateMessage(func() time.Time { return c.Now() })
	c.settingsPromptForm = f
	f.Dockable = false
	f.Init("panel15", 250, 50, 50, 0, 0, true, 140, 300, 180)
	f.SetMargins(15, 15, 15, 15)
	text := seui.NewEditor(c.Env, f)
	text.Init("", 15, 0, 0, 0, 0, false, 25, 270, 30)
	text.ReadOnly = true
	text.SetText([]byte(message))
	text.Color = 0xffff
	c.UI.Add(f)
	for i, label := range choices {
		choice := i
		b := seui.NewButton(c.Env, f)
		b.Init("btn_module_1", 15+i*90, 20, 80, 0, 0, true, 20, 80, 90)
		b.Color = 0xffff
		b.SetCaption([]byte(label))
		b.OnClick = func() { c.closeSettingsPrompt(); selectChoice(choice) }
	}
	c.UI.Modal = f
	f.Show()
}

func (c *Client) closeSettingsPrompt() {
	f := c.settingsPromptForm
	if f == nil {
		return
	}
	c.settingsPromptForm = nil
	if c.UI.Modal == f {
		c.UI.Modal = nil
	}
	f.Hide()
	c.UI.Remove(f)
	if c.logoutPrompt != nil {
		logout := c.logoutPrompt
		c.logoutPrompt = nil
		if c.UI.Modal == logout {
			c.UI.Modal = nil
		}
		logout.Hide()
		c.UI.Remove(logout)
	}
}
