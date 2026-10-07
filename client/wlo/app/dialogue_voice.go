package app

import (
	"fmt"
	"wonderland-gonline/client/wlo/movie"
	"wonderland-gonline/internal/clientfs"
)

const (
	voicePlayerPrefix     = "_10"
	voiceNPCPrefix        = "_20"
	voiceCompanionPrefix  = "_30"
	voiceRobinsonTemplate = 10000
	voiceRobinsonSuffix   = "_1010"
	voiceNPCMaximum       = 200
)

// FUN_004855a8 maps body/head to the voice actor, independently of the
// character creation carousel. These are authored compatibility values.
var playerVoiceActors = map[[2]byte]byte{
	{1, 0}: 1, {2, 0}: 2, {2, 1}: 3, {3, 0}: 4, {3, 1}: 5, {3, 2}: 6,
	{3, 3}: 14, {4, 0}: 7, {4, 1}: 8, {4, 2}: 9, {4, 3}: 10,
	{4, 4}: 11, {4, 5}: 12, {4, 6}: 13, {4, 7}: 15,
}
var companionVoiceActors = map[byte]byte{201: 3, 206: 6, 211: 2, 216: 5, 221: 1, 226: 4}

// FUN_0034d3e8 prefers a speaker-specific recording, then the plain talk
// ID. The base odd archive wins over odd_d01 for specific recordings;
// the native plain-ID fallback consults only odd.
func (c *Client) dialogueVoice(talk uint16, speaker uint32) string {
	suffix := ""
	if speaker == movie.PlayerTemplate {
		p := c.World.Player
		suffix = fmt.Sprintf("%s%02d", voicePlayerPrefix, playerVoiceActors[[2]byte{p.Body, p.Head}])
	} else if speaker == voiceRobinsonTemplate {
		suffix = voiceRobinsonSuffix
	} else if t, ok := c.npcTemplates[speaker]; ok && t.Voice != 0 {
		if t.Voice <= voiceNPCMaximum {
			suffix = fmt.Sprintf("%s%02d", voiceNPCPrefix, t.Voice)
		} else if actor := companionVoiceActors[t.Voice]; actor != 0 {
			suffix = fmt.Sprintf("%s%02d", voiceCompanionPrefix, actor)
		}
	}
	if suffix != "" {
		for _, archive := range []string{"odd", "odd_d01"} {
			path := fmt.Sprintf(`..\audio\%s\%d%s.ogg`, archive, talk, suffix)
			if _, err := clientfs.Stat(c.Assets.MediaPath("..", "audio", archive, fmt.Sprintf("%d%s.ogg", talk, suffix))); err == nil {
				return path
			}
		}
	}
	name := fmt.Sprintf("%d.ogg", talk)
	if _, err := clientfs.Stat(c.Assets.DataPath("audio", "odd", name)); err == nil {
		return `..\audio\odd\` + name
	}
	return ""
}

func (c *Client) playDialogueVoice(talk uint16, speaker uint32) {
	if c.World != nil && c.Env.Sound != nil {
		if c.sfx != nil {
			c.sfx.StopVoice()
		}
		if path := c.dialogueVoice(talk, speaker); path != "" {
			if c.sfx != nil {
				c.sfx.PlayVoice(path)
			} else {
				c.Env.Sound(path)
			}
		}
	}
}
