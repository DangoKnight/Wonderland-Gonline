package hud

import "bytes"

// Talk text markup. FUN_00478140 scans a line for tags (FUN_00477e2c): an
// opener "#X" counts only when its closer "/#X" follows somewhere after it.
// Both are removed (FUN_00014374), and the opener's kind is kept for the
// characters between them:
//
//	#B bold (1)        #R red (2)          #s sound (3), "#swav1541"
//	#P (4, no style)   #n player name (5)  #e line break (6)
//	#F1..#F3 face (7..9)  #g castle owner (11)  #W white (12)
//
// A sound opener carries the effect's seven-character name; #n, #e and #g
// insert their text at the opener. FUN_00470f48 turns the kinds into a
// character's style: bold, red (0xf800) over white (0xffff) over the
// default yellow (FUN_00471028).
const (
	markBold = 1 << iota
	markRed
	markWhite
)

const (
	talkInkRed   = 0xf800
	talkInkWhite = 0xffff
	soundNameLen = 7
	// noCastleOwner is #g's text while no guild owns the castle.
	noCastleOwner = "No guild own Castle"
)

// talkText is a line with its markup applied.
type talkText struct {
	text   []byte
	marks  []byte // one style per byte of text
	sounds []string
	face   int // #F1..#F3, 0 for none
}

// parseTalk applies the markup; name is the player's name for #n.
func parseTalk(src, name []byte) talkText {
	var t talkText
	var open byte // active style
	for i := 0; i < len(src); {
		if src[i] == '#' && i+1 < len(src) {
			c := lower(src[i+1])
			closer := []byte{'/', '#', src[i+1]}
			if bytes.Contains(src[i+2:], closer) || bytes.Contains(src[i+2:], []byte{'/', '#', upper(src[i+1])}) {
				switch c {
				case 's':
					if i+2+soundNameLen <= len(src) {
						t.sounds = append(t.sounds, string(src[i+2:i+2+soundNameLen]))
						i += 2 + soundNameLen
						continue
					}
				case 'f':
					if i+2 < len(src) && src[i+2] >= '1' && src[i+2] <= '3' {
						t.face = int(src[i+2] - '0')
						i += 3
						continue
					}
				case 'b', 'r', 'w', 'p':
					open |= markOf(c)
					i += 2
					continue
				case 'n':
					t.add(name, open)
					i += 2
					continue
				case 'e':
					t.add([]byte{'\r'}, open)
					i += 2
					continue
				case 'g':
					t.add([]byte(noCastleOwner), open)
					i += 2
					continue
				}
			}
		}
		if src[i] == '/' && i+2 < len(src) && src[i+1] == '#' && isTag(lower(src[i+2])) {
			open &^= markOf(lower(src[i+2]))
			i += 3
			continue
		}
		n := 1
		if src[i] >= 0x80 && i+1 < len(src) {
			n = 2
		}
		t.add(src[i:i+n], open)
		i += n
	}
	return t
}

func (t *talkText) add(b []byte, mark byte) {
	t.text = append(t.text, b...)
	for range b {
		t.marks = append(t.marks, mark)
	}
}

func markOf(c byte) byte {
	switch c {
	case 'b':
		return markBold
	case 'r':
		return markRed
	case 'w':
		return markWhite
	}
	return 0
}

func isTag(c byte) bool { return bytes.IndexByte([]byte("bsrpnefgw"), c) >= 0 }

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func upper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 'a' + 'A'
	}
	return c
}

// inkOf is FUN_00471028's colour for a style.
func inkOf(mark byte) uint16 {
	switch {
	case mark&markRed != 0:
		return talkInkRed
	case mark&markWhite != 0:
		return talkInkWhite
	}
	return talkInk
}
