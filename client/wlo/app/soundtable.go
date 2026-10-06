package app

import (
	"encoding/json"
	"strings"
	"wonderland-gonline/internal/clientfs"
)

// The sound table (sound\soundtabel.txt, FUN_00493c7c): its first line is
// read and dropped, then each line is one entry, numbered from 1. Movie
// stages play an entry as the music (FUN_004048b8) and keyframes as a
// sound (FUN_00404fa4), both under sound\.
const soundTableExport = "soundtabel.txt.json"

// soundTable is the table's entries, index 0 unused.
func (c *Client) soundTable() []string {
	if c.sounds != nil {
		return c.sounds
	}
	c.sounds = []string{""}
	b, err := clientfs.ReadFile(c.Assets.MediaPath("sound", soundTableExport))
	if err != nil {
		return c.sounds
	}
	var doc struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(b, &doc) != nil {
		return c.sounds
	}
	lines := strings.Split(strings.ReplaceAll(doc.Text, "\r\n", "\n"), "\n")
	for _, l := range lines[min(1, len(lines)):] {
		if l = strings.TrimSpace(l); l != "" {
			c.sounds = append(c.sounds, l)
		}
	}
	return c.sounds
}

// tableEntry is a sound table entry's path, "" when out of range.
func (c *Client) tableEntry(i int) string {
	if t := c.soundTable(); i > 0 && i < len(t) {
		return musicDir + t[i]
	}
	return ""
}

// playTableMusic plays a sound table entry as the music.
func (c *Client) playTableMusic(i int) {
	if path := c.tableEntry(i); path != "" && c.Music != nil {
		c.Music.Play(path)
	}
}

// playTableSound plays a sound table entry once.
func (c *Client) playTableSound(i int) {
	if path := c.tableEntry(i); path != "" && c.Env.Sound != nil {
		c.Env.Sound(path)
	}
}
